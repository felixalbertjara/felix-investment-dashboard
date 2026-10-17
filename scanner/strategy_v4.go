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
	TradingScore           int      `json:"trading_score"`
	LongScore              int      `json:"long_score"`
	RecommendationV4       string   `json:"recommendation_v4"`
	StrongBuy              bool     `json:"strong_buy"`
	TradeSetup             string   `json:"trade_setup"`
	ExpectedWindow         string   `json:"expected_window"`
	ADX                    float64  `json:"adx"`
	RelativeStrength20     float64  `json:"relative_strength_20"`
	RelativeStrength60     float64  `json:"relative_strength_60"`
	BenchmarkTicker        string   `json:"benchmark_ticker"`
	MarketRegime           string   `json:"market_regime"`
	MarketRegimeScore      int      `json:"market_regime_score"`
	RewardRisk             float64  `json:"reward_risk"`
	ProjectedStop          float64  `json:"projected_stop"`
	ProjectedTarget        float64  `json:"projected_target"`
	ExtensionATR           float64  `json:"extension_atr"`
	EarningsRisk           bool     `json:"earnings_risk"`
	EarningsDays           int      `json:"earnings_days,omitempty"`
	CCLOnlyRisk            bool     `json:"ccl_only_risk"`
	LongDataQuality        string   `json:"long_data_quality"`
	VIXLevel               float64  `json:"vix_level"`
	VIXRegime              string   `json:"vix_regime"`
	Breakout20             bool     `json:"breakout_20"`
	Trend200Rising         bool     `json:"trend_200_rising"`
	HistoricalConfidence   string   `json:"historical_confidence"`
	HistoricalTrades       int      `json:"historical_trades"`
	HistoricalWinRate      float64  `json:"historical_win_rate"`
	HistoricalProfitFactor float64  `json:"historical_profit_factor"`
	HistoricalExpectancyR  float64  `json:"historical_expectancy_r"`
	HistoricalMaxDrawdown  float64  `json:"historical_max_drawdown"`
	V4Reasons              []string `json:"v4_reasons"`
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
	v4FactsMu         sync.Mutex
	v4FactsCache      = map[string]v4Fundamentals{}
	v5ValidationMu    sync.Mutex
	v5ValidationCache = map[string]v5ValidationCacheItem{}
	v5VIXMu           sync.Mutex
	v5VIXCachedAt     time.Time
	v5VIXLevel        float64
	v5VIXRegime       string
)

type v5Validation struct {
	Confidence   string
	Trades       int
	WinRate      float64
	ProfitFactor float64
	ExpectancyR  float64
	MaxDrawdown  float64
}

type v5ValidationCacheItem struct {
	Value     v5Validation
	FetchedAt time.Time
}

