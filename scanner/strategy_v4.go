package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// V4Analysis agrega una capa selectiva encima del motor V2.
// La idea es que TRADING FUERTE sea deliberadamente raro: no alcanza con sumar puntos,
// también deben pasar todos los filtros críticos (tendencia, fuerza, timing, R/R, contexto, etc.).
type V4Analysis struct {
	TradingScore       int      `json:"trading_score"`
	LongScore          int      `json:"long_score"`
	RecommendationV4   string   `json:"recommendation_v4"`
	StrongBuy          bool     `json:"strong_buy"`
	TradeSetup         string   `json:"trade_setup"`
	ExpectedWindow     string   `json:"expected_window"`
	ADX                float64  `json:"adx"`
	RelativeStrength20 float64  `json:"relative_strength_20"`
	RelativeStrength60 float64  `json:"relative_strength_60"`
	BenchmarkTicker    string   `json:"benchmark_ticker"`
	MarketRegime       string   `json:"market_regime"`
	MarketRegimeScore  int      `json:"market_regime_score"`
	RewardRisk         float64  `json:"reward_risk"`
	ProjectedStop      float64  `json:"projected_stop"`
	ProjectedTarget    float64  `json:"projected_target"`
	ExtensionATR       float64  `json:"extension_atr"`
	EarningsRisk       bool     `json:"earnings_risk"`
	EarningsDays       int      `json:"earnings_days,omitempty"`
	CCLOnlyRisk        bool     `json:"ccl_only_risk"`
	LongDataQuality    string   `json:"long_data_quality"`
	V4Reasons          []string `json:"v4_reasons"`
}

type V4Result struct {
	Result
	V4Analysis
}

type v4Fundamentals struct {
	Ticker            string
	QuoteType         string
	MarketCap         float64
	TrailingPE        float64
	ForwardPE         float64
	EPSTrailing       float64
	EPSForward        float64
	PriceToBook       float64
	EarningsTimestamp int64
	FetchedAt         time.Time
	Valid             bool
}

var (
	v4FactsMu    sync.Mutex
	v4FactsCache = map[string]v4Fundamentals{}
)

