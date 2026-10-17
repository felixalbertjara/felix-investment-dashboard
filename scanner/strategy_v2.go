package main

import (
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// V2 agrega gestión profesional de swing trading sin automatizar operaciones.
// El objetivo es separar: calidad de entrada, riesgo inicial, evolución del trade,
// velocidad del retorno, tiempo consumido y salida por invalidación / trailing / toma.

const (
	defaultSwingDays    = 15
	defaultPositionDays = 120
)

var knownUnderlyingTickersV2 = map[string]bool{
	"AAL":   true,
	"AAPL":  true,
	"ABBV":  true,
	"ABT":   true,
	"ACWI":  true,
	"ADBE":  true,
	"ADI":   true,
	"AMAT":  true,
	"AMD":   true,
	"AMZN":  true,
	"ARKK":  true,
	"ASML":  true,
	"AVGO":  true,
	"AXP":   true,
	"AZN":   true,
	"BA":    true,
	"BABA":  true,
	"BAC":   true,
	"BK":    true,
	"BMY":   true,
	"BP":    true,
	"C":     true,
	"CIBR":  true,
	"COIN":  true,
	"COST":  true,
	"CRM":   true,
	"CVX":   true,
	"DAL":   true,
	"DIA":   true,
	"DOCU":  true,
	"EBAY":  true,
	"EEM":   true,
	"ETHA":  true,
	"ETSY":  true,
	"EWZ":   true,
	"F":     true,
	"GILD":  true,
	"GLD":   true,
	"GM":    true,
	"GOOG":  true,
	"GOOGL": true,
	"GS":    true,
	"HAL":   true,
	"HMC":   true,
	"HUT":   true,
	"IBB":   true,
	"IBIT":  true,
	"INTC":  true,
	"ITA":   true,
	"IVE":   true,
	"IVV":   true,
	"IVW":   true,
	"IWM":   true,
	"JNJ":   true,
	"JPM":   true,
	"KO":    true,
	"LLY":   true,
	"LMT":   true,
	"LRCX":  true,
	"MA":    true,
	"MBG":   true,
	"MCD":   true,
	"MDLZ":  true,
	"MDT":   true,
	"MELI":  true,
	"META":  true,
	"MRK":   true,
	"MS":    true,
	"MSFT":  true,
	"MSTR":  true,
	"MU":    true,
	"NFLX":  true,
	"NKE":   true,
	"NOW":   true,
	"NVDA":  true,
	"NVS":   true,
	"ORCL":  true,
	"OXY":   true,
	"PDD":   true,
	"PEP":   true,
	"PFE":   true,
	"PG":    true,
	"PLTR":  true,
	"PYPL":  true,
	"QCOM":  true,
	"QQQ":   true,
	"RIOT":  true,
	"RTX":   true,
	"SBUX":  true,
	"SCHW":  true,
	"SE":    true,
	"SHEL":  true,
	"SHOP":  true,
	"SLB":   true,
	"SLV":   true,
	"SMH":   true,
	"SNOW":  true,
	"SPOT":  true,
	"SPXL":  true,
	"SPY":   true,
	"TEAM":  true,
	"TM":    true,
	"TQQQ":  true,
	"TSLA":  true,
	"TSM":   true,
	"TTE":   true,
	"TXN":   true,
	"UAL":   true,
	"UBER":  true,
	"UNH":   true,
	"URA":   true,
	"USO":   true,
	"V":     true,
	"VEA":   true,
	"VIG":   true,
	"VXX":   true,
	"WFC":   true,
	"WMT":   true,
	"XLE":   true,
	"XLF":   true,
	"XLI":   true,
	"XLK":   true,
	"XLP":   true,
	"XLU":   true,
	"XLV":   true,
	"XLY":   true,
	"XOM":   true,
}

func isKnownUnderlyingTickerV2(t string) bool {
	return knownUnderlyingTickersV2[strings.ToUpper(strings.TrimSpace(t))]
}

type TechnicalBreakdown struct {
	Trend    int
	Momentum int
	Volume   int
	Location int
	Timing   int
	Total    int
}

type CCLContext struct {
	Value       float64
	Change20Pct float64
	Source      string
	UpdatedAt   time.Time
}

var (
	cclMu    sync.Mutex
	cclCache CCLContext
)

type chartCacheItem struct {
	Quote Quote
	At    time.Time
}

var (
	chartCacheMu sync.Mutex
	chartCacheV2 = map[string]chartCacheItem{}
)

func fetchYahooChart(ticker, period, interval string) Quote {
	key := strings.ToUpper(strings.TrimSpace(ticker)) + "|" + period + "|" + interval
	chartCacheMu.Lock()
	if item, ok := chartCacheV2[key]; ok && time.Since(item.At) < 4*time.Minute {
		q := item.Quote
		chartCacheMu.Unlock()
		return q
	}
	chartCacheMu.Unlock()
	q := fetchYahooChartRaw(ticker, period, interval)
	// No cacheamos errores sin precio para permitir reintento rápido.
	if q.Price > 0 {
		chartCacheMu.Lock()
		chartCacheV2[key] = chartCacheItem{Quote: q, At: time.Now()}
		chartCacheMu.Unlock()
	}
	return q
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func technicalScoreV2(q Quote, rsi, stoch, ema20, ema50, ema200, macd, sig, hist, atr, volumeRatio, support, resistance float64, fast *FastMetrics) (TechnicalBreakdown, []string) {
	var b TechnicalBreakdown
	reasons := []string{}
	price := q.Price
	if price <= 0 {
		b.Total = -6
		return b, []string{"No hay precio válido desde Yahoo; no conviene tomar decisión."}
	}

	// TENDENCIA: máximo +/-4. Evita contar la misma información 2 o 3 veces.
	if ema200 > 0 {
		if price > ema200 {
			b.Trend += 2
			reasons = append(reasons, "Tendencia: precio arriba de EMA 200.")
		} else {
			b.Trend -= 2
			reasons = append(reasons, "Tendencia: precio debajo de EMA 200.")
		}
	}
	if ema20 > 0 && ema50 > 0 {
		if ema20 > ema50 {
			b.Trend += 1
			reasons = append(reasons, "Tendencia: EMA 20 arriba de EMA 50.")
		} else {
			b.Trend -= 1
			reasons = append(reasons, "Tendencia: EMA 20 debajo de EMA 50.")
		}
	}
	if ema50 > 0 && ema200 > 0 {
		if ema50 > ema200 {
			b.Trend += 1
			reasons = append(reasons, "Tendencia: EMA 50 arriba de EMA 200.")
		} else {
			b.Trend -= 1
			reasons = append(reasons, "Tendencia: EMA 50 debajo de EMA 200.")
		}
	}
	b.Trend = clampInt(b.Trend, -4, 4)

	// MOMENTUM: máximo +/-3.
	if rsi > 0 {
		switch {
		case rsi >= 82:
			b.Momentum -= 2
			reasons = append(reasons, "Momentum: RSI extremadamente alto; riesgo de sobreextensión.")
		case rsi >= 72:
			b.Momentum -= 1
			reasons = append(reasons, "Momentum: RSI alto; evitar perseguir precio.")
		case rsi >= 45 && rsi <= 65:
			b.Momentum += 1
			reasons = append(reasons, "Momentum: RSI en zona sana.")
		case rsi < 30:
			reasons = append(reasons, "Momentum: RSI sobrevendido; no se premia sin confirmación.")
		}
	}
	if macd != 0 || sig != 0 {
		if macd > sig && hist > 0 {
			b.Momentum += 2
			reasons = append(reasons, "Momentum: MACD comprador con histograma positivo.")
		} else if macd < sig && hist < 0 {
			b.Momentum -= 2
			reasons = append(reasons, "Momentum: MACD vendedor con histograma negativo.")
		}
	}
	if stoch > 85 && rsi > 70 {
		b.Momentum -= 1
		reasons = append(reasons, "Momentum: Stoch RSI y RSI altos al mismo tiempo.")
	}
	b.Momentum = clampInt(b.Momentum, -3, 3)

	// VOLUMEN: máximo +/-2.
	if volumeRatio >= 1.25 && q.PreviousClose > 0 {
		if price >= q.PreviousClose {
			b.Volume = 2
			reasons = append(reasons, "Volumen: suba/estabilidad con volumen superior al promedio.")
		} else {
			b.Volume = -2
			reasons = append(reasons, "Volumen: caída con volumen superior al promedio.")
		}
	} else if volumeRatio > 0 && volumeRatio < 0.45 {
		b.Volume = -1
		reasons = append(reasons, "Volumen: participación muy baja; señal menos confiable.")
	}

	// UBICACIÓN: máximo +/-2.
	if support > 0 && price >= support {
		ds := (price - support) / price * 100
		if ds <= 4 {
			b.Location += 2
			reasons = append(reasons, "Zona: precio cerca de soporte.")
		}
	}
	if resistance > 0 {
		dr := (resistance - price) / price * 100
		if dr >= 0 && dr <= 3 {
			b.Location -= 2
			reasons = append(reasons, "Zona: precio muy cerca de resistencia.")
		} else if price > resistance*1.005 && volumeRatio >= 1.2 {
			b.Location += 2
			reasons = append(reasons, "Zona: ruptura de resistencia acompañada por volumen.")
		}
	}
	b.Location = clampInt(b.Location, -2, 2)

	// TIMING 4H: máximo +/-2. Se usa para afinar entrada/salida, no para reemplazar el diario.
	if fast != nil && fast.Valid {
		if fast.EMA20 > 0 && fast.EMA50 > 0 {
			if fast.EMA20 > fast.EMA50 {
				b.Timing++
			} else {
				b.Timing--
			}
		}
		if fast.MACD > fast.MACDSignal && fast.MACDHist > 0 {
			b.Timing++
		}
		if fast.MACD < fast.MACDSignal && fast.MACDHist < 0 {
			b.Timing--
		}
		if fast.RSI >= 78 {
			b.Timing--
		}
		b.Timing = clampInt(b.Timing, -2, 2)
		if b.Timing >= 2 {
			reasons = append(reasons, "Timing 4H: confirma momentum alcista.")
		} else if b.Timing <= -2 {
			reasons = append(reasons, "Timing 4H: deterioro claro; conviene esperar o proteger.")
		}
	}

	if atr > 0 && price > 0 && (atr/price*100) > 8 {
		reasons = append(reasons, "Volatilidad: ATR alto; usar tamaño de posición y stop prudentes.")
	}

	b.Total = b.Trend + b.Momentum + b.Volume + b.Location + b.Timing
	return b, uniqueStrings(reasons)
}

type FastMetrics struct {
	Valid      bool
	RSI        float64
	EMA20      float64
	EMA50      float64
	MACD       float64
	MACDSignal float64
	MACDHist   float64
	Score      int
}

func fastMetricsForTicker(ticker string) FastMetrics {
	q := fetchYahooChart(ticker, "3mo", "1h")
	if len(q.Candles) < 60 {
		return FastMetrics{}
	}
	c := aggregateCandles(q.Candles, 4)
	if len(c) < 30 {
		return FastMetrics{}
	}
	f := FastMetrics{Valid: true}
	f.RSI = calcRSI(c, 14)
	f.EMA20 = calcEMA(c, 20)
	f.EMA50 = calcEMA(c, 50)
	f.MACD, f.MACDSignal, f.MACDHist = calcMACD(c)
	if f.EMA20 > f.EMA50 {
		f.Score++
	} else {
		f.Score--
	}
	if f.MACD > f.MACDSignal && f.MACDHist > 0 {
		f.Score++
	}
	if f.MACD < f.MACDSignal && f.MACDHist < 0 {
		f.Score--
	}
	if f.RSI >= 78 {
		f.Score--
	}
	f.Score = clampInt(f.Score, -2, 2)
	return f
}

func aggregateCandles(in []Candle, group int) []Candle {
	if group <= 1 {
		return append([]Candle(nil), in...)
	}
	out := []Candle{}
	for i := 0; i < len(in); i += group {
		end := i + group
		if end > len(in) {
			end = len(in)
		}
		chunk := in[i:end]
		if len(chunk) == 0 {
			continue
		}
		c := Candle{Time: chunk[0].Time, Open: chunk[0].Open, High: chunk[0].High, Low: chunk[0].Low, Close: chunk[len(chunk)-1].Close}
		if c.Open == 0 {
			c.Open = chunk[0].Close
		}
		if c.High == 0 {
			c.High = chunk[0].Close
		}
		if c.Low == 0 {
			c.Low = chunk[0].Close
		}
		for _, x := range chunk {
			if x.High > c.High {
				c.High = x.High
			}
			if x.Low > 0 && (c.Low == 0 || x.Low < c.Low) {
				c.Low = x.Low
			}
			c.Volume += x.Volume
		}
		out = append(out, c)
	}
	return out
}

func yahooUnderlyingTicker(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	m := map[string]string{
		"BRKB":  "BRK-B",
		"BRKBC": "BRK-B",
		"BRKBD": "BRK-B",
		"BABA":  "BABA",
		"GOOGL": "GOOGL",
		"GOOG":  "GOOG",
	}
	if v, ok := m[t]; ok {
		return v
	}
	return t
}

func cclContext() CCLContext {
	cclMu.Lock()
	defer cclMu.Unlock()
	if cclCache.Value > 0 && time.Since(cclCache.UpdatedAt) < 10*time.Minute {
		return cclCache
	}
	// Proxy de CCL: YPFD local / ADR YPF. El ADR representa una acción clase D.
	local := fetchYahooChart("YPFD.BA", "3mo", "1d")
	usd := fetchYahooChart("YPF", "3mo", "1d")
	ctx := deriveRatioContext(local, usd, 1.0, "Proxy YPFD.BA / YPF")
	if ctx.Value <= 0 {
		// Fallback aproximado: GGAL ADR representa 10 acciones locales.
		gl := fetchYahooChart("GGAL.BA", "3mo", "1d")
		gu := fetchYahooChart("GGAL", "3mo", "1d")
		ctx = deriveRatioContext(gl, gu, 10.0, "Proxy GGAL.BA x10 / GGAL")
	}
	ctx.UpdatedAt = time.Now()
	cclCache = ctx
	return ctx
}

func deriveRatioContext(local, usd Quote, adrRatio float64, source string) CCLContext {
	out := CCLContext{Source: source}
	if local.Price <= 0 || usd.Price <= 0 {
		return out
	}
	out.Value = local.Price * adrRatio / usd.Price
	lm := map[string]float64{}
	for _, c := range local.Candles {
		lm[c.Time.Format("2006-01-02")] = c.Close
	}
	ratios := []float64{}
	for _, c := range usd.Candles {
		if c.Close <= 0 {
			continue
		}
		if lp, ok := lm[c.Time.Format("2006-01-02")]; ok && lp > 0 {
			ratios = append(ratios, lp*adrRatio/c.Close)
		}
	}
	if len(ratios) >= 21 {
		old := ratios[len(ratios)-21]
		cur := ratios[len(ratios)-1]
		if old > 0 {
			out.Change20Pct = (cur - old) / old * 100
		}
	}
	return out
}

func enrichResultV2(a Asset, r *Result, force bool) {
	if r == nil || r.Quote.Price <= 0 {
		return
	}

	// Solo gastamos requests de intradía si la señal diaria merece atención o si está en cartera.
	if force || math.Abs(float64(r.ScoreTechnical)) >= 3 {
		f := fastMetricsForTicker(a.YahooTicker)
		r.FastRSI, r.FastEMA20, r.FastEMA50 = f.RSI, f.EMA20, f.EMA50
		r.FastMACD, r.FastMACDSignal, r.FastMACDHist = f.MACD, f.MACDSignal, f.MACDHist
		r.ScoreTiming = f.Score
		if f.Valid {
			// Recalcular el técnico incluyendo timing, conservando las familias diarias.
			b, rs := technicalScoreV2(r.Quote, r.RSI, r.StochRSI, r.EMA20, r.EMA50, r.EMA200, r.MACD, r.MACDSignal, r.MACDHist, r.ATR, r.VolumeRatio, r.Support, r.Resistance, &f)
			r.ScoreTrend, r.ScoreMomentum, r.ScoreVolume, r.ScoreLocation, r.ScoreTiming, r.ScoreTechnical = b.Trend, b.Momentum, b.Volume, b.Location, b.Timing, b.Total
			r.Reasons = append(r.Reasons, rs...)
		}
	}

	if a.Type != "CEDEAR" {
		return
	}
	under := yahooUnderlyingTicker(a.Underlying)
	if under == "" {
		under = yahooUnderlyingTicker(guessUnderlying(a.Ticker))
	}
	r.UnderlyingTicker = under

	if force || r.ScoreTechnical >= 3 || r.ScoreTechnical <= -3 {
		uq := fetchYahooChart(under, deepChartRange, deepChartInterval)
		if uq.Price > 0 && len(uq.Candles) >= 60 {
			ur := calcRSI(uq.Candles, 14)
			ue20 := calcEMA(uq.Candles, 20)
			ue50 := calcEMA(uq.Candles, 50)
			ue200 := calcEMA(uq.Candles, 200)
			um, us, uh := calcMACD(uq.Candles)
			ua := calcATR(uq.Candles, 14)
			uv := avgVolume(uq.Candles, 20)
			uvr := 0.0
			if uv > 0 {
				uvr = lastVolume(uq.Candles) / uv
			}
			usp, urs := supportResistance(uq.Candles, 60)
			ub, _ := technicalScoreV2(uq, ur, calcStochRSI(uq.Candles, 14), ue20, ue50, ue200, um, us, uh, ua, uvr, usp, urs, nil)
			r.UnderlyingPrice = uq.Price
			r.UnderlyingRSI = ur
			r.UnderlyingMACDHist = uh
			r.UnderlyingScore = ub.Total
			if ub.Total >= 5 {
				r.ScoreContext += 2
				r.Reasons = append(r.Reasons, "Subyacente USA fuerte: confirma el CEDEAR.")
			} else if ub.Total >= 2 {
				r.ScoreContext++
				r.Reasons = append(r.Reasons, "Subyacente USA acompaña moderadamente.")
			} else if ub.Total <= -5 {
				r.ScoreContext -= 2
				r.Reasons = append(r.Reasons, "Subyacente USA débil: alerta aunque el CEDEAR en pesos parezca fuerte.")
			} else if ub.Total <= -2 {
				r.ScoreContext--
				r.Reasons = append(r.Reasons, "Subyacente USA con debilidad moderada.")
			}
		}
	}
	cx := cclContext()
	r.CCLProxy = cx.Value
	r.CCLChange20Pct = cx.Change20Pct
	if cx.Value > 0 {
		if cx.Change20Pct >= 3 {
			r.ScoreContext++
			r.Reasons = append(r.Reasons, "CCL proxy subiendo en 20 ruedas: viento a favor para el CEDEAR en pesos.")
		} else if cx.Change20Pct <= -3 {
			r.ScoreContext--
			r.Reasons = append(r.Reasons, "CCL proxy bajando en 20 ruedas: puede restar rendimiento al CEDEAR en pesos.")
		}
	}
	r.ScoreContext = clampInt(r.ScoreContext, -3, 3)
}

func defaultExpectedDays(strategy string) int {
	if strings.EqualFold(strategy, "POSITION") {
		return defaultPositionDays
	}
	return defaultSwingDays
}

func normalizeStrategy(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "POSITION") {
		return "POSITION"
	}
	return "SWING"
}

func deriveInitialStop(entry float64, r Result, manual float64) (float64, float64, string) {
	if entry <= 0 {
		return 0, 0, "sin entrada"
	}
	if manual > 0 && manual < entry {
		risk := (entry - manual) / entry * 100
		return manual, risk, "manual"
	}
	maxRisk := 9.0
	minRisk := 3.5
	if r.Asset.Type == "CEDEAR" {
		maxRisk = 10.0
	}
	if isLeveragedTicker(guessUnderlying(r.Asset.Ticker)) {
		maxRisk = 13.0
		minRisk = 5.0
	}

	dist := entry * 0.055
	source := "auto 5.5%"
	if r.ATR > 0 {
		dist = math.Max(dist, r.ATR*1.8)
		source = "ATR + soporte"
	}
	candidate := entry - dist
	if r.Support > 0 && r.Support < entry {
		pad := 0.0
		if r.ATR > 0 {
			pad = r.ATR * 0.35
		}
		supportStop := r.Support - pad
		if supportStop > 0 {
			candidate = math.Min(candidate, supportStop)
		}
	}
	riskPct := (entry - candidate) / entry * 100
	riskPct = clampFloat(riskPct, minRisk, maxRisk)
	return entry * (1 - riskPct/100), riskPct, source
}

func positionPlan(p PortfolioPosition, r Result) (stop float64, riskPct float64, source string, expected int, strategy string) {
	strategy = normalizeStrategy(p.Strategy)
	expected = p.ExpectedDays
	if expected <= 0 {
		expected = defaultExpectedDays(strategy)
	}
	if p.InitialStopPrice > 0 && p.InitialStopPrice < p.AvgBuyPrice {
		stop = p.InitialStopPrice
		if p.InitialRiskPct > 0 {
			riskPct = p.InitialRiskPct
		} else if p.AvgBuyPrice > 0 {
			riskPct = (p.AvgBuyPrice - stop) / p.AvgBuyPrice * 100
		}
		source = p.StopSource
		if source == "" {
			source = "plan guardado"
		}
		return
	}
	// Compatibilidad con cartera vieja: jamás queda sin stop solo por haber sido creada con una versión anterior.
	fallbackRisk := 8.0
	if r.Asset.Type == "CEDEAR" {
		fallbackRisk = 10.0
	}
	if isLeveragedTicker(guessUnderlying(p.Ticker)) {
		fallbackRisk = 13.0
	}
	if p.AvgBuyPrice > 0 {
		stop = p.AvgBuyPrice * (1 - fallbackRisk/100)
		riskPct = fallbackRisk
		source = "legacy auto"
	}
	return
}

func rMultiple(entry, stop, current float64) float64 {
	risk := entry - stop
	if entry <= 0 || stop <= 0 || risk <= 0 {
		return 0
	}
	return (current - entry) / risk
}

func tradingSessionsSince(c []Candle, buyDate string) int {
	if buyDate == "" {
		return 0
	}
	start, err := time.Parse("2006-01-02", buyDate)
	if err != nil {
		return 0
	}
	n := 0
	for _, x := range c {
		if !x.Time.Before(start) {
			n++
		}
	}
	if n > 0 {
		return n
	}
	return 0
}

func velocityInfo(pnlPct float64, days int) (string, int, float64) {
	d := days
	if d < 1 {
		d = 1
	}
	perDay := pnlPct / float64(d)
	pts := 0
	label := "Normal"
	switch {
	case pnlPct >= 5 && days <= 1:
		label, pts = "🚀 Excepcionalmente rápida", 2
	case pnlPct >= 7 && days <= 3:
		label, pts = "🔥 Muy rápida", 2
	case pnlPct >= 7 && days <= 10:
		label, pts = "🟢 Rápida", 1
	case pnlPct >= 7 && days <= 30:
		label = "🟢 Buena"
	case days >= 120 && pnlPct > 0 && pnlPct < 10:
		label = "🟡 Capital lento"
	case pnlPct < 0 && days >= 45:
		label = "🟠 Pérdida prolongada"
	}
	return label, pts, perDay
}

func profitLockStop(entry, initialStop, r float64) float64 {
	risk := entry - initialStop
	if risk <= 0 {
		return initialStop
	}
	stop := initialStop
	switch {
	case r >= 4:
		stop = math.Max(stop, entry+2.5*risk)
	case r >= 3:
		stop = math.Max(stop, entry+1.5*risk)
	case r >= 2:
		stop = math.Max(stop, entry+0.5*risk)
	case r >= 1:
		stop = math.Max(stop, entry)
	}
	return stop
}

func analyzePortfolioExitV2(p PortfolioPosition, r Result, pnlPct float64, days int) PortfolioExit {
	price := r.Quote.Price
	reasons := []string{}
	maxSinceBuy := maxPriceSinceBuy(r.Quote.Candles, p.BuyDate, price)
	drawdown := 0.0
	if maxSinceBuy > 0 && price > 0 {
		drawdown = (maxSinceBuy - price) / maxSinceBuy * 100
	}

	initStop, initRiskPct, stopSource, expectedDays, strategy := positionPlan(p, r)
	sessions := tradingSessionsSince(r.Quote.Candles, p.BuyDate)
	if sessions <= 0 {
		sessions = days
	}
	rm := rMultiple(p.AvgBuyPrice, initStop, price)
	velocityLabel, velocityPts, returnPerDay := velocityInfo(pnlPct, sessions)
	trailPct := dynamicTrailingStopPctV2(r, pnlPct, rm)
	trailPrice := 0.0
	if maxSinceBuy > 0 {
		trailPrice = maxSinceBuy * (1 - trailPct/100)
	}
	lockStop := profitLockStop(p.AvgBuyPrice, initStop, rm)
	effectiveStop := math.Max(initStop, lockStop)
	if pnlPct > 0 && rm >= 1 && trailPrice > 0 {
		effectiveStop = math.Max(effectiveStop, trailPrice)
	}

	out := PortfolioExit{
		Signal: "HOLD - DEJAR CORRER", Class: "hold",
		Action:           "Mantener mientras la tesis siga válida. El stop efectivo y el tiempo del trade se recalculan automáticamente.",
		MaxPriceSinceBuy: maxSinceBuy, DrawdownFromHighPct: drawdown,
		TrailingStopPrice: trailPrice, TrailingStopPct: trailPct,
		InitialStopPrice: initStop, EffectiveStopPrice: effectiveStop, InitialRiskPct: initRiskPct,
		RMultiple: rm, VelocityLabel: velocityLabel, ReturnPerDayPct: returnPerDay,
		ExpectedDays: expectedDays, HoldingSessions: sessions, Strategy: strategy, StopSource: stopSource,
	}
	if price <= 0 {
		out.Signal, out.Class = "SIN PRECIO - VALIDAR", "neutral"
		out.Action = "No tomar decisión automática: validar precio y puntas en Balanz."
		out.Reasons = []string{"No hay precio válido para calcular una salida confiable."}
		return out
	}

	// 1) Stops duros: no esperan confirmación.
	if initStop > 0 && price <= initStop {
		out.Signal, out.Class, out.Score, out.SuggestedSellPct, out.Immediate = "STOP - TESIS INVALIDADA", "strongsell", 12, 100, true
		out.Action = "El precio rompió el stop técnico definido al entrar. Revisar salida total; no transformar un swing fallido en inversión eterna."
		out.Reasons = []string{fmt.Sprintf("Stop inicial roto: precio %s <= stop %s (%s).", money(price), money(initStop), stopSource)}
		return out
	}
	if rm >= 1 && effectiveStop > initStop && price <= effectiveStop {
		out.Signal, out.Class, out.Score, out.SuggestedSellPct, out.Immediate = "STOP DINÁMICO - ASEGURAR", "strongsell", 11, 80, true
		out.Action = "Se rompió el stop de protección de una operación que ya había avanzado. Asegurar 70% a 100% según liquidez y puntas."
		out.Reasons = []string{fmt.Sprintf("Stop efectivo roto en %.2fR; protege una ganancia ya conseguida.", rm)}
		return out
	}

	score := 0
	if rm >= 4 {
		score += 3
		reasons = append(reasons, fmt.Sprintf("Trade excelente: %.2fR.", rm))
	} else if rm >= 3 {
		score += 2
		reasons = append(reasons, fmt.Sprintf("Muy buen trade: %.2fR.", rm))
	} else if rm >= 2 {
		score += 1
		reasons = append(reasons, fmt.Sprintf("Trade ya alcanzó %.2fR; conviene proteger parte.", rm))
	}

	if velocityPts > 0 {
		score += velocityPts
		reasons = append(reasons, fmt.Sprintf("Velocidad: %s (%.2f%% por rueda aprox.).", velocityLabel, returnPerDay))
	} else if strings.Contains(velocityLabel, "Capital lento") || strings.Contains(velocityLabel, "Pérdida prolongada") {
		reasons = append(reasons, "Eficiencia temporal baja: el capital lleva demasiado tiempo inmovilizado.")
	}

	// Momentum / estructura para salida.
	if r.RSI >= 82 {
		score += 3
		reasons = append(reasons, "RSI diario extremadamente alto.")
	} else if r.RSI >= 72 {
		score += 2
		reasons = append(reasons, "RSI diario alto; vigilar agotamiento.")
	} else if r.RSI >= 45 && r.RSI <= 62 && pnlPct > 0 {
		score--
		reasons = append(reasons, "RSI diario sano; todavía puede quedar recorrido.")
	}

	if r.MACD < r.MACDSignal && r.MACDHist < 0 {
		score += 2
		reasons = append(reasons, "MACD diario bajista.")
	} else if r.MACD > r.MACDSignal && r.MACDHist > 0 && pnlPct > 0 {
		score--
		reasons = append(reasons, "MACD diario sigue alcista.")
	}

	if r.FastEMA20 > 0 && r.FastEMA50 > 0 {
		if r.FastEMA20 < r.FastEMA50 && r.FastMACDHist < 0 {
			score += 2
			reasons = append(reasons, "4H confirma deterioro: EMA20<EMA50 y MACD negativo.")
		} else if r.FastEMA20 > r.FastEMA50 && r.FastMACDHist > 0 && pnlPct > 0 {
			score--
			reasons = append(reasons, "4H todavía acompaña el movimiento.")
		}
	}

	if r.EMA20 > 0 && price < r.EMA20 {
		score++
		reasons = append(reasons, "Precio perdió EMA20 diaria.")
	}
	if r.EMA50 > 0 && price < r.EMA50 {
		score += 2
		reasons = append(reasons, "Precio perdió EMA50 diaria.")
	}
	if r.EMA20 > 0 && r.EMA50 > 0 && price > r.EMA20 && r.EMA20 > r.EMA50 && pnlPct > 0 {
		score--
		reasons = append(reasons, "Estructura diaria sigue fuerte: precio>EMA20>EMA50.")
	}

	if r.Resistance > 0 && pnlPct > 0 {
		dr := (r.Resistance - price) / price * 100
		if dr >= 0 && dr <= 2.5 {
			score += 2
			reasons = append(reasons, "Muy cerca de resistencia diaria.")
		} else if dr > 2.5 && dr <= 6 {
			score++
			reasons = append(reasons, "Acercándose a resistencia diaria.")
		}
	}
	if r.VolumeRatio > 1.5 && r.Quote.PreviousClose > 0 && price < r.Quote.PreviousClose {
		score += 2
		reasons = append(reasons, "Caída con volumen alto.")
	}

	// CEDEAR: no confundir fortaleza del peso/CCL con la del activo subyacente.
	if r.Asset.Type == "CEDEAR" {
		if r.UnderlyingScore <= -5 {
			score += 2
			reasons = append(reasons, "Subyacente USA claramente débil.")
		} else if r.UnderlyingScore >= 5 && pnlPct > 0 {
			score--
			reasons = append(reasons, "Subyacente USA sigue fuerte.")
		}
		if r.CCLChange20Pct <= -3 {
			reasons = append(reasons, "CCL proxy bajista: puede presionar el CEDEAR en pesos.")
		}
	}

	// FARO V5: la salida prioriza ruptura de tendencia, pérdida de fuerza relativa y
	// deterioro del contexto. VIX y mercado contextualizan; nunca venden por sí solos.
	faro := evaluateV4(r)
	faroLow10 := v5PriorLow(r.Quote.Candles, 10)
	faroLow20 := v5PriorLow(r.Quote.Candles, 20)
	faroBreak10 := faroLow10 > 0 && price < faroLow10
	faroBreak20 := faroLow20 > 0 && price < faroLow20
	if faroBreak20 {
		score += 4
		reasons = append(reasons, fmt.Sprintf("FARO: rompió el mínimo de 20 ruedas (%s); tendencia claramente deteriorada.", money(faroLow20)))
	} else if faroBreak10 {
		score += 3
		reasons = append(reasons, fmt.Sprintf("FARO: rompió el mínimo de 10 ruedas (%s), señal clásica de salida trend-following.", money(faroLow10)))
	}
	if faro.RelativeStrength20 < -5 {
		score += 2
		reasons = append(reasons, fmt.Sprintf("Fuerza relativa 20r débil vs %s: %.1f%%.", faro.BenchmarkTicker, faro.RelativeStrength20))
	} else if faro.RelativeStrength20 < 0 {
		score++
		reasons = append(reasons, "Fuerza relativa de corto plazo dejó de acompañar.")
	} else if pnlPct > 0 && faro.RelativeStrength20 > 3 {
		score--
		reasons = append(reasons, "Fuerza relativa sigue positiva; no apurar una salida ganadora.")
	}
	structureWeakFARO := (r.EMA20 > 0 && price < r.EMA20) || r.MACDHist < 0 || faroBreak10
	if faro.MarketRegime == "DÉBIL" && structureWeakFARO {
		score++
		reasons = append(reasons, "Benchmark débil y estructura propia deteriorada.")
	}
	if faro.VIXLevel >= 30 && structureWeakFARO {
		score++
		reasons = append(reasons, fmt.Sprintf("VIX %.1f (%s): aumenta el riesgo mientras la estructura ya está débil.", faro.VIXLevel, faro.VIXRegime))
	}
	if rm >= 2 && pnlPct > 0 && !faroBreak10 && r.EMA20 > 0 && price > r.EMA20 && r.MACDHist > 0 && faro.RelativeStrength20 > 0 {
		score--
		reasons = append(reasons, "FARO: +2R o más con tendencia y momentum todavía sanos; conviene dejar correr antes que vender por ansiedad.")
	}

	// Time stop V4: para trading corto, la operación debe demostrar relativamente rápido que la tesis funcionaba.
	timeExpired := sessions > expectedDays
	timeVeryExpired := sessions > expectedDays*2
	weakTrend := (r.EMA20 > 0 && price < r.EMA20) || (r.MACDHist < 0)
	tradeSlow := false
	tradeVerySlow := false
	if strings.EqualFold(strategy, "SWING") {
		v4 := evaluateV4(r)
		if sessions >= 5 && pnlPct < 0.5 && v4.TradingScore < 75 {
			reasons = append(reasons, fmt.Sprintf("Trade lento: a las %d ruedas todavía no mostró progreso y Trading Score cayó a %d/100.", sessions, v4.TradingScore))
		}
		if sessions >= 8 && pnlPct < 1.0 && v4.TradingScore < 70 {
			score += 2
			tradeSlow = true
			reasons = append(reasons, "Capital inmóvil: 8+ ruedas sin avance suficiente y momentum débil.")
		}
		if sessions >= 10 && pnlPct < 2.0 && v4.TradingScore < 72 {
			score += 3
			tradeVerySlow = true
			reasons = append(reasons, "Rotación sugerida: 10+ ruedas sin alcanzar un movimiento razonable.")
		}
		if sessions >= expectedDays && pnlPct < 3.0 && v4.TradingScore < 78 {
			score += 4
			tradeVerySlow = true
			reasons = append(reasons, fmt.Sprintf("Fin de tesis corta: %d/%d ruedas con retorno pobre y sin aceleración.", sessions, expectedDays))
		}
	}
	if timeVeryExpired && pnlPct < 8 && weakTrend {
		score += 3
		reasons = append(reasons, fmt.Sprintf("Time stop: %d ruedas, más del doble del horizonte %s (%d ruedas), con retorno pobre/debilidad.", sessions, strategy, expectedDays))
	} else if timeExpired && pnlPct < 5 && weakTrend {
		score += 2
		reasons = append(reasons, fmt.Sprintf("Time stop: excedió %d ruedas y el capital no está trabajando bien.", expectedDays))
	} else if timeExpired {
		reasons = append(reasons, fmt.Sprintf("Horizonte %s excedido (%d/%d ruedas), pero la técnica no obliga a salir todavía.", strategy, sessions, expectedDays))
	}

	// Drawdown desde máximo: se usa como señal, pero el stop efectivo manda.
	if drawdown >= trailPct*0.8 && pnlPct > 0 {
		score += 2
		reasons = append(reasons, "Retroceso importante desde el máximo del trade.")
	}

	// Pérdidas: si todavía no rompió el stop pero se deterioró, avisar antes de llegar tarde.
	if pnlPct <= -6 && weakTrend {
		score += 3
		reasons = append(reasons, "Pérdida relevante con estructura débil; no esperar pasivamente al -20%.")
	}
	if pnlPct <= -10 {
		score += 2
		reasons = append(reasons, "Pérdida de dos dígitos: revisión obligatoria.")
	}

	out.Score = score
	out.Reasons = uniqueStrings(reasons)
	if len(out.Reasons) == 0 {
		out.Reasons = []string{"Sin señales suficientes para salir ahora."}
	}

	// Señales finales. El time stop tiene su propia categoría para no confundirlo con take profit.
	if pnlPct < 0 && score >= 6 {
		out.Signal, out.Class, out.SuggestedSellPct = "PROTEGER CAPITAL", "strongsell", 70
		out.Action = "La operación está perdiendo y la estructura/tiempo empeoró. Revisar salida 50% a 100%; el stop inicial sigue siendo la línea roja."
	} else if tradeVerySlow && score >= 4 {
		out.Signal, out.Class, out.SuggestedSellPct = "ROTAR CAPITAL / TRADE LENTO", "sell", 70
		out.Action = "La operación corta no hizo lo que debía en el tiempo esperado. Considerar liberar 50% a 100% y reasignar capital."
	} else if tradeSlow && score >= 2 {
		out.Signal, out.Class, out.SuggestedSellPct = "TRADE LENTO - REVISAR", "sell", 35
		out.Action = "El trade está consumiendo tiempo sin progreso. Revisar si conviene reducir 25% a 50% si la próxima lectura no mejora."
	} else if timeVeryExpired && pnlPct < 8 && weakTrend && score >= 4 {
		out.Signal, out.Class, out.SuggestedSellPct = "ROTAR CAPITAL / TIME STOP", "sell", 50
		out.Action = "El swing consumió demasiado tiempo sin compensar. Considerar liberar 50% a 100% para una oportunidad mejor."
	} else if faroBreak10 && pnlPct > 0 && score >= 5 {
		out.Signal, out.Class, out.SuggestedSellPct = "SALIR / FIN DE TENDENCIA", "strongsell", 80
		out.Action = "Mejor momento estimado de salida: la tendencia que justificaba mantener se quebró. Considerar vender 70% a 100% según spread/liquidez."
	} else if score >= 9 && pnlPct > 0 {
		out.Signal, out.Class, out.SuggestedSellPct = "SALIR / VENDER MUCHO", "strongsell", 90
		out.Action = "Confluencia fuerte de salida: ganancia + agotamiento + deterioro técnico/contextual. Asegurar 70% a 100%."
	} else if score >= 6 && pnlPct > 0 {
		out.Signal, out.Class, out.SuggestedSellPct = "TOMAR GANANCIA FUERTE", "strongsell", 65
		out.Action = "Momento favorable para realizar 50% a 70%: hay deterioro suficiente para asegurar ganancia sin depender de acertar el máximo exacto."
	} else if score >= 4 && pnlPct > 0 {
		out.Signal, out.Class, out.SuggestedSellPct = "TOMAR GANANCIA PARCIAL", "sell", 30
		out.Action = "Tomar aproximadamente 25% a 40% y dejar el resto con stop efectivo mientras la tendencia no se quiebre."
	} else if score >= 2 && pnlPct > 0 {
		out.Signal, out.Class = "PROTEGER GANANCIA", "protect"
		out.Action = "No vender todavía. Subir disciplina: respetar el stop efectivo y vigilar las próximas dos lecturas."
	}
	return out
}

func dynamicTrailingStopPctV2(r Result, pnlPct, rm float64) float64 {
	pct := 8.0
	if r.Asset.Type == "CEDEAR" {
		pct = 9.0
	}
	if isLeveragedTicker(guessUnderlying(r.Asset.Ticker)) {
		pct = 11.0
	}
	if rm >= 4 {
		pct = math.Min(pct, 6.0)
	} else if rm >= 3 {
		pct = math.Min(pct, 7.0)
	} else if rm >= 2 {
		pct = math.Min(pct, 8.0)
	}
	if r.ATR > 0 && r.Quote.Price > 0 {
		atrPct := r.ATR / r.Quote.Price * 100
		pct = math.Max(pct, math.Min(14, atrPct*2.0))
	}
	return clampFloat(pct, 5, 14)
}

func captureTradePlan(ticker string, entry float64, strategy string, expectedDays int, manualStop float64) TradePlanCapture {
	out := TradePlanCapture{Strategy: normalizeStrategy(strategy)}
	if expectedDays <= 0 {
		expectedDays = defaultExpectedDays(out.Strategy)
	}
	out.ExpectedDays = expectedDays
	if entry <= 0 {
		return out
	}
	a, ok := assetByTicker(ticker)
	if !ok {
		return out
	}
	q := fetchYahooChart(a.YahooTicker, deepChartRange, deepChartInterval)
	if q.Price <= 0 {
		q.Price = entry
	}
	r := resultFromQuoteV2(a, q, true)
	stop, riskPct, source := deriveInitialStop(entry, r, manualStop)
	out.InitialStopPrice, out.InitialRiskPct, out.StopSource = stop, riskPct, source
	out.EntryATR, out.EntrySupport, out.EntryRSI = r.ATR, r.Support, r.RSI
	out.EntryScore, out.EntryUnderlyingScore, out.EntryCCLProxy = r.ScoreFinal, r.UnderlyingScore, r.CCLProxy
	out.PlanCreatedAt = time.Now()
	return out
}

type TradePlanCapture struct {
	Strategy             string
	ExpectedDays         int
	InitialStopPrice     float64
	InitialRiskPct       float64
	StopSource           string
	EntryATR             float64
	EntrySupport         float64
	EntryRSI             float64
	EntryScore           int
	EntryUnderlyingScore int
	EntryCCLProxy        float64
	PlanCreatedAt        time.Time
}

func assetByTicker(ticker string) (Asset, bool) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	for _, a := range loadAllAssets() {
		if strings.EqualFold(a.Ticker, ticker) {
			return a, true
		}
	}
	return Asset{}, false
}

