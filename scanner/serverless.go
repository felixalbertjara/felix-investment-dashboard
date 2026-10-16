package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	serverlessPortfolioSnapshot = "portfolio_snapshot.json"
	serverlessMarketSnapshot    = "market_snapshot.json"
)

var (
	serverlessPortfolioVersion    string
	serverlessAlertHistoryVersion string
)

type serverlessExport struct {
	Portfolio           json.RawMessage `json:"portfolio"`
	PortfolioVersion    string          `json:"portfolio_version"`
	AlertState          json.RawMessage `json:"alert_state"`
	AlertHistory        json.RawMessage `json:"alert_history"`
	AlertHistoryVersion string          `json:"alert_history_version"`
	LastPrices          json.RawMessage `json:"last_prices"`
	BuyAlertsState      json.RawMessage `json:"buy_alerts_state"`
}

type serverlessImport struct {
	PortfolioPlanUpdates    []PortfolioPosition `json:"portfolio_plan_updates,omitempty"`
	PortfolioBaseVersion    string              `json:"portfolio_base_version,omitempty"`
	AlertHistoryBaseVersion string              `json:"alert_history_base_version,omitempty"`
	AlertState              json.RawMessage     `json:"alert_state,omitempty"`
	AlertHistory            json.RawMessage     `json:"alert_history,omitempty"`
	LastPrices              json.RawMessage     `json:"last_prices,omitempty"`
	BuyAlertsState          json.RawMessage     `json:"buy_alerts_state,omitempty"`
	ScanMode                string              `json:"scan_mode"`
	ScanAt                  string              `json:"scan_at"`
}

type portfolioSnapshot struct {
	GeneratedAt      string             `json:"generated_at"`
	Portfolio        []PortfolioView    `json:"portfolio"`
	AlertHistory     []AlertHistoryItem `json:"alert_history"`
	PortfolioSummary string             `json:"portfolio_summary"`
	TotalInvested    float64            `json:"total_invested"`
	TotalCurrent     float64            `json:"total_current"`
	TotalPnL         float64            `json:"total_pnl"`
	TotalPnLPct      float64            `json:"total_pnl_pct"`
}

type marketSnapshot struct {
	GeneratedAt  string     `json:"generated_at"`
	Results      []V4Result `json:"results"`
	Summary      string     `json:"summary"`
	TotalAssets  int        `json:"total_assets"`
	TotalActions int        `json:"total_actions"`
	TotalCedears int        `json:"total_cedears"`
	Errors       []string   `json:"errors,omitempty"`
}

func runServerlessScan() error {
	log.SetOutput(os.Stdout)
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SCAN_MODE")))
	if mode == "" {
		mode = "portfolio"
	}
	if mode != "portfolio" && mode != "radar" {
		return fmt.Errorf("SCAN_MODE inválido: %s", mode)
	}
	if err := syncServerlessStateDown(); err != nil {
		return err
	}

	force := strings.EqualFold(strings.TrimSpace(os.Getenv("FORCE_SCAN")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("FORCE_SCAN")), "true")
	if !force {
		authorized, reason, err := serverlessMarketAuthorized()
		if err != nil {
			return err
		}
		if !authorized {
			fmt.Println("Fuera de la ventana lunes-viernes 11:00-17:00 ART. Scan omitido:", reason)
			return nil
		}
	}

	if err := ensurePendingTradePlans(); err != nil {
		fmt.Println("WARN plan:", err)
	}

	imp := serverlessImport{ScanMode: mode, ScanAt: argentinaNow().Format(time.RFC3339)}
	if mode == "portfolio" {
		views := runMonitorOnce()
		snap := makePortfolioSnapshot(views)
		if err := writeJSONAtomic(filepath.Join(stateDir(), serverlessPortfolioSnapshot), snap); err != nil {
			return err
		}
	} else {
		// En radar usamos la lógica normal: exige dos lecturas consecutivas para COMPRA FUERTE.
		runBuyMonitorServerless()
		d := buildDashboard() // reutiliza el cache del radar dentro del mismo proceso
		snap := makeMarketSnapshot(d)
		if err := writeJSONAtomic(filepath.Join(stateDir(), serverlessMarketSnapshot), snap); err != nil {
			return err
		}
	}

	return syncServerlessStateUp(mode, imp)
}