func evaluateV4(r Result) V4Analysis {
	out := V4Analysis{RecommendationV4: "⚪ SIN DATOS", TradeSetup: "SIN SETUP", ExpectedWindow: "-"}
	if r.Quote.Price <= 0 || len(r.Quote.Candles) < 60 {
		out.V4Reasons = []string{"Datos insuficientes para una evaluación V4 confiable."}
		return out
	}

	baseTicker := r.Asset.YahooTicker
	baseQ := r.Quote
	if r.Asset.Type == "CEDEAR" {
		baseTicker = r.UnderlyingTicker
		if strings.TrimSpace(baseTicker) == "" {
			baseTicker = yahooUnderlyingTicker(r.Asset.Underlying)
		}
		if strings.TrimSpace(baseTicker) == "" {
			baseTicker = yahooUnderlyingTicker(guessUnderlying(r.Asset.Ticker))
		}
		if baseTicker != "" {
			q := fetchYahooChart(baseTicker, deepChartRange, deepChartInterval)
			if q.Price > 0 && len(q.Candles) >= 60 {
				baseQ = q
			}
		}
	}

	benchmark := v4Benchmark(r.Asset)
	benchQ := fetchYahooChart(benchmark, deepChartRange, deepChartInterval)
	out.BenchmarkTicker = benchmark
	out.MarketRegimeScore, out.MarketRegime = v4MarketRegime(benchQ)
	out.RelativeStrength20 = v4ReturnPct(baseQ.Candles, 20) - v4ReturnPct(benchQ.Candles, 20)
	out.RelativeStrength60 = v4ReturnPct(baseQ.Candles, 60) - v4ReturnPct(benchQ.Candles, 60)
	out.ADX = calcADXV4(r.Quote.Candles, 14)

	trendAligned := r.Quote.Price > r.EMA20 && r.EMA20 > r.EMA50 && r.EMA50 > r.EMA200 && r.EMA200 > 0
	trendSlopes := v4EMASlopeUp(r.Quote.Candles, 20, 5) && v4EMASlopeUp(r.Quote.Candles, 50, 5)
	momentumHealthy := r.RSI >= 48 && r.RSI <= 72 && r.MACDHist > 0
	fastAligned := r.ScoreTiming >= 1 || (r.FastEMA20 > r.FastEMA50 && r.FastMACDHist > 0)

	out.ExtensionATR = 0
	if r.ATR > 0 && r.EMA20 > 0 && r.Quote.Price > r.EMA20 {
		out.ExtensionATR = (r.Quote.Price - r.EMA20) / r.ATR
	}
	notExtended := out.ExtensionATR <= 1.8

	prevSupport, prevResistance := v4PriorSupportResistance(r.Quote.Candles, 60)
	out.TradeSetup = v4TradeSetup(r, prevResistance, trendAligned, fastAligned)
	out.ProjectedStop, _, _ = deriveInitialStop(r.Quote.Price, r, 0)
	out.ProjectedTarget = v4ProjectedTarget(r, out.TradeSetup, prevSupport, prevResistance)
	if risk := r.Quote.Price - out.ProjectedStop; risk > 0 && out.ProjectedTarget > r.Quote.Price {
		out.RewardRisk = (out.ProjectedTarget - r.Quote.Price) / risk
	}

	factsTicker := baseTicker
	if factsTicker == "" {
		factsTicker = r.Asset.YahooTicker
	}
	facts := v4Fundamentals{}

	// Detecta CEDEAR cuyo movimiento local parece explicarse principalmente por CCL.
	if r.Asset.Type == "CEDEAR" {
		local20 := v4ReturnPct(r.Quote.Candles, 20)
		under20 := v4ReturnPct(baseQ.Candles, 20)
		expectedLocal := under20 + r.CCLChange20Pct
		distortion := math.Abs(local20 - expectedLocal)
		out.CCLOnlyRisk = (local20 >= 4 && under20 <= 1 && r.CCLChange20Pct >= 2.5) || distortion >= 12
	}

	// Trading score 0-100. El score ordena candidatos, pero NO reemplaza los hard gates.
	score := 0
	if trendAligned {
		score += 20
	} else if r.Quote.Price > r.EMA50 && r.EMA50 > r.EMA200 && r.EMA200 > 0 {
		score += 10
	}
	if trendSlopes {
		score += 5
	}
	switch {
	case out.ADX >= 30:
		score += 15
	case out.ADX >= 25:
		score += 13
	case out.ADX >= 22:
		score += 10
	case out.ADX >= 18:
		score += 5
	}
	if r.MACDHist > 0 {
		score += 7
	}
	if r.RSI >= 50 && r.RSI <= 68 {
		score += 6
	} else if r.RSI >= 45 && r.RSI <= 72 {
		score += 3
	}
	if r.StochRSI > 0 && r.StochRSI < 85 {
		score += 2
	}
	if fastAligned {
		if r.ScoreTiming >= 2 {
			score += 10
		} else {
			score += 7
		}
	}
	if out.RelativeStrength20 > 3 {
		score += 8
	} else if out.RelativeStrength20 > 0 {
		score += 5
	}
	if out.RelativeStrength60 > 5 {
		score += 7
	} else if out.RelativeStrength60 > 0 {
		score += 4
	}
	if r.VolumeRatio >= 1.25 {
		score += 10
	} else if r.VolumeRatio >= 0.80 {
		score += 6
	} else if r.VolumeRatio >= 0.60 {
		score += 3
	}
	if out.TradeSetup == "BREAKOUT CONFIRMADO" || out.TradeSetup == "PULLBACK SANO" {
		score += 10
	}
	if !notExtended {
		score -= 12
	}
	if out.MarketRegimeScore >= 4 {
		score += 5
	} else if out.MarketRegimeScore <= 1 {
		score -= 8
	}
	switch {
	case out.RewardRisk >= 3:
		score += 10
	case out.RewardRisk >= 2:
		score += 7
	case out.RewardRisk >= 1.5:
		score += 3
	default:
		score -= 8
	}
	if out.CCLOnlyRisk {
		score -= 15
	}
	if r.Quote.Error != "" {
		score -= 15
	}

	// Primero calculamos largo con precio/tendencia. Sólo pedimos fundamentales/earnings
	// para candidatos razonables, reduciendo muchísimo llamadas a Yahoo y riesgo de rate-limit.
	out.LongScore, out.LongDataQuality = v4LongScore(r, baseQ, benchQ, facts)
	preTrading := clampInt(score, 0, 100)
	baseSym := strings.ToUpper(strings.TrimSpace(guessUnderlying(r.Asset.Ticker)))
	if preTrading >= 80 || out.LongScore >= 50 || v4BroadLongETF(baseSym) {
		facts = fetchV4Fundamentals(factsTicker)
		out.LongScore, out.LongDataQuality = v4LongScore(r, baseQ, benchQ, facts)
		out.EarningsDays, out.EarningsRisk = v4EarningsRisk(facts)
		if out.EarningsRisk {
			score -= 15
		}
	}
	out.TradingScore = clampInt(score, 0, 100)

	volumeGate := r.VolumeRatio >= 0.75
	if out.TradeSetup == "BREAKOUT CONFIRMADO" {
		volumeGate = r.VolumeRatio >= 1.10
	}
	cedearGate := true
	if r.Asset.Type == "CEDEAR" {
		cedearGate = r.UnderlyingScore >= 3 && r.UnderlyingRSI >= 42 && r.UnderlyingRSI <= 75 && !out.CCLOnlyRisk
	}
	setupGate := out.TradeSetup == "BREAKOUT CONFIRMADO" || out.TradeSetup == "PULLBACK SANO"
	qualityGate := r.Quote.Error == "" && len(r.Quote.Candles) >= 100

	out.StrongBuy = out.TradingScore >= 90 && trendAligned && trendSlopes && out.ADX >= 22 &&
		momentumHealthy && fastAligned && out.RelativeStrength20 > 0 && out.RelativeStrength60 > 0 &&
		out.MarketRegimeScore >= 4 && setupGate && volumeGate && notExtended && out.RewardRisk >= 2 &&
		!out.EarningsRisk && cedearGate && qualityGate

	out.ExpectedWindow = v4ExpectedWindow(out.TradeSetup, out.ADX, out.StrongBuy)
	out.RecommendationV4 = v4Recommendation(out)
	out.V4Reasons = v4Reasons(out, r, trendAligned, trendSlopes, momentumHealthy, fastAligned, volumeGate, setupGate, qualityGate)
	return out
}