func resultFromQuoteV2(a Asset, q Quote, force bool) Result {
	rsi := calcRSI(q.Candles, 14)
	stoch := calcStochRSI(q.Candles, 14)
	ema20 := calcEMA(q.Candles, 20)
	ema50 := calcEMA(q.Candles, 50)
	ema200 := calcEMA(q.Candles, 200)
	macd, sig, hist := calcMACD(q.Candles)
	atr := calcATR(q.Candles, 14)
	volAvg := avgVolume(q.Candles, 20)
	vr := 0.0
	if volAvg > 0 {
		vr = lastVolume(q.Candles) / volAvg
	}
	support, resistance := supportResistance(q.Candles, 60)
	b, reasons := technicalScoreV2(q, rsi, stoch, ema20, ema50, ema200, macd, sig, hist, atr, vr, support, resistance, nil)
	riskScoreValue, riskText, riskReasons := riskScore(a, rsi, q)
	r := Result{Asset: a, Quote: q, RSI: rsi, StochRSI: stoch, EMA20: ema20, EMA50: ema50, EMA200: ema200, MACD: macd, MACDSignal: sig, MACDHist: hist, ATR: atr, VolumeAvg20: volAvg, VolumeRatio: vr, Support: support, Resistance: resistance, ScoreTrend: b.Trend, ScoreMomentum: b.Momentum, ScoreVolume: b.Volume, ScoreLocation: b.Location, ScoreTechnical: b.Total, ScoreRisk: riskScoreValue, Risk: riskText, Reasons: append(reasons, riskReasons...), AnalysisMode: "Profundo V2"}
	enrichResultV2(a, &r, force)
	r.ScoreFinal = r.ScoreTechnical + r.ScoreContext + r.ScoreRisk
	r.Recommendation = recommendation(r.ScoreFinal)
	return r
}