func makePortfolioSnapshot(views []PortfolioView) portfolioSnapshot {
	clean := append([]PortfolioView(nil), views...)
	totalInv, totalCur := 0.0, 0.0
	for i := range clean {
		clean[i].Result.Quote.Candles = nil
		totalInv += clean[i].Position.AmountARS
		totalCur += clean[i].CurrentValue
	}
	pnl := totalCur - totalInv
	pnlPct := 0.0
	if totalInv > 0 {
		pnlPct = pnl / totalInv * 100
	}
	return portfolioSnapshot{
		GeneratedAt:      argentinaNow().Format("02/01/2006 15:04:05"),
		Portfolio:        clean,
		AlertHistory:     loadAlertHistory(),
		PortfolioSummary: buildPortfolioSummary(views),
		TotalInvested:    totalInv,
		TotalCurrent:     totalCur,
		TotalPnL:         pnl,
		TotalPnLPct:      pnlPct,
	}
}

func makeMarketSnapshot(d Dashboard) marketSnapshot {
	results := make([]V4Result, 0, len(d.Results))
	for _, base := range d.Results {
		v4 := V4Analysis{}
		if strings.Contains(strings.ToLower(base.AnalysisMode), "profundo") {
			v4 = evaluateV4(base)
		}
		// Los indicadores ya están calculados; las velas crudas no se muestran en la web.
		// Quitarlas suele achicar el snapshot de varios MB a unos pocos cientos de KB.
		base.Quote.Candles = nil
		results = append(results, V4Result{Result: base, V4Analysis: v4})
	}
	// Las oportunidades V4 más interesantes quedan arriba del radar.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].StrongBuy != results[j].StrongBuy {
			return results[i].StrongBuy
		}
		if results[i].TradingScore != results[j].TradingScore {
			return results[i].TradingScore > results[j].TradingScore
		}
		return results[i].ScoreFinal > results[j].ScoreFinal
	})
	return marketSnapshot{
		GeneratedAt:  d.GeneratedAt,
		Results:      results,
		Summary:      d.Summary,
		TotalAssets:  d.TotalAssets,
		TotalActions: d.TotalActions,
		TotalCedears: d.TotalCedears,
		Errors:       d.Errors,
	}
}