func v4Recommendation(v V4Analysis) string {
	if v.StrongBuy && v.LongScore >= 78 {
		return "🔥 TRADING FUERTE + APTO LARGO"
	}
	if v.StrongBuy {
		return "🚀 TRADING FUERTE"
	}
	if v.TradingScore >= 78 && (v.TradeSetup == "BREAKOUT CONFIRMADO" || v.TradeSetup == "PULLBACK SANO") {
		return "🟢 TRADING"
	}
	if v.LongScore >= 78 {
		return "🏦 LARGO PLAZO"
	}
	if v.TradingScore >= 60 || v.LongScore >= 65 {
		return "🟡 ESPERAR"
	}
	return "⚪ NO COMPRAR"
}

func v4ExpectedWindow(setup string, adx float64, strong bool) string {
	if strong && setup == "BREAKOUT CONFIRMADO" && adx >= 30 {
		return "3–8 ruedas"
	}
	if strong && setup == "BREAKOUT CONFIRMADO" {
		return "4–10 ruedas"
	}
	if strong && setup == "PULLBACK SANO" {
		return "4–12 ruedas"
	}
	if setup == "BREAKOUT CONFIRMADO" || setup == "PULLBACK SANO" {
		return "5–15 ruedas"
	}
	return "sin entrada inmediata"
}