// ---------------- BACKTEST ON DEMAND ----------------

type BacktestTrade struct {
	EntryDate time.Time
	ExitDate  time.Time
	Entry     float64
	Exit      float64
	ReturnPct float64
	R         float64
	Days      int
	Reason    string
}

type BacktestReport struct {
	Ticker          string
	YahooTicker     string
	Trades          []BacktestTrade
	Count           int
	WinRate         float64
	AvgReturn       float64
	AvgR            float64
	AvgDays         float64
	TotalCompounded float64
	MaxDrawdown     float64
	Note            string
}

func backtestHandler(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("ticker")))
	if ticker == "" {
		ticker = "GOOGL"
	}
	a, ok := assetByTicker(ticker)
	if !ok {
		http.Error(w, "Ticker no encontrado en el dashboard", http.StatusBadRequest)
		return
	}
	rep := runBacktestV2(a)
	tpl := template.Must(template.New("bt").Funcs(template.FuncMap{"pct": pct, "money": money, "fmtDate": func(t time.Time) string { return t.Format("02/01/2006") }}).Parse(backtestHTML))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tpl.Execute(w, rep)
}

func runBacktestV2(a Asset) BacktestReport {
	q := fetchYahooChart(a.YahooTicker, "5y", "1d")
	rep := BacktestReport{Ticker: a.Ticker, YahooTicker: a.YahooTicker, Note: "Backtest orientativo con cierres diarios, sin comisiones, spread, slippage ni impuestos. No predice resultados futuros."}
	c := q.Candles
	if len(c) < 260 {
		return rep
	}
	equity, peak, maxDD := 1.0, 1.0, 0.0
	inTrade := false
	var entry, stop, maxP float64
	var entryDate time.Time
	entryIndex := 0
	for i := 220; i < len(c); i++ {
		sub := append([]Candle(nil), c[:i+1]...)
		cq := Quote{Ticker: a.YahooTicker, Price: c[i].Close, PreviousClose: c[i-1].Close, Candles: sub}
		r := resultFromHistoricalQuoteNoContext(a, cq)
		if !inTrade {
			// Entrada exigente para evitar sobreoperar el backtest.
			if r.ScoreTechnical >= 6 && r.EMA20 > r.EMA50 && r.MACDHist > 0 && r.RSI < 72 {
				entry = c[i].Close
				stop, _, _ = deriveInitialStop(entry, r, 0)
				maxP = entry
				entryDate = c[i].Time
				entryIndex = i
				inTrade = true
			}
			continue
		}
		if c[i].High > maxP {
			maxP = c[i].High
		}
		sessions := i - entryIndex
		rm := rMultiple(entry, stop, c[i].Close)
		trailPct := dynamicTrailingStopPctV2(r, (c[i].Close-entry)/entry*100, rm)
		eff := profitLockStop(entry, stop, rm)
		if rm >= 1 {
			eff = math.Max(eff, maxP*(1-trailPct/100))
		}
		exitReason := ""
		if c[i].Close <= stop {
			exitReason = "Stop inicial"
		}
		if exitReason == "" && rm >= 1 && c[i].Close <= eff {
			exitReason = "Stop dinámico"
		}
		if exitReason == "" && rm >= 3 && r.RSI >= 72 && r.MACDHist < 0 {
			exitReason = "Toma 3R + agotamiento"
		}
		if exitReason == "" && sessions > defaultSwingDays && c[i].Close < r.EMA20 && r.MACDHist < 0 {
			exitReason = "Time stop"
		}
		if i == len(c)-1 && exitReason == "" {
			exitReason = "Fin de muestra"
		}
		if exitReason != "" {
			ret := (c[i].Close - entry) / entry * 100
			rr := rMultiple(entry, stop, c[i].Close)
			rep.Trades = append(rep.Trades, BacktestTrade{EntryDate: entryDate, ExitDate: c[i].Time, Entry: entry, Exit: c[i].Close, ReturnPct: ret, R: rr, Days: sessions, Reason: exitReason})
			equity *= (1 + ret/100)
			if equity > peak {
				peak = equity
			}
			dd := (peak - equity) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
			inTrade = false
		}
	}
	rep.Count = len(rep.Trades)
	rep.MaxDrawdown = maxDD
	rep.TotalCompounded = (equity - 1) * 100
	if rep.Count > 0 {
		wins := 0
		sr, sR, sd := 0.0, 0.0, 0.0
		for _, t := range rep.Trades {
			if t.ReturnPct > 0 {
				wins++
			}
			sr += t.ReturnPct
			sR += t.R
			sd += float64(t.Days)
		}
		rep.WinRate = float64(wins) / float64(rep.Count) * 100
		rep.AvgReturn = sr / float64(rep.Count)
		rep.AvgR = sR / float64(rep.Count)
		rep.AvgDays = sd / float64(rep.Count)
	}
	sort.Slice(rep.Trades, func(i, j int) bool { return rep.Trades[i].EntryDate.After(rep.Trades[j].EntryDate) })
	if len(rep.Trades) > 30 {
		rep.Trades = rep.Trades[:30]
	}
	return rep
}