func evaluateV4(r Result) V4Analysis {
	out := V4Analysis{
		RecommendationV4:     "⚪ SIN DATOS",
		TradeSetup:           "SIN SETUP",
		ExpectedWindow:       "-",
		HistoricalConfidence: "NO EVALUADO",
		VIXRegime:            "SIN DATOS",
	}
	if r.Quote.Price <= 0 || len(r.Quote.Candles) < 60 {
		out.V4Reasons = []string{"Datos insuficientes para una evaluación FARO confiable."}
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
	out.VIXLevel, out.VIXRegime = v5VIXContext()

	// FARO: el núcleo del sistema es tendencia + momentum. Las señales aisladas
	// (RSI, velas, titulares) sólo contextualizan; no disparan una compra por sí solas.
	out.Trend200Rising = v5SMA200Rising(baseQ.Candles)
	baseSMA200 := v5SMA(baseQ.Candles, 200)
	longTrend := baseQ.Price > baseSMA200 && baseSMA200 > 0 && out.Trend200Rising
	priorHigh20 := v5PriorHigh(baseQ.Candles, 20)
	out.Breakout20 = priorHigh20 > 0 && baseQ.Price > priorHigh20*1.001

	trendAligned := r.Quote.Price > r.EMA20 && r.EMA20 > r.EMA50 && r.EMA50 > r.EMA200 && r.EMA200 > 0
	trendSlopes := v4EMASlopeUp(r.Quote.Candles, 20, 5) && v4EMASlopeUp(r.Quote.Candles, 50, 5)
	momentumHealthy := r.RSI >= 48 && r.RSI <= 74 && r.MACDHist > 0
	fastAligned := r.ScoreTiming >= 1 || (r.FastEMA20 > r.FastEMA50 && r.FastMACDHist > 0)

	out.ExtensionATR = 0
	if r.ATR > 0 && r.EMA20 > 0 && r.Quote.Price > r.EMA20 {
		out.ExtensionATR = (r.Quote.Price - r.EMA20) / r.ATR
	}
	notExtended := out.ExtensionATR <= 1.6

	prevSupport, prevResistance := v4PriorSupportResistance(r.Quote.Candles, 60)
	out.TradeSetup = v4TradeSetup(r, prevResistance, trendAligned, fastAligned)
	if out.Breakout20 && trendAligned && fastAligned && r.VolumeRatio >= 1.10 {
		out.TradeSetup = "BREAKOUT 20D FARO"
	}
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

	// CEDEAR: si la suba local parece explicada principalmente por CCL, se degrada.
	if r.Asset.Type == "CEDEAR" {
		local20 := v4ReturnPct(r.Quote.Candles, 20)
		under20 := v4ReturnPct(baseQ.Candles, 20)
		expectedLocal := under20 + r.CCLChange20Pct
		distortion := math.Abs(local20 - expectedLocal)
		out.CCLOnlyRisk = (local20 >= 4 && under20 <= 1 && r.CCLChange20Pct >= 2.5) || distortion >= 12
	}

	// FARO score 0-100. Trend y momentum tienen el mayor peso; RSI/Stoch son secundarios.
	score := 0
	if longTrend {
		score += 25
	} else if baseQ.Price > baseSMA200 && baseSMA200 > 0 {
		score += 10
	}
	if trendAligned {
		score += 10
	}
	if trendSlopes {
		score += 5
	}
	if out.Breakout20 {
		score += 15
	}
	switch {
	case out.ADX >= 30:
		score += 8
	case out.ADX >= 24:
		score += 6
	case out.ADX >= 20:
		score += 4
	}
	if r.MACDHist > 0 {
		score += 8
	}
	if r.RSI >= 50 && r.RSI <= 68 {
		score += 3
	} else if r.RSI > 75 {
		score -= 4
	}
	if fastAligned {
		score += 8
	}
	if out.RelativeStrength20 > 3 {
		score += 8
	} else if out.RelativeStrength20 > 0 {
		score += 5
	}
	if out.RelativeStrength60 > 5 {
		score += 6
	} else if out.RelativeStrength60 > 0 {
		score += 3
	}
	if r.VolumeRatio >= 1.50 {
		score += 8
	} else if r.VolumeRatio >= 1.10 {
		score += 6
	} else if r.VolumeRatio >= 0.80 {
		score += 2
	}
	if out.TradeSetup == "BREAKOUT 20D FARO" {
		score += 7
	} else if out.TradeSetup == "PULLBACK SANO" {
		score += 3
	}
	if !notExtended {
		score -= 10
	}
	if out.MarketRegimeScore >= 4 {
		score += 5
	} else if out.MarketRegimeScore <= 1 {
		score -= 7
	}
	// VIX es contexto, no gatillo: la propia investigación mostró que filtros de volatilidad
	// pueden proteger drawdown pero también recortar rentabilidad.
	switch {
	case out.VIXLevel > 0 && out.VIXLevel < 22:
		score += 2
	case out.VIXLevel >= 30 && out.VIXLevel < 40:
		score -= 2
	case out.VIXLevel >= 40:
		score -= 4
	}
	switch {
	case out.RewardRisk >= 3:
		score += 8
	case out.RewardRisk >= 2:
		score += 6
	case out.RewardRisk >= 1.5:
		score += 2
	default:
		score -= 8
	}
	if out.CCLOnlyRisk {
		score -= 15
	}
	if r.Quote.Error != "" {
		score -= 15
	}

	// Largo sigue siendo un motor separado.
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

	volumeGate := r.VolumeRatio >= 1.10
	cedearGate := true
	if r.Asset.Type == "CEDEAR" {
		cedearGate = r.UnderlyingScore >= 3 && r.UnderlyingRSI >= 42 && r.UnderlyingRSI <= 75 && !out.CCLOnlyRisk
	}
	qualityGate := r.Quote.Error == "" && len(r.Quote.Candles) >= 100

	// "Oportunidad jugosa" es deliberadamente rara: exige FARO breakout, tendencia,
	// momentum, fuerza relativa, R/R, confirmación 4H y luego validación histórica fuera de muestra.
	technicalStrong := out.TradingScore >= 90 && longTrend && trendAligned && trendSlopes && out.Breakout20 &&
		out.ADX >= 20 && momentumHealthy && fastAligned && out.RelativeStrength20 > 0 && out.RelativeStrength60 > 0 &&
		out.MarketRegimeScore >= 3 && volumeGate && notExtended && out.RewardRisk >= 2 &&
		!out.EarningsRisk && cedearGate && qualityGate

	if technicalStrong && strings.TrimSpace(factsTicker) != "" {
		val := v5ValidateFARO(factsTicker)
		out.HistoricalConfidence = val.Confidence
		out.HistoricalTrades = val.Trades
		out.HistoricalWinRate = val.WinRate
		out.HistoricalProfitFactor = val.ProfitFactor
		out.HistoricalExpectancyR = val.ExpectancyR
		out.HistoricalMaxDrawdown = val.MaxDrawdown
	} else if out.TradingScore >= 80 {
		out.HistoricalConfidence = "PENDIENTE"
	}

	out.StrongBuy = technicalStrong && out.HistoricalConfidence == "VALIDADO"
	out.ExpectedWindow = v4ExpectedWindow(out.TradeSetup, out.ADX, out.StrongBuy)
	out.RecommendationV4 = v4Recommendation(out)
	out.V4Reasons = v4Reasons(out, r, longTrend, trendSlopes, momentumHealthy, fastAligned, volumeGate, out.Breakout20, qualityGate)
	return out
}

func v4Recommendation(v V4Analysis) string {
	if v.StrongBuy && v.LongScore >= 78 {
		return "🔥 OPORTUNIDAD JUGOSA + APTO LARGO"
	}
	if v.StrongBuy {
		return "🔥 OPORTUNIDAD JUGOSA"
	}
	if v.TradingScore >= 82 && v.Breakout20 {
		return "🟢 TRADING - CERCA"
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
	if strong && (setup == "BREAKOUT CONFIRMADO" || setup == "BREAKOUT 20D FARO") && adx >= 30 {
		return "3–8 ruedas"
	}
	if strong && (setup == "BREAKOUT CONFIRMADO" || setup == "BREAKOUT 20D FARO") {
		return "4–10 ruedas"
	}
	if strong && setup == "PULLBACK SANO" {
		return "4–12 ruedas"
	}
	if setup == "BREAKOUT CONFIRMADO" || setup == "BREAKOUT 20D FARO" || setup == "PULLBACK SANO" {
		return "5–15 ruedas"
	}
	return "sin entrada inmediata"
}

func v4Reasons(v V4Analysis, r Result, longTrend, trendSlopes, momentumHealthy, fastAligned, volumeGate, breakoutGate, qualityGate bool) []string {
	x := []string{}
	if v.StrongBuy {
		x = append(x, fmt.Sprintf("FARO completo: Trading %d/100, ADX %.1f y R/R %.2f.", v.TradingScore, v.ADX, v.RewardRisk))
		x = append(x, fmt.Sprintf("Validación histórica: %s · %d trades OOS · PF %.2f · Exp %.2fR.", v.HistoricalConfidence, v.HistoricalTrades, v.HistoricalProfitFactor, v.HistoricalExpectancyR))
		x = append(x, fmt.Sprintf("Fuerza relativa vs %s: 20r %+0.1f%% · 60r %+0.1f%%.", v.BenchmarkTicker, v.RelativeStrength20, v.RelativeStrength60))
		return x
	}
	if !longTrend || !trendSlopes {
		x = append(x, "Tendencia FARO incompleta: exigimos precio sobre SMA200 ascendente y estructura de corto plazo sana.")
	}
	if !breakoutGate {
		x = append(x, "Todavía no rompió el máximo de 20 ruedas: sin breakout FARO no hay alerta jugosa.")
	}
	if v.ADX < 20 {
		x = append(x, fmt.Sprintf("ADX %.1f: tendencia con poca fuerza / riesgo de lateralidad.", v.ADX))
	}
	if !momentumHealthy || !fastAligned {
		x = append(x, "Momentum diario y/o 4H todavía no están completamente alineados.")
	}
	if v.RelativeStrength20 <= 0 || v.RelativeStrength60 <= 0 {
		x = append(x, fmt.Sprintf("No lidera con claridad a %s en 20 y 60 ruedas.", v.BenchmarkTicker))
	}
	if !volumeGate {
		x = append(x, "Volumen insuficiente para confirmar la ruptura.")
	}
	if v.ExtensionATR > 1.6 {
		x = append(x, fmt.Sprintf("Anti-FOMO: precio extendido %.1f ATR sobre EMA20.", v.ExtensionATR))
	}
	if v.RewardRisk < 2 {
		x = append(x, fmt.Sprintf("Riesgo/beneficio %.2f:1; para alerta jugosa exigimos al menos 2:1.", v.RewardRisk))
	}
	if v.MarketRegimeScore < 3 {
		x = append(x, "El mercado/benchmark no está suficientemente alineado.")
	}
	if v.EarningsRisk {
		x = append(x, fmt.Sprintf("Resultados próximos (~%d días): no iniciar una señal fuerte antes del evento.", v.EarningsDays))
	}
	if v.CCLOnlyRisk {
		x = append(x, "CEDEAR: parte importante de la suba puede venir del CCL y no del subyacente USA.")
	}
	if v.HistoricalConfidence == "DÉBIL" || v.HistoricalConfidence == "EXPERIMENTAL" || v.HistoricalConfidence == "SIN DATOS" {
		x = append(x, fmt.Sprintf("Validación histórica %s: no habilita Telegram de compra.", v.HistoricalConfidence))
	}
	if !qualityGate {
		x = append(x, "Calidad de datos insuficiente para disparar una oportunidad fuerte.")
	}
	if len(x) == 0 {
		x = append(x, fmt.Sprintf("Trading %d/100: buen candidato, pero todavía no pasa todos los filtros FARO.", v.TradingScore))
	}
	if len(x) > 7 {
		x = x[:7]
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

func v5SMA(c []Candle, period int) float64 {
	if period <= 0 || len(c) < period {
		return 0
	}
	s := 0.0
	for _, x := range c[len(c)-period:] {
		s += x.Close
	}
	return s / float64(period)
}

func v5SMA200Rising(c []Candle) bool {
	if len(c) < 225 {
		return false
	}
	now := v5SMA(c, 200)
	past := v5SMA(c[:len(c)-20], 200)
	return now > past && past > 0
}

func v5PriorHigh(c []Candle, lookback int) float64 {
	if lookback <= 0 || len(c) <= lookback {
		return 0
	}
	start := len(c) - 1 - lookback
	if start < 0 {
		start = 0
	}
	end := len(c) - 1
	h := 0.0
	for _, x := range c[start:end] {
		v := x.High
		if v <= 0 {
			v = x.Close
		}
		if v > h {
			h = v
		}
	}
	return h
}

func v5PriorLow(c []Candle, lookback int) float64 {
	if lookback <= 0 || len(c) <= lookback {
		return 0
	}
	start := len(c) - 1 - lookback
	if start < 0 {
		start = 0
	}
	end := len(c) - 1
	lo := 0.0
	for _, x := range c[start:end] {
		v := x.Low
		if v <= 0 {
			v = x.Close
		}
		if v > 0 && (lo == 0 || v < lo) {
			lo = v
		}
	}
	return lo
}

func v5VIXContext() (float64, string) {
	v5VIXMu.Lock()
	defer v5VIXMu.Unlock()
	if !v5VIXCachedAt.IsZero() && time.Since(v5VIXCachedAt) < 10*time.Minute {
		return v5VIXLevel, v5VIXRegime
	}
	q := fetchYahooChart("^VIX", "3mo", "1d")
	level := q.Price
	regime := "SIN DATOS"
	switch {
	case level > 0 && level < 20:
		regime = "CALMO"
	case level >= 20 && level < 30:
		regime = "ELEVADO"
	case level >= 30 && level < 40:
		regime = "ALTO"
	case level >= 40:
		regime = "EXTREMO"
	}
	v5VIXCachedAt = time.Now()
	v5VIXLevel = level
	v5VIXRegime = regime
	return level, regime
}

func v5ValidateFARO(ticker string) v5Validation {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return v5Validation{Confidence: "SIN DATOS"}
	}
	v5ValidationMu.Lock()
	if c, ok := v5ValidationCache[ticker]; ok && time.Since(c.FetchedAt) < 6*time.Hour {
		v := c.Value
		v5ValidationMu.Unlock()
		return v
	}
	v5ValidationMu.Unlock()

	q := fetchYahooChart(ticker, "5y", "1d")
	v := v5BacktestFARO(q.Candles)
	v5ValidationMu.Lock()
	v5ValidationCache[ticker] = v5ValidationCacheItem{Value: v, FetchedAt: time.Now()}
	v5ValidationMu.Unlock()
	return v
}

func v5BacktestFARO(c []Candle) v5Validation {
	out := v5Validation{Confidence: "SIN DATOS"}
	if len(c) < 500 {
		return out
	}
	// Se usa únicamente el 30% final como validación fuera de muestra.
	start := int(float64(len(c)) * 0.70)
	if start < 240 {
		start = 240
	}
	inTrade := false
	partial := false
	entry, stop, risk := 0.0, 0.0, 0.0
	entryIndex := 0
	realizedHalfR := 0.0
	tradeRs := []float64{}

	closeTrade := func(exit float64) {
		if !inTrade || risk <= 0 || exit <= 0 {
			return
		}
		r := (exit - entry) / risk
		if partial {
			r = realizedHalfR + 0.5*r
		}
		tradeRs = append(tradeRs, r)
		inTrade = false
		partial = false
		entry, stop, risk = 0, 0, 0
		realizedHalfR = 0
	}

	for i := start; i < len(c)-1; i++ {
		if !inTrade {
			sub := c[:i+1]
			sma200 := v5SMA(sub, 200)
			pastSMA := 0.0
			if len(sub) > 220 {
				pastSMA = v5SMA(sub[:len(sub)-20], 200)
			}
			priorHigh := v5PriorHigh(sub, 20)
			atr := calcATR(sub, 20)
			avgVol := 0.0
			if i >= 20 {
				for _, x := range c[i-20 : i] {
					avgVol += x.Volume
				}
				avgVol /= 20
			}
			volRatio := 0.0
			if avgVol > 0 {
				volRatio = c[i].Volume / avgVol
			}
			signal := sma200 > 0 && pastSMA > 0 && c[i].Close > sma200 && sma200 > pastSMA &&
				priorHigh > 0 && c[i].Close > priorHigh && atr > 0 && volRatio >= 1.10
			if !signal {
				continue
			}
			nextOpen := c[i+1].Open
			if nextOpen <= 0 {
				nextOpen = c[i+1].Close
			}
			// Anti-FOMO: si abre demasiado arriba del cierre de ruptura, se omite.
			if nextOpen <= 0 || nextOpen > c[i].Close+0.5*atr || nextOpen < priorHigh*0.995 {
				continue
			}
			entry = nextOpen
			risk = 2 * atr
			stop = entry - risk
			entryIndex = i + 1
			inTrade = true
			partial = false
			realizedHalfR = 0
			continue
		}

		if c[i].Low > 0 && c[i].Low <= stop {
			closeTrade(stop)
			continue
		}
		if !partial && c[i].High >= entry+2*risk {
			partial = true
			realizedHalfR = 1.0 // 50% realizado a +2R.
			if stop < entry {
				stop = entry
			}
		}
		low10 := v5PriorLow(c[:i+1], 10)
		if low10 > 0 && c[i].Close < low10 {
			closeTrade(c[i].Close)
			continue
		}
		if i-entryIndex >= 80 {
			closeTrade(c[i].Close)
			continue
		}
		if i == len(c)-2 {
			closeTrade(c[i].Close)
		}
	}

	out.Trades = len(tradeRs)
	if out.Trades == 0 {
		return out
	}
	wins := 0
	grossWin, grossLoss, sumR := 0.0, 0.0, 0.0
	equity, peak, maxDD := 1.0, 1.0, 0.0
	for _, r := range tradeRs {
		sumR += r
		if r > 0 {
			wins++
			grossWin += r
		} else {
			grossLoss += -r
		}
		// Drawdown comparable asumiendo 1% de riesgo de capital por operación.
		equity *= 1 + r*0.01
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := (peak - equity) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	out.WinRate = float64(wins) / float64(out.Trades) * 100
	out.ExpectancyR = sumR / float64(out.Trades)
	out.MaxDrawdown = maxDD
	if grossLoss > 0 {
		out.ProfitFactor = grossWin / grossLoss
	} else if grossWin > 0 {
		out.ProfitFactor = 9.99
	}
	switch {
	case out.Trades >= 6 && out.ProfitFactor >= 1.25 && out.ExpectancyR >= 0.10 && out.MaxDrawdown <= 12:
		out.Confidence = "VALIDADO"
	case out.Trades >= 4 && out.ProfitFactor >= 1.0 && out.ExpectancyR > 0:
		out.Confidence = "EXPERIMENTAL"
	case out.Trades >= 4:
		out.Confidence = "DÉBIL"
	default:
		out.Confidence = "SIN DATOS"
	}
	return out
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