func v4Reasons(v V4Analysis, r Result, trendAligned, trendSlopes, momentumHealthy, fastAligned, volumeGate, setupGate, qualityGate bool) []string {
	x := []string{}
	if v.StrongBuy {
		x = append(x, fmt.Sprintf("Confluencia V4 completa: Trading %d/100, ADX %.1f y R/R %.2f.", v.TradingScore, v.ADX, v.RewardRisk))
		x = append(x, fmt.Sprintf("Fuerza relativa positiva vs %s: 20r %+0.1f%% · 60r %+0.1f%%.", v.BenchmarkTicker, v.RelativeStrength20, v.RelativeStrength60))
		x = append(x, "Diario + 4H + contexto de mercado alineados; señal deliberadamente rara.")
		return x
	}
	if !trendAligned || !trendSlopes {
		x = append(x, "No cumple todavía la tendencia perfecta y ascendente requerida para COMPRA FUERTE.")
	}
	if v.ADX < 22 {
		x = append(x, fmt.Sprintf("ADX %.1f: tendencia con poca fuerza / riesgo de lateralidad.", v.ADX))
	}
	if !momentumHealthy || !fastAligned {
		x = append(x, "Momentum diario y/o 4H todavía no están completamente alineados.")
	}
	if v.RelativeStrength20 <= 0 || v.RelativeStrength60 <= 0 {
		x = append(x, fmt.Sprintf("No lidera con claridad a %s en 20 y 60 ruedas.", v.BenchmarkTicker))
	}
	if !setupGate {
		x = append(x, "No hay breakout confirmado ni pullback sano para entrar ahora.")
	}
	if !volumeGate {
		x = append(x, "Volumen insuficiente para validar la entrada.")
	}
	if v.ExtensionATR > 1.8 {
		x = append(x, fmt.Sprintf("Anti-FOMO: precio extendido %.1f ATR sobre EMA20.", v.ExtensionATR))
	}
	if v.RewardRisk < 2 {
		x = append(x, fmt.Sprintf("Riesgo/beneficio %.2f:1; para señal fuerte exigimos al menos 2:1.", v.RewardRisk))
	}
	if v.MarketRegimeScore < 4 {
		x = append(x, "El mercado/benchmark no está suficientemente alineado para una señal excepcional.")
	}
	if v.EarningsRisk {
		x = append(x, fmt.Sprintf("Evento de resultados próximo (~%d días): no iniciar un swing fuerte antes del evento.", v.EarningsDays))
	}
	if v.CCLOnlyRisk {
		x = append(x, "CEDEAR: parte importante de la suba puede venir del CCL y no del subyacente USA.")
	}
	if !qualityGate {
		x = append(x, "Calidad de datos insuficiente para disparar una COMPRA FUERTE.")
	}
	if len(x) == 0 {
		x = append(x, fmt.Sprintf("Trading %d/100: buen candidato, pero no pasa todos los filtros críticos.", v.TradingScore))
	}
	if len(x) > 6 {
		x = x[:6]
	}
	return x
}

func v4Benchmark(a Asset) string {
	if a.Type == "ACCION" {
		return "^MERV"
	}
	s := strings.ToLower(a.Sector)
	switch {
	case strings.Contains(s, "semiconductor"):
		return "SMH"
	case strings.Contains(s, "tecnolog"):
		return "QQQ"
	case strings.Contains(s, "financier") || strings.Contains(s, "banco"):
		return "XLF"
	case strings.Contains(s, "energ"):
		return "XLE"
	case strings.Contains(s, "salud") || strings.Contains(s, "pharma"):
		return "XLV"
	case strings.Contains(s, "consumo") || strings.Contains(s, "e-commerce") || strings.Contains(s, "autos"):
		return "XLY"
	case strings.Contains(s, "aerol") || strings.Contains(s, "defensa"):
		return "XLI"
	default:
		return "SPY"
	}
}