func resultFromHistoricalQuoteNoContext(a Asset, q Quote) Result {
	rsi := calcRSI(q.Candles, 14)
	st := calcStochRSI(q.Candles, 14)
	e20 := calcEMA(q.Candles, 20)
	e50 := calcEMA(q.Candles, 50)
	e200 := calcEMA(q.Candles, 200)
	m, s, h := calcMACD(q.Candles)
	atr := calcATR(q.Candles, 14)
	va := avgVolume(q.Candles, 20)
	vr := 0.0
	if va > 0 {
		vr = lastVolume(q.Candles) / va
	}
	sp, rs := supportResistance(q.Candles, 60)
	b, reasons := technicalScoreV2(q, rsi, st, e20, e50, e200, m, s, h, atr, vr, sp, rs, nil)
	return Result{Asset: a, Quote: q, RSI: rsi, StochRSI: st, EMA20: e20, EMA50: e50, EMA200: e200, MACD: m, MACDSignal: s, MACDHist: h, ATR: atr, VolumeAvg20: va, VolumeRatio: vr, Support: sp, Resistance: rs, ScoreTrend: b.Trend, ScoreMomentum: b.Momentum, ScoreVolume: b.Volume, ScoreLocation: b.Location, ScoreTechnical: b.Total, Reasons: reasons}
}