func serverlessMarketAuthorized() (bool, string, error) {
	api, secret, err := serverlessConfig()
	if err != nil {
		return false, "", err
	}
	at := strings.TrimSpace(os.Getenv("SCAN_AT"))
	endpoint := api + "/internal/market-status"
	if at != "" {
		endpoint += "?at=" + url.QueryEscape(at)
	}
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("market-status HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var st struct {
		Open   bool   `json:"open"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		return false, "", err
	}
	return st.Open, st.Reason, nil
}

func serverlessMarketWindowNow() bool {
	now := argentinaNow()
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}
	mins := now.Hour()*60 + now.Minute()
	return mins >= 11*60 && mins <= 17*60
}

func ensurePendingTradePlans() error {
	portfolioMu.Lock()
	defer portfolioMu.Unlock()
	store := loadPortfolioUnlocked()
	changed := false
	for i := range store.Positions {
		p := &store.Positions[i]
		if p.AvgBuyPrice <= 0 {
			continue
		}
		if p.Strategy == "" {
			p.Strategy = "SWING"
			changed = true
		}
		if p.ExpectedDays <= 0 {
			p.ExpectedDays = defaultExpectedDays(p.Strategy)
			changed = true
		}
		if p.InitialStopPrice > 0 && p.InitialRiskPct > 0 && !p.PlanCreatedAt.IsZero() {
			continue
		}
		plan := captureTradePlan(p.Ticker, p.AvgBuyPrice, p.Strategy, p.ExpectedDays, p.InitialStopPrice)
		if plan.InitialStopPrice <= 0 {
			continue
		}
		p.Strategy = plan.Strategy
		p.ExpectedDays = plan.ExpectedDays
		p.InitialStopPrice = plan.InitialStopPrice
		p.InitialRiskPct = plan.InitialRiskPct
		p.StopSource = plan.StopSource
		p.EntryATR = plan.EntryATR
		p.EntrySupport = plan.EntrySupport
		p.EntryRSI = plan.EntryRSI
		p.EntryScore = plan.EntryScore
		p.EntryUnderlyingScore = plan.EntryUnderlyingScore
		p.EntryCCLProxy = plan.EntryCCLProxy
		p.PlanCreatedAt = plan.PlanCreatedAt
		p.UpdatedAt = time.Now()
		changed = true
	}
	if changed {
		return savePortfolioUnlocked(store)
	}
	return nil
}

func syncServerlessStateDown() error {
	api, secret, err := serverlessConfig()
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodGet, api+"/internal/export", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("descargando estado: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("export HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var x serverlessExport
	if err := json.Unmarshal(body, &x); err != nil {
		return err
	}
	serverlessPortfolioVersion = x.PortfolioVersion
	serverlessAlertHistoryVersion = x.AlertHistoryVersion
	ensureRuntimeLayout()
	defaults := map[string][]byte{
		portfolioFile:           []byte(`{"positions":[]}`),
		alertStateFile:          []byte(`{}`),
		alertHistoryFile:        []byte(`[]`),
		lastPricesFile:          []byte(`{"prices":{}}`),
		"buy_alerts_state.json": []byte(`{}`),
	}
	entries := []struct {
		name string
		raw  json.RawMessage
	}{
		{portfolioFile, x.Portfolio}, {alertStateFile, x.AlertState}, {alertHistoryFile, x.AlertHistory},
		{lastPricesFile, x.LastPrices}, {"buy_alerts_state.json", x.BuyAlertsState},
	}
	for _, e := range entries {
		b := []byte(e.raw)
		if len(bytes.TrimSpace(b)) == 0 || string(bytes.TrimSpace(b)) == "null" {
			b = defaults[e.name]
		}
		path := filepath.Join(stateDir(), e.name)
		if err := os.WriteFile(path, b, 0644); err != nil {
			return err
		}
	}
	return nil
}

func syncServerlessStateUp(mode string, imp serverlessImport) error {
	api, secret, err := serverlessConfig()
	if err != nil {
		return err
	}
	read := func(name string) json.RawMessage {
		b, err := os.ReadFile(filepath.Join(stateDir(), name))
		if err != nil {
			return nil
		}
		return json.RawMessage(b)
	}
	// El Worker es dueño de monto/precio/fecha/notas de la cartera. El scanner sólo
	// devuelve campos del plan técnico, así una ejecución concurrente nunca pisa una compra nueva.
	store := loadPortfolio()
	imp.PortfolioPlanUpdates = append([]PortfolioPosition(nil), store.Positions...)
	imp.PortfolioBaseVersion = serverlessPortfolioVersion
	imp.AlertHistoryBaseVersion = serverlessAlertHistoryVersion
	if mode == "portfolio" {
		imp.AlertState = read(alertStateFile)
		imp.AlertHistory = read(alertHistoryFile)
	}
	if mode == "radar" {
		imp.LastPrices = read(lastPricesFile)
		imp.BuyAlertsState = read("buy_alerts_state.json")
	}
	b, _ := json.Marshal(imp)
	req, _ := http.NewRequest(http.MethodPost, api+"/internal/import", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("subiendo estado: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("import HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Los snapshots pueden pesar varios MB. Los subimos crudos a Workers KV en un
	// endpoint separado: evita el límite por fila de D1 y reduce CPU del Worker.
	if mode == "portfolio" {
		if err := uploadServerlessSnapshot(api, secret, "portfolio", filepath.Join(stateDir(), serverlessPortfolioSnapshot)); err != nil {
			return err
		}
	}
	if mode == "radar" {
		if err := uploadServerlessSnapshot(api, secret, "market", filepath.Join(stateDir(), serverlessMarketSnapshot)); err != nil {
			return err
		}
	}
	fmt.Printf("Estado sincronizado: modo=%s\n", mode)
	return nil
}

func uploadServerlessSnapshot(api, secret, kind, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("leyendo snapshot %s: %w", kind, err)
	}
	if len(b) > 24<<20 {
		return fmt.Errorf("snapshot %s demasiado grande: %.1f MiB", kind, float64(len(b))/(1024*1024))
	}
	req, _ := http.NewRequest(http.MethodPut, api+"/internal/snapshot/"+url.PathEscape(kind), bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("subiendo snapshot %s: %w", kind, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("snapshot %s HTTP %d: %s", kind, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func serverlessConfig() (string, string, error) {
	api := strings.TrimRight(strings.TrimSpace(os.Getenv("DASHBOARD_API_URL")), "/")
	secret := strings.TrimSpace(os.Getenv("SCANNER_SECRET"))
	if api == "" || secret == "" {
		return "", "", fmt.Errorf("faltan DASHBOARD_API_URL / SCANNER_SECRET")
	}
	return api, secret, nil
}