func v4MarketRegime(q Quote) (int, string) {
	if q.Price <= 0 || len(q.Candles) < 200 {
		return 0, "SIN DATOS"
	}
	e50, e200 := calcEMA(q.Candles, 50), calcEMA(q.Candles, 200)
	_, _, hist := calcMACD(q.Candles)
	s := 0
	if q.Price > e50 {
		s += 2
	}
	if e50 > e200 && e200 > 0 {
		s += 2
	}
	if hist > 0 {
		s++
	}
	switch {
	case s >= 4:
		return s, "FAVORABLE"
	case s >= 2:
		return s, "NEUTRAL"
	default:
		return s, "DÉBIL"
	}
}

func v4TradeSetup(r Result, priorResistance float64, trendAligned, fastAligned bool) string {
	if !trendAligned || r.ATR <= 0 {
		return "SIN SETUP"
	}
	if priorResistance > 0 && r.Quote.Price > priorResistance*1.003 && r.VolumeRatio >= 1.10 && fastAligned {
		return "BREAKOUT CONFIRMADO"
	}
	if r.EMA20 > 0 && math.Abs(r.Quote.Price-r.EMA20) <= r.ATR*1.05 && r.Quote.Price >= r.EMA20*0.985 && r.MACDHist > 0 && fastAligned {
		return "PULLBACK SANO"
	}
	return "TENDENCIA SIN ENTRADA"
}

func v4ProjectedTarget(r Result, setup string, priorSupport, priorResistance float64) float64 {
	price := r.Quote.Price
	if price <= 0 {
		return 0
	}
	if setup == "PULLBACK SANO" && priorResistance > price*1.01 {
		return priorResistance
	}
	if setup == "BREAKOUT CONFIRMADO" {
		rangeHeight := priorResistance - priorSupport
		move := 0.0
		if r.ATR > 0 {
			move = r.ATR * 2.5
		}
		if rangeHeight > 0 {
			if move <= 0 || rangeHeight > move {
				move = rangeHeight
			}
			if r.ATR > 0 {
				move = math.Min(move, r.ATR*5)
			}
		}
		if move > 0 {
			return price + move
		}
	}
	if priorResistance > price*1.01 {
		return priorResistance
	}
	if r.ATR > 0 {
		return price + r.ATR*2.5
	}
	return 0
}

func v4PriorSupportResistance(c []Candle, lookback int) (float64, float64) {
	if len(c) < 3 {
		return 0, 0
	}
	end := len(c) - 1 // excluye la vela actual para detectar ruptura real
	start := end - lookback
	if start < 0 {
		start = 0
	}
	return supportResistance(c[start:end], lookback)
}

func v4ReturnPct(c []Candle, sessions int) float64 {
	if sessions <= 0 || len(c) <= sessions {
		return 0
	}
	cur := c[len(c)-1].Close
	old := c[len(c)-1-sessions].Close
	if cur <= 0 || old <= 0 {
		return 0
	}
	return (cur - old) / old * 100
}

func v4EMASlopeUp(c []Candle, period, back int) bool {
	if len(c) < period+back+2 {
		return false
	}
	now := calcEMA(c, period)
	past := calcEMA(c[:len(c)-back], period)
	return now > past && past > 0
}

