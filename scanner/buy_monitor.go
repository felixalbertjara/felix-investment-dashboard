package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBuyMonitorInterval = 12 * time.Minute
	defaultBuyMinScore        = 90
	buyNotifyCooldown         = 12 * time.Hour
)

type BuyAlertState struct {
	Ticker            string    `json:"ticker"`
	Recommendation    string    `json:"recommendation"`
	Score             int       `json:"score"`
	Price             float64   `json:"price"`
	PendingStrong     int       `json:"pending_strong"`
	PendingCancel     int       `json:"pending_cancel"`
	ActiveStrongAlert bool      `json:"active_strong_alert"`
	LastNotifiedAt    time.Time `json:"last_notified_at,omitempty"`
	LastNotifiedScore int       `json:"last_notified_score,omitempty"`
	LastReason        string    `json:"last_reason,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type buyCandidate struct {
	Result Result
	V4     V4Analysis
	State  BuyAlertState
}

func buyStatePath() string { return filepath.Join(stateDir(), "buy_alerts_state.json") }

func buyMonitorInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("FELIX_BUY_SCAN_MINUTES")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 5 && n <= 120 {
			return time.Duration(n) * time.Minute
		}
	}
	return defaultBuyMonitorInterval
}

func buyMinScore() int {
	if raw := strings.TrimSpace(os.Getenv("FELIX_BUY_MIN_SCORE")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 80 && n <= 100 {
			return n
		}
	}
	return defaultBuyMinScore
}

func buyMonitorEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("FELIX_BUY_MONITOR")))
	return raw != "0" && raw != "false" && raw != "off"
}

func argentinaNow() time.Time {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		loc = time.FixedZone("ART", -3*60*60)
	}
	return time.Now().In(loc)
}

func isArgentinaMarketWindow() bool {
	now := argentinaNow()
	wd := now.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	mins := now.Hour()*60 + now.Minute()
	// Ventana de BYMA usada por el scanner. Margen de 5 min para evitar datos de pre/apertura.
	return mins >= 11*60 && mins <= 17*60
}

func buyMonitorLoop(ctx interface{ Done() <-chan struct{} }) {
	if !buyMonitorEnabled() {
		log.Println("Buy monitor deshabilitado por FELIX_BUY_MONITOR")
		return
	}
	// El primer scan espera un poco para que red/Tailscale terminen de levantar.
	timer := time.NewTimer(45 * time.Second)
	select {
	case <-ctx.Done():
		timer.Stop()
		return
	case <-timer.C:
	}
	runBuyMonitorOnce(false)

	ticker := time.NewTicker(buyMonitorInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runBuyMonitorOnce(false)
		}
	}
}

func runBuyMonitorOnce(force bool) {
	runBuyMonitorCore(force, force)
}

// En serverless el Worker ya validó lunes-viernes + ventana 11:00-17:00 ART.
// Saltamos sólo el chequeo de reloj local del runner, pero mantenemos las 2 confirmaciones normales de compra/cancelación.
func runBuyMonitorServerless() {
	runBuyMonitorCore(true, false)
}

func runBuyMonitorCore(skipWindow bool, forceConfirm bool) {
	if !skipWindow && !isArgentinaMarketWindow() {
		return
	}

	configured, _ := telegramConfigurationStatus()
	if !configured {
		log.Println("Buy monitor: Telegram no está configurado; el radar igual se calcula pero se omiten notificaciones.")
	}

	store := loadPortfolio()
	owned := map[string]bool{}
	for _, p := range store.Positions {
		owned[strings.ToUpper(strings.TrimSpace(p.Ticker))] = true
	}

	allAssets := loadAllAssets()
	results, _, _ := getMarketResultsCached(allAssets, owned)
	states := loadBuyStates()
	now := argentinaNow()
	threshold := buyMinScore()

	newBuys := []buyCandidate{}
	cancelled := []buyCandidate{}
	seen := map[string]bool{}

	for _, r := range results {
		t := strings.ToUpper(strings.TrimSpace(r.Asset.Ticker))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true

		// Para CEDEARs avisamos la especie base ARS, no las variantes C/D de liquidación.
		if r.Asset.Type == "CEDEAR" && guessUnderlying(t) != t {
			continue
		}
		if strings.Contains(t, ".") {
			continue
		}

		deep := strings.Contains(strings.ToLower(r.AnalysisMode), "profundo")
		st, hadState := states[t]
		// En cloud no persistimos estado inútil para cientos de filas livianas.
		// Un candidato sólo puede activar compra con análisis profundo; conservamos
		// también cualquier estado previo para poder cerrar/cancelar una señal activa.
		if !deep && !hadState {
			continue
		}
		st.Ticker = t
		st.Recommendation = r.Recommendation
		st.Score = r.ScoreFinal
		st.Price = r.Quote.Price
		st.UpdatedAt = now
		st.LastReason = firstReason(r.Reasons)

		// Si ya está en cartera, deja de ser candidato de compra; el monitor de salida toma el control.
		if owned[t] {
			st.PendingStrong = 0
			st.PendingCancel = 0
			st.ActiveStrongAlert = false
			states[t] = st
			continue
		}

		v4 := V4Analysis{}
		if deep && r.Quote.Price > 0 {
			v4 = evaluateV4(r)
			st.Recommendation = v4.RecommendationV4
			st.Score = v4.TradingScore
			if len(v4.V4Reasons) > 0 {
				st.LastReason = v4.V4Reasons[0]
			}
		}
		// V4: Telegram sólo se activa para la señal rara que pasa TODOS los hard gates.
		strong := deep && r.Quote.Price > 0 && v4.StrongBuy && v4.TradingScore >= threshold

		if strong {
			st.PendingCancel = 0
			st.PendingStrong++
			if forceConfirm && st.PendingStrong < 2 {
				st.PendingStrong = 2
			}
			if st.PendingStrong >= 2 {
				shouldNotify := !st.ActiveStrongAlert
				if st.ActiveStrongAlert && !st.LastNotifiedAt.IsZero() && time.Since(st.LastNotifiedAt) >= buyNotifyCooldown && v4.TradingScore >= st.LastNotifiedScore+3 {
					shouldNotify = true
				}
				if shouldNotify {
					st.ActiveStrongAlert = true
					st.LastNotifiedAt = now
					st.LastNotifiedScore = v4.TradingScore
					newBuys = append(newBuys, buyCandidate{Result: r, V4: v4, State: st})
				}
			}
		} else {
			st.PendingStrong = 0
			if st.ActiveStrongAlert {
				st.PendingCancel++
				if forceConfirm && st.PendingCancel < 2 {
					st.PendingCancel = 2
				}
				if st.PendingCancel >= 2 {
					st.ActiveStrongAlert = false
					st.PendingCancel = 0
					cancelled = append(cancelled, buyCandidate{Result: r, V4: v4, State: st})
				}
			} else {
				st.PendingCancel = 0
			}
		}
		states[t] = st
	}

	saveBuyStates(states)

	if len(newBuys) > 0 {
		sort.Slice(newBuys, func(i, j int) bool { return newBuys[i].V4.TradingScore > newBuys[j].V4.TradingScore })
		if len(newBuys) > 5 {
			newBuys = newBuys[:5]
		}
		var b strings.Builder
		b.WriteString("🔥 RADAR · OPORTUNIDAD JUGOSA CONFIRMADA\n\n")
		for i, c := range newBuys {
			r, v4 := c.Result, c.V4
			fmt.Fprintf(&b, "%d) %s · %s\n", i+1, r.Asset.Ticker, v4.RecommendationV4)
			fmt.Fprintf(&b, "Trading %d/100 · Largo %d/100 · %s\n", v4.TradingScore, v4.LongScore, v4.TradeSetup)
			fmt.Fprintf(&b, "Precio: %s · R/R %.2f:1 · ADX %.1f\n", money(r.Quote.Price), v4.RewardRisk, v4.ADX)
			fmt.Fprintf(&b, "Histórico: %s · %d trades · PF %.2f · Exp %.2fR\n", v4.HistoricalConfidence, v4.HistoricalTrades, v4.HistoricalProfitFactor, v4.HistoricalExpectancyR)
			fmt.Fprintf(&b, "VIX: %.1f (%s) · Mercado: %s\n", v4.VIXLevel, v4.VIXRegime, v4.MarketRegime)
			fmt.Fprintf(&b, "Ventana esperada: %s · Benchmark %s\n", v4.ExpectedWindow, v4.BenchmarkTicker)
			fmt.Fprintf(&b, "Fuerza relativa: 20r %+0.1f%% · 60r %+0.1f%%\n", v4.RelativeStrength20, v4.RelativeStrength60)
			if r.UnderlyingTicker != "" {
				fmt.Fprintf(&b, "Underlying %s: score %d · CCL20 %.1f%%\n", r.UnderlyingTicker, r.UnderlyingScore, r.CCLChange20Pct)
			}
			if len(v4.V4Reasons) > 0 {
				fmt.Fprintf(&b, "Motivo: %s\n", v4.V4Reasons[0])
			}
			fmt.Fprintln(&b)
		}
		b.WriteString("Acción: revisar puntas/spread en Balanz antes de ejecutar. FARO exige tendencia, momentum, breakout, fuerza relativa, R/R y validación histórica. No es una garantía de ganancia.")
		if configured {
			if err := sendTelegramMessage(b.String()); err != nil {
				log.Println("Buy monitor Telegram:", err)
			}
		}
	}

	if len(cancelled) > 0 {
		sort.Slice(cancelled, func(i, j int) bool { return cancelled[i].V4.TradingScore < cancelled[j].V4.TradingScore })
		if len(cancelled) > 5 {
			cancelled = cancelled[:5]
		}
		var b strings.Builder
		b.WriteString("⚪ RADAR · OPORTUNIDAD CANCELADA\n\n")
		for _, c := range cancelled {
			r := c.Result
			fmt.Fprintf(&b, "%s → %s · Trading %d/100\n", r.Asset.Ticker, c.V4.RecommendationV4, c.V4.TradingScore)
		}
		b.WriteString("Acción: NO comprar por la señal anterior; esperar un nuevo setup confirmado.")
		if configured {
			if err := sendTelegramMessage(b.String()); err != nil {
				log.Println("Buy cancel Telegram:", err)
			}
		}
	}
}

func loadBuyStates() map[string]BuyAlertState {
	b, err := os.ReadFile(buyStatePath())
	if err != nil {
		return map[string]BuyAlertState{}
	}
	out := map[string]BuyAlertState{}
	if json.Unmarshal(b, &out) != nil {
		return map[string]BuyAlertState{}
	}
	return out
}

func saveBuyStates(m map[string]BuyAlertState) {
	_ = writeJSONAtomic(buyStatePath(), m)
}