const backtestHTML = `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Backtest {{.Ticker}}</title><style>body{font-family:Segoe UI,Arial;background:#070b14;color:#e8eefc;margin:0;padding:24px}.wrap{max-width:1200px;margin:auto}.card{background:#0d1424;border:1px solid #22304d;border-radius:16px;padding:16px;margin:10px 0}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px}table{width:100%;border-collapse:collapse;background:#0d1424}th,td{padding:10px;border-bottom:1px solid #22304d;text-align:left}.muted{color:#8ea0c4}.good{color:#86efac}.bad{color:#fca5a5}a{color:#7dd3fc}</style></head><body><div class="wrap"><p><a href="/">← Volver al dashboard</a></p><h1>Backtest V2 · {{.Ticker}}</h1><p class="muted">{{.Note}}</p><div class="grid"><div class="card"><b>Trades</b><h2>{{.Count}}</h2></div><div class="card"><b>Win rate</b><h2>{{pct .WinRate}}</h2></div><div class="card"><b>Retorno medio/trade</b><h2>{{pct .AvgReturn}}</h2></div><div class="card"><b>R medio</b><h2>{{printf "%.2fR" .AvgR}}</h2></div><div class="card"><b>Ruedas medias</b><h2>{{printf "%.1f" .AvgDays}}</h2></div><div class="card"><b>Compuesto muestra</b><h2>{{pct .TotalCompounded}}</h2></div><div class="card"><b>Drawdown aproximado</b><h2>{{pct .MaxDrawdown}}</h2></div></div><div class="card"><b>Interpretación:</b> sirve para comparar reglas y detectar si una modificación empeora/mejora el sistema históricamente. No usar el número como promesa de rentabilidad.</div><table><thead><tr><th>Entrada</th><th>Salida</th><th>Precio E</th><th>Precio S</th><th>Retorno</th><th>R</th><th>Ruedas</th><th>Motivo</th></tr></thead><tbody>{{range .Trades}}<tr><td>{{fmtDate .EntryDate}}</td><td>{{fmtDate .ExitDate}}</td><td>{{money .Entry}}</td><td>{{money .Exit}}</td><td>{{pct .ReturnPct}}</td><td>{{printf "%.2fR" .R}}</td><td>{{.Days}}</td><td>{{.Reason}}</td></tr>{{else}}<tr><td colspan="8">No hubo suficientes señales con estas reglas.</td></tr>{{end}}</tbody></table></div></body></html>`