func calcADXV4(c []Candle, period int) float64 {
	if period < 2 || len(c) < period*2+2 {
		return 0
	}
	trs := make([]float64, 0, len(c)-1)
	plus := make([]float64, 0, len(c)-1)
	minus := make([]float64, 0, len(c)-1)
	for i := 1; i < len(c); i++ {
		h, l, pc := c[i].High, c[i].Low, c[i-1].Close
		tr := math.Max(h-l, math.Max(math.Abs(h-pc), math.Abs(l-pc)))
		up := c[i].High - c[i-1].High
		down := c[i-1].Low - c[i].Low
		pdm, mdm := 0.0, 0.0
		if up > down && up > 0 {
			pdm = up
		}
		if down > up && down > 0 {
			mdm = down
		}
		trs = append(trs, tr)
		plus = append(plus, pdm)
		minus = append(minus, mdm)
	}
	atr, psm, msm := 0.0, 0.0, 0.0
	for i := 0; i < period; i++ {
		atr += trs[i]
		psm += plus[i]
		msm += minus[i]
	}
	dx := []float64{}
	for i := period; i < len(trs); i++ {
		atr = atr - atr/float64(period) + trs[i]
		psm = psm - psm/float64(period) + plus[i]
		msm = msm - msm/float64(period) + minus[i]
		if atr <= 0 {
			continue
		}
		pdi := 100 * psm / atr
		mdi := 100 * msm / atr
		den := pdi + mdi
		if den <= 0 {
			continue
		}
		dx = append(dx, 100*math.Abs(pdi-mdi)/den)
	}
	if len(dx) == 0 {
		return 0
	}
	start := len(dx) - period
	if start < 0 {
		start = 0
	}
	s := 0.0
	for _, v := range dx[start:] {
		s += v
	}
	return s / float64(len(dx[start:]))
}

func v4LongScore(r Result, baseQ, benchQ Quote, f v4Fundamentals) (int, string) {
	if baseQ.Price <= 0 || len(baseQ.Candles) < 200 {
		return 0, "datos insuficientes"
	}
	e50, e200 := calcEMA(baseQ.Candles, 50), calcEMA(baseQ.Candles, 200)
	ret120, ret240 := v4ReturnPct(baseQ.Candles, 120), v4ReturnPct(baseQ.Candles, 240)
	rs60 := v4ReturnPct(baseQ.Candles, 60) - v4ReturnPct(benchQ.Candles, 60)
	high := v4HighestHigh(baseQ.Candles, 252)
	dd := 100.0
	if high > 0 {
		dd = (high - baseQ.Price) / high * 100
	}

	score := 0
	if baseQ.Price > e200 && e200 > 0 {
		score += 15
	}
	if e50 > e200 && e200 > 0 {
		score += 10
	}
	if ret120 > 0 {
		score += 8
		if ret120 >= 10 {
			score += 4
		}
	}
	if ret240 > 0 {
		score += 8
		if ret240 >= 15 {
			score += 5
		}
	}
	if dd <= 20 {
		score += 5
		if dd <= 10 {
			score += 5
		}
	}
	if rs60 > -2 {
		score += 5
	}

	base := strings.ToUpper(strings.TrimSpace(r.UnderlyingTicker))
	if base == "" {
		base = strings.ToUpper(strings.TrimSpace(guessUnderlying(r.Asset.Ticker)))
	}
	if v4BroadLongETF(base) {
		score += 28
		return clampInt(score, 0, 100), "ETF amplio + tendencia de largo plazo"
	}
	if isLeveragedTicker(base) || base == "VXX" {
		score -= 25
	}

	fundPts := 0
	if f.Valid {
		if f.EPSTrailing > 0 {
			fundPts += 8
		}
		if f.EPSForward > f.EPSTrailing && f.EPSForward > 0 {
			fundPts += 8
		} else if f.EPSForward > 0 {
			fundPts += 4
		}
		if f.TrailingPE > 0 && f.TrailingPE <= 60 {
			fundPts += 8
		} else if f.TrailingPE > 60 && f.TrailingPE <= 100 {
			fundPts += 4
		}
		if f.MarketCap >= 10_000_000_000 {
			fundPts += 8
		} else if f.MarketCap >= 2_000_000_000 {
			fundPts += 5
		} else if f.MarketCap >= 500_000_000 {
			fundPts += 2
		}
		if f.ForwardPE > 0 && f.TrailingPE > 0 && f.ForwardPE < f.TrailingPE {
			fundPts += 4
		}
		if f.PriceToBook > 0 && f.PriceToBook <= 15 {
			fundPts += 4
		}
		score += clampInt(fundPts, 0, 40)
		return clampInt(score, 0, 100), "tendencia + fundamentales básicos Yahoo"
	}
	if score > 70 {
		score = 70 // sin fundamentales no etiquetamos una acción como gran inversión de largo plazo
	}
	return clampInt(score, 0, 100), "técnico; fundamentales no disponibles"
}

func v4HighestHigh(c []Candle, n int) float64 {
	if len(c) == 0 {
		return 0
	}
	start := len(c) - n
	if start < 0 {
		start = 0
	}
	h := 0.0
	for _, x := range c[start:] {
		if x.High > h {
			h = x.High
		}
	}
	return h
}

func v4BroadLongETF(t string) bool {
	t = strings.ToUpper(strings.TrimSpace(t))
	switch t {
	case "SPY", "IVV", "ACWI", "VEA", "VIG", "QQQ", "IWM", "EEM", "EFA":
		return true
	default:
		return false
	}
}

func fetchV4Fundamentals(ticker string) v4Fundamentals {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return v4Fundamentals{}
	}
	v4FactsMu.Lock()
	if f, ok := v4FactsCache[ticker]; ok && time.Since(f.FetchedAt) < 6*time.Hour {
		v4FactsMu.Unlock()
		return f
	}
	v4FactsMu.Unlock()

	u := "https://query1.finance.yahoo.com/v7/finance/quote?symbols=" + url.QueryEscape(ticker)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 FelixDashboard/4.0")
	resp, err := client.Do(req)
	if err != nil {
		return v4Fundamentals{Ticker: ticker, FetchedAt: time.Now()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return v4Fundamentals{Ticker: ticker, FetchedAt: time.Now()}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	var data struct {
		QuoteResponse struct {
			Result []struct {
				Symbol                 string  `json:"symbol"`
				QuoteType              string  `json:"quoteType"`
				MarketCap              float64 `json:"marketCap"`
				TrailingPE             float64 `json:"trailingPE"`
				ForwardPE              float64 `json:"forwardPE"`
				EPSTrailing            float64 `json:"epsTrailingTwelveMonths"`
				EPSForward             float64 `json:"epsForward"`
				PriceToBook            float64 `json:"priceToBook"`
				EarningsTimestampStart int64   `json:"earningsTimestampStart"`
				EarningsTimestamp      int64   `json:"earningsTimestamp"`
			} `json:"result"`
		} `json:"quoteResponse"`
	}
	f := v4Fundamentals{Ticker: ticker, FetchedAt: time.Now()}
	if json.Unmarshal(body, &data) == nil && len(data.QuoteResponse.Result) > 0 {
		x := data.QuoteResponse.Result[0]
		ets := x.EarningsTimestampStart
		if ets == 0 {
			ets = x.EarningsTimestamp
		}
		f = v4Fundamentals{Ticker: ticker, QuoteType: x.QuoteType, MarketCap: x.MarketCap, TrailingPE: x.TrailingPE, ForwardPE: x.ForwardPE, EPSTrailing: x.EPSTrailing, EPSForward: x.EPSForward, PriceToBook: x.PriceToBook, EarningsTimestamp: ets, FetchedAt: time.Now(), Valid: true}
	}
	v4FactsMu.Lock()
	v4FactsCache[ticker] = f
	v4FactsMu.Unlock()
	return f
}

func v4EarningsRisk(f v4Fundamentals) (int, bool) {
	if f.EarningsTimestamp <= 0 {
		return 0, false
	}
	e := time.Unix(f.EarningsTimestamp, 0)
	d := int(math.Ceil(e.Sub(time.Now()).Hours() / 24))
	if d >= 0 && d <= 7 {
		return d, true
	}
	return d, false
}
