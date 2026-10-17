//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	appName        = "Felix Investment Dashboard"
	port           = "127.0.0.1:8080"
	serviceName    = "FelixInvestmentDashboard"
	serviceDisplay = "Felix Investment Dashboard"

	portfolioFile     = "portfolio.json"
	alertStateFile    = "alerts_state.json"
	alertHistoryFile  = "alerts_history.json"
	lastPricesFile    = "last_prices.json"
	dataSubdir        = "data"
	monitorInterval   = 5 * time.Minute
	alertCooldown     = 6 * time.Hour
	cacheTTL          = 10 * time.Minute
	quoteBatchSize    = 80
	quoteWorkers      = 8
	chartWorkers      = 8
	httpTimeout       = 14 * time.Second
	deepChartRange    = "1y"
	deepChartInterval = "1d"
)

const accionesHardcoded = `
ALUA
BBAR
BMA
BYMA
CEPU
COME
CRES
EDN
GGAL
LOMA
METR
PAMP
SUPV
TECO2
TGNO4
TGSU2
TRAN
TXAR
VALO
YPFD
`

const cedearsHardcoded = `
AAL
AALC
AALD
AAP
AAPC
AAPD
AAPL
AAPLC
AAPLD
ABBV
ABBVC
ABBVD
ABE3C
ABE3D
ABEV
ABEV3
ABEVC
ABEVD
ABNB
ABNBC
ABNBD
ABT
ABTC
ABTD
ACN
ACNC
ACND
ACWI
ACWIC
ACWID
ADBE
ADBEC
ADBED
ADGO
ADGOC
ADGOD
ADI
ADID
ADP
ADS
ADSC
ADSD
AEG
AEM
AEMC
AEMD
AI
AIC
AID
AIG
AIGC
AIGD
AKO.B
ALAB
ALAC
ALAD
AMAT
AMATC
AMATD
AMD
AMDC
AMDD
AMGN
AMGNC
AMGND
AMX
AMXC
AMXD
AMZN
AMZNC
AMZND
ANET
ANETC
ANETD
ANF
ANFC
ANFD
ARCO
ARCOC
ARCOD
ARKK
ARKKC
ARKKD
ARM
ARMC
ARMD
ASML
ASMLC
ASMLD
ASR
ASTS
ASTSC
ASTSD
AVGO
AVGOC
AVGOD
AVY
AVYC
AVYD
AXIA
AXIAC
AXIAD
AXP
AXPC
AXPD
AZN
AZNC
AZND
B
B.C
B.D
BA
BA.C
BA.CC
BA.CD
BABA
BABAC
BABAD
BAC
BAD
BAK
BAKC
BAKD
BAS
BAYN
BB
BBA3C
BBA3D
BBAS3
BBD
BBDC3
BBDCC
BBDCD
BBDD
BBV
BBVD
BCS
BHP
BHPD
BIDU
BIDUC
BIDUD
BIIB
BIIBC
BIIBD
BIOX
BIOXC
BIOXD
BK
BKC
BKD
BKNG
BKNGC
BKNGD
BKR
BKRC
BKRD
BMNR
BMNRC
BMNRD
BMY
BNG
BNGC
BNGD
BP
BPA11
BPA1C
BPA1D
BPC
BPD
BRKB
BRKBC
BRKBD
BSBR
BX
BXC
BXD
C
C.D
CAAP
CAAPC
CAAPD
CAH
CAHC
CAHD
CAR
CAR.C
CAR.D
CAT
CATC
CATD
CC
CCJ
CCJC
CCJD
CCL
CCLC
CCLD
CDE
CEG
CEGC
CEGD
CIBR
CIBRC
CIBRD
CL
CLC
CLD
CLS
CLSC
CLSD
COIN
COINC
COIND
COP
COPC
COPD
COPX
COPXC
COPXD
COST
COSTC
COSTD
CRM
CRMC
CRMD
CRWV
CRWVC
CRWVD
CSCO
CSCOC
CSCOD
CSNA3
CSNAC
CSNAD
CVS
CVSC
CVSD
CVX
CVXC
CVXD
CX
DAL
DALC
DALD
DD
DE
DEC
DECK
DECKC
DECKD
DED
DEO
DEOC
DEOD
DHR
DHRC
DHRD
DIA
DIAC
DIAD
DISN
DISNC
DISND
DJN3C
DJN3D
DJNJ3
DOCU
DOCUC
DOCUD
DOW
DOWC
DOWD
E
EA
EAC
EAD
EBAY
EBAYC
EBAYD
EC
ECL
ECLC
ECLD
EEM
EEMC
EEMD
EFA
EFAC
EFAD
EFX
EFXC
EFXD
ELP
ELPC
ELPCC
ELPCD
ELPD
EMBJ
EMBJC
EMBJD
EOAN
EOANC
EQNR
EQNRC
EQNRD
ERIC
ESGU
ESGUC
ESGUD
ETHA
ETHAC
ETHAD
ETSY
ETSYC
ETSYD
EWJ
EWJC
EWJD
EWY
EWYC
EWYD
EWZ
EWZC
EWZD
F
FC
FCX
FCXC
FCXD
FD
FDX
FDXC
FDXD
FISV
FISVC
FISVD
FMX
FMXC
FMXD
FSLR
FSLRC
FSLRD
FXI
FXIC
FXID
GDX
GDXC
GDXD
GE
GEC
GED
GFI
GGB
GGBC
GGBD
GILD
GILDC
GILDD
GLD
GLDC
GLDD
GLNG
GLNGC
GLNGD
GLOB
GLOBC
GLOBD
GLW
GLWC
GM
GMC
GMD
GOGLC
GOGLD
GOOGL
GPRK
GPRKC
GPRKD
GRMN
GRMNC
GRMND
GS
GSC
GSD
GSK
GSKC
GSKD
GT
GTC
GTD
HAL
HALC
HALD
HAPV3
HD
HDB
HDC
HDD
HIMS
HIMSC
HIMSD
HL
HLC
HLD
HMC
HMC.C
HMY
HMYC
HMYD
HOG
HON
HOND
HOOD
HOODC
HOODD
HPQ
HPQD
HSBC
HSBCC
HSY
HSYC
HSYD
HUT
HUTC
HUTD
HWM
IBB
IBBC
IBBD
IBIT
IBITC
IBITD
IBM
IBMC
IBMD
IBN
ICLN
ICLNC
ICLND
IEMG
IEMGC
IEMGD
IEUR
IEURC
IEURD
IFF
IJH
IJHC
IJHD
ILF
ILFC
ILFD
INFY
INFYC
ING
INTC
INTCC
INTCD
IP
IPC
IPD
IREN
IRENC
IREND
ISRG
ISRGC
ISRGD
ITA
ITAC
ITAD
ITU3C
ITU3D
ITUB
ITUB3
ITUBC
ITUBD
IVE
IVEC
IVED
IVV
IVVC
IVVD
IVW
IVWC
IVWD
IWDA
IWDAC
IWDAD
IWM
IWMC
IWMD
JCI
JCIC
JD
JDC
JDD
JMIA
JMIAC
JMIAD
JNJ
JNJC
JNJD
JOYY
JOYYC
JOYYD
JPM
JPMC
JPMD
KB
KEEL
KEELC
KEELD
KEP
KGC
KMB
KMBC
KMBD
KO
KOC
KOD
KOFM
LAC
LACC
LACD
LAR
LARC
LARD
LLY
LLYC
LLYD
LMT
LMTC
LMTD
LND
LRCX
LRCXC
LRCXD
LRE3C
LREN3
LVS
LYG
MA
MAC
MAD
MBG
MBGC
MBGD
MCD
MCDC
MCDD
MDLZ
MDLZC
MDLZD
MDT
MELI
MELIC
MELID
META
METAC
METAD
MFG
MFGC
MGL3C
MGL3D
MGLU3
MMC
MMM
MMMC
MMMD
MO
MOC
MOD
MOS
MOSC
MOSD
MP
MPC
MPD
MRK
MRKC
MRKD
MRNA
MRNAC
MRNAD
MRSH
MRSHC
MRSHD
MRVL
MRVLC
MRVLD
MSFT
MSFTC
MSFTD
MSI
MSIC
MSTR
MSTRC
MSTRD
MU
MUC
MUD
MUFG
MUFGC
MUFGD
MUX
MUXC
MUXD
NAT3C
NAT3D
NATU3
NBIS
NBISC
NBISD
NEE
NEEC
NEED
NEM
NEMD
NFLX
NFLXC
NFLXD
NG
NGC
NGD
NGG
NIO
NIOC
NIOD
NKE
NKEC
NKED
NMR
NMRC
NOKA
NOW
NOWC
NOWD
NSAN
NSANC
NTES
NTESC
NTESD
NU
NUC
NUD
NUE
NUEC
NVDA
NVDAC
NVDAD
NVS
NVSC
NVSD
NXE
NXEC
NXED
O
OC
OD
OKLO
OKLOC
OKLOD
ONDS
ONDSC
ONDSD
ORCL
ORCLC
ORCLD
ORLY
ORLYC
ORLYD
OXY
OXYC
OXYD
PAAS
PAASC
PAASD
PAC
PAGS
PAGSC
PAGSD
PANW
PANWC
PANWD
PATH
PATHC
PATHD
PBI
PBR
PBRC
PBRD
PCAR
PCARC
PCARD
PDD
PDDC
PDDD
PEP
PEPC
PEPD
PETR3
PETRC
PETRD
PFE
PFEC
PFED
PG
PGC
PGD
PHG
PHGC
PHGD
PINS
PINSC
PINSD
PKS
PLTR
PLTRC
PLTRD
PM
PMC
PMD
PRI3C
PRIO3
PSQ
PSQC
PSQD
PSX
PSXC
PSXD
PYPL
PYPLC
PYPLD
QCOM
QCOMC
QCOMD
QQQ
QQQC
QQQD
RACE
RACEC
RACED
RBLX
RBLXC
RBLXD
REN3C
RENT3
RGTI
RGTIC
RGTID
RIO
RIOD
RIOT
RIOTC
RIOTD
RKLB
RKLBC
RKLBD
ROKU
ROKUC
ROKUD
ROST
ROSTC
ROSTD
RSP
RSPC
RSPD
RTX
RTXC
RTXD
SAN
SANC
SAND
SAP
SAPC
SAPD
SATL
SATLC
SATLD
SBS
SBSP3
SBSPC
SBSPD
SBUX
SBUXC
SBUXD
SCCO
SCHW
SCHWC
SCHWD
SDA
SDAC
SDAD
SE
SEC
SED
SH
SHC
SHD
SHEL
SHELC
SHELD
SHOP
SHOPC
SHOPD
SID
SIDC
SIDD
SIEGY
SLB
SLBC
SLBD
SLV
SLVC
SLVD
SMH
SMHC
SMHD
SMSN
SNA
SNAP
SNAPD
SNDK
SNDKC
SNDKD
SNOW
SNOWC
SNOWD
SONY
SONYC
SONYD
SPCE
SPGI
SPGIC
SPGID
SPHQ
SPHQC
SPHQD
SPOT
SPOTC
SPOTD
SPXL
SPXLC
SPXLD
SPY
SPYC
SPYD
STLA
STLAC
STLAD
STNE
STNEC
STNED
SUZ
SUZB3
SUZBC
SUZBD
SUZC
SUZD
SWKS
SWKSC
SWKSD
SYY
SYYC
SYYD
T
TC
TCOM
TCOMD
TD
TEAM
TEAMC
TEAMD
TEFO
TEFOC
TEM
TEMC
TEMD
TEN
TENC
TEND
TGT
TGTC
TGTD
TIMB
TIMBC
TIMBD
TIMS3
TIMSC
TIMSD
TJX
TJXC
TJXD
TM
TMC
TMD
TMO
TMOC
TMOD
TMUS
TMUSC
TMUSD
TQQQ
TQQQC
TQQQD
TRIP
TRIPC
TRIPD
TRVV
TRVVC
TRVVD
TSLA
TSLAC
TSLAD
TSM
TSMC
TSMD
TTE
TTEC
TTED
TV
TWLO
TXN
TXNC
TXND
TXR
TXRD
UAL
UALC
UALD
UBER
UBERC
UBERD
UGP
UGPC
UGPD
UL
ULD
UNH
UNHC
UNHD
UNP
UNPC
UNPD
UPST
UPSTC
UPSTD
URA
URAC
URAD
URBN
URBNC
URBND
USB
USO
USOD
V
VAL3C
VAL3D
VALE
VALE3
VALEC
VALED
VC
VD
VEA
VEAC
VEAD
VIG
VIGC
VIGD
VIST
VISTC
VISTD
VIV
VIVC
VIVD
VIVT3
VIVTC
VIVTD
VOD
VODC
VODD
VRSN
VRSNC
VRSND
VRTX
VRTXC
VRTXD
VST
VSTC
VSTD
VXX
VXXC
VXXD
VZ
VZC
VZD
WBO
WBOD
WEGE3
WEGEC
WEGED
WFC
WFCC
WFCD
WMT
WMTC
WMTD
XLB
XLBC
XLBD
XLC
XLCC
XLCD
XLE
XLEC
XLED
XLF
XLFC
XLFD
XLI
XLIC
XLID
XLK
XLKC
XLKD
XLP
XLPC
XLPD
XLRE
XLREC
XLRED
XLU
XLUC
XLUD
XLV
XLVC
XLVD
XLY
XLYC
XLYD
XME
XMEC
XMED
XOM
XOMC
XOMD
XP
XPC
XPD
XPEV
XPEVC
XPEVD
XROX
XROXC
XROXD
XYZ
XYZC
XYZD
YELP
YELPD
YZCA
ZM
ZMC
ZMD
`

var (
	client        = &http.Client{Timeout: httpTimeout}
	isServiceMode bool

	marketCacheMu   sync.Mutex
	marketRefreshMu sync.Mutex
	marketCacheAt   time.Time
	marketCache     []Result
	marketErrs      []string

	portfolioMu sync.Mutex
	alertMu     sync.Mutex
)

type Asset struct {
	Ticker      string
	YahooTicker string
	Name        string
	Type        string
	Sector      string
	Underlying  string
}

type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type Quote struct {
	Ticker        string
	Price         float64
	PreviousClose float64
	Currency      string
	LastUpdate    time.Time
	Candles       []Candle
	Source        string
	Error         string
}

type NewsItem struct {
	Title string
	Link  string
	Date  string
	Score int
}

type Result struct {
	Asset              Asset
	Quote              Quote
	News               []NewsItem
	RSI                float64
	StochRSI           float64
	EMA20              float64
	EMA50              float64
	EMA200             float64
	MACD               float64
	MACDSignal         float64
	MACDHist           float64
	ATR                float64
	VolumeAvg20        float64
	VolumeRatio        float64
	Support            float64
	Resistance         float64
	ChangePct          float64
	FastRSI            float64
	FastEMA20          float64
	FastEMA50          float64
	FastMACD           float64
	FastMACDSignal     float64
	FastMACDHist       float64
	UnderlyingTicker   string
	UnderlyingPrice    float64
	UnderlyingRSI      float64
	UnderlyingMACDHist float64
	UnderlyingScore    int
	CCLProxy           float64
	CCLChange20Pct     float64
	ScoreTrend         int
	ScoreMomentum      int
	ScoreVolume        int
	ScoreLocation      int
	ScoreTiming        int
	ScoreContext       int
	ScoreTechnical     int
	ScoreNews          int
	ScoreRisk          int
	ScoreFinal         int
	Recommendation     string
	Risk               string
	Confidence         string
	Reasons            []string
	AnalysisMode       string
}

type PortfolioPosition struct {
	Ticker               string    `json:"ticker"`
	AmountARS            float64   `json:"amount_ars"`
	AvgBuyPrice          float64   `json:"avg_buy_price"`
	BuyDate              string    `json:"buy_date"`
	Notes                string    `json:"notes"`
	Strategy             string    `json:"strategy,omitempty"`
	ExpectedDays         int       `json:"expected_days,omitempty"`
	InitialStopPrice     float64   `json:"initial_stop_price,omitempty"`
	InitialRiskPct       float64   `json:"initial_risk_pct,omitempty"`
	StopSource           string    `json:"stop_source,omitempty"`
	EntryATR             float64   `json:"entry_atr,omitempty"`
	EntrySupport         float64   `json:"entry_support,omitempty"`
	EntryRSI             float64   `json:"entry_rsi,omitempty"`
	EntryScore           int       `json:"entry_score,omitempty"`
	EntryUnderlyingScore int       `json:"entry_underlying_score,omitempty"`
	EntryCCLProxy        float64   `json:"entry_ccl_proxy,omitempty"`
	PlanCreatedAt        time.Time `json:"plan_created_at,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type PortfolioStore struct {
	Positions []PortfolioPosition `json:"positions"`
}

type PortfolioView struct {
	Position            PortfolioPosition
	Asset               Asset
	Result              Result
	Quantity            float64
	CurrentValue        float64
	PnL                 float64
	PnLPct              float64
	HoldingDays         int
	HoldingSessions     int
	Alert               string
	AlertClass          string
	ExitScore           int
	ExitAction          string
	ExitReasons         []string
	SuggestedSellPct    float64
	MaxPriceSinceBuy    float64
	DrawdownFromHighPct float64
	TrailingStopPrice   float64
	TrailingStopPct     float64
	InitialStopPrice    float64
	EffectiveStopPrice  float64
	InitialRiskPct      float64
	RMultiple           float64
	VelocityLabel       string
	ReturnPerDayPct     float64
	ExpectedDays        int
	Strategy            string
	StopSource          string
	ExitImmediate       bool
	BuyDateDisplay      string
	PortfolioAdvice     string
}

type AlertSnapshot struct {
	Ticker              string    `json:"ticker"`
	Alert               string    `json:"alert"`
	ExitScore           int       `json:"exit_score"`
	SuggestedSellPct    float64   `json:"suggested_sell_pct"`
	PnLPct              float64   `json:"pnl_pct"`
	TrailingStop        float64   `json:"trailing_stop"`
	Price               float64   `json:"price"`
	FirstReason         string    `json:"first_reason"`
	PendingAlert        string    `json:"pending_alert,omitempty"`
	PendingCount        int       `json:"pending_count,omitempty"`
	LastNotifiedSignal  string    `json:"last_notified_signal,omitempty"`
	LastNotifiedScore   int       `json:"last_notified_score,omitempty"`
	LastNotifiedPnLPct  float64   `json:"last_notified_pnl_pct,omitempty"`
	LastNotifiedSellPct float64   `json:"last_notified_sell_pct,omitempty"`
	LastNotifiedReason  string    `json:"last_notified_reason,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
	LastNotifiedAt      time.Time `json:"last_notified_at"`
}

type AlertHistoryItem struct {
	Time             time.Time `json:"time"`
	Ticker           string    `json:"ticker"`
	PreviousAlert    string    `json:"previous_alert"`
	NewAlert         string    `json:"new_alert"`
	ChangeReason     string    `json:"change_reason"`
	Price            float64   `json:"price"`
	PnLPct           float64   `json:"pnl_pct"`
	ExitScore        int       `json:"exit_score"`
	SuggestedSellPct float64   `json:"suggested_sell_pct"`
	Action           string    `json:"action"`
	FirstReason      string    `json:"first_reason"`
}

type Dashboard struct {
	AppName            string
	GeneratedAt        string
	Results            []Result
	Portfolio          []PortfolioView
	AlertHistory       []AlertHistoryItem
	Summary            string
	PortfolioSummary   string
	TotalInvested      float64
	TotalCurrent       float64
	TotalPnL           float64
	TotalPnLPct        float64
	PortfolioFile      string
	AlertStateFile     string
	AlertHistoryFile   string
	ServiceMode        bool
	MonitorInterval    string
	TotalAssets        int
	TotalActions       int
	TotalCedears       int
	CacheAge           string
	TelegramConfigured bool
	TelegramStatus     string
	Errors             []string
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

type yahooQuoteResponse struct {
	QuoteResponse struct {
		Result []struct {
			Symbol                     string  `json:"symbol"`
			RegularMarketPrice         float64 `json:"regularMarketPrice"`
			RegularMarketPreviousClose float64 `json:"regularMarketPreviousClose"`
			Currency                   string  `json:"currency"`
			ShortName                  string  `json:"shortName"`
			RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"quoteResponse"`
}

type rssFeed struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

type LastPriceStore struct {
	Prices map[string]Quote `json:"prices"`
}

func main() {
	ensureRuntimeLayout()
	setupLogging()

	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "test-alert":
			must(sendTestNotification())
			fmt.Println("Alerta de prueba enviada.")
			return
		case "telegram-setup":
			must(setupTelegramSecretsInteractive())
			fmt.Println("Telegram configurado y guardado cifrado en:", telegramSecretsPath())
			return
		case "telegram-test":
			must(sendTelegramMessage("✅ Felix Investment Dashboard Cloud: prueba de Telegram OK."))
			fmt.Println("Mensaje de prueba enviado por Telegram.")
			return
		case "telegram-status":
			configured, status := telegramConfigurationStatus()
			fmt.Println("Telegram configurado:", configured)
			fmt.Println("Estado:", status)
			return
		case "scan-buy-now":
			runBuyMonitorOnce(true)
			fmt.Println("Scan de compras ejecutado.")
			return
		case "serverless-scan":
			must(runServerlessScan())
			return
		case "serverless-test-alert":
			must(sendTelegramMessage("✅ Felix Investment Dashboard V3 Serverless: prueba de Telegram OK."))
			fmt.Println("Mensaje de prueba enviado por Telegram.")
			return
		}
	}

	// En la nube el proceso corre bajo systemd 24/7.
	isServiceMode = true

	if dashboardAlreadyRunning() {
		fmt.Println("El dashboard ya está ejecutándose en http://127.0.0.1:8080")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitorLoop(ctx)    // salidas / cartera cada 5 minutos
	go buyMonitorLoop(ctx) // oportunidades de compra cada 10 minutos en horario BYMA

	fmt.Println("======================================================")
	fmt.Println(" Felix Investment Dashboard CLOUD")
	fmt.Println(" Backend local: http://127.0.0.1:8080")
	fmt.Println(" Acceso remoto recomendado: Tailscale Serve")
	fmt.Println(" Activos hardcodeados:", len(loadAllAssets()))
	fmt.Println(" Cartera:", fullPath(portfolioFile))
	fmt.Println(" Telegram: telegram-setup | telegram-test | telegram-status")
	fmt.Println(" Compra: scan-buy-now")
	fmt.Println("======================================================")

	if err := startHTTPServer(ctx); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func must(err error) {
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func setupLogging() {
	logFile, err := os.OpenFile(fullPath("dashboard.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(logFile)
	}
}

func appDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func stateDir() string {
	return filepath.Join(appDir(), dataSubdir)
}

func fullPath(name string) string {
	switch name {
	case portfolioFile, alertStateFile, alertHistoryFile, lastPricesFile:
		return filepath.Join(stateDir(), name)
	default:
		return filepath.Join(appDir(), name)
	}
}

func ensureRuntimeLayout() {
	_ = os.MkdirAll(stateDir(), 0755)
}

func startHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", dashboardHandler)
	mux.HandleFunc("/api/data", apiHandler)
	mux.HandleFunc("/portfolio/save", portfolioSaveHandler)
	mux.HandleFunc("/portfolio/delete", portfolioDeleteHandler)
	mux.HandleFunc("/test-alert", testAlertHandler)
	mux.HandleFunc("/refresh", refreshHandler)
	mux.HandleFunc("/backtest", backtestHandler)

	srv := &http.Server{
		Addr:              port,
		Handler:           mux,
		ReadHeaderTimeout: 8 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv.ListenAndServe()
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	data := buildDashboard()
	tpl := template.Must(template.New("dash").Funcs(template.FuncMap{
		"money":           money,
		"pct":             pct,
		"classScore":      classScore,
		"classRec":        classRec,
		"classPnl":        classPnl,
		"formatLocalTime": formatLocalDateTime,
	}).Parse(pageHTML))
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, buf.String())
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	data := buildDashboard()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(data)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	marketCacheMu.Lock()
	marketCache = nil
	marketCacheAt = time.Time{}
	marketErrs = nil
	marketCacheMu.Unlock()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func portfolioSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/#portfolio", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	ticker := strings.ToUpper(strings.TrimSpace(r.FormValue("ticker")))
	mode := strings.TrimSpace(r.FormValue("mode"))
	amount := parseFloat(r.FormValue("amount_ars"))
	avgPrice := parseFloat(r.FormValue("avg_buy_price"))
	buyDate := strings.TrimSpace(r.FormValue("buy_date"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	strategy := normalizeStrategy(r.FormValue("strategy"))
	expectedDays, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("expected_days")))
	manualStop := parseFloat(r.FormValue("initial_stop_price"))
	if ticker == "" || amount <= 0 || avgPrice <= 0 {
		http.Redirect(w, r, "/#portfolio", http.StatusSeeOther)
		return
	}
	if buyDate == "" {
		buyDate = time.Now().Format("2006-01-02")
	}
	if expectedDays <= 0 {
		expectedDays = defaultExpectedDays(strategy)
	}

	// La foto del mercado se obtiene fuera del lock para no bloquear la cartera durante requests HTTP.
	// El stop se deriva después usando el precio promedio final.
	var planResult Result
	if a, ok := assetByTicker(ticker); ok {
		q := fetchYahooChart(a.YahooTicker, deepChartRange, deepChartInterval)
		if q.Price <= 0 && avgPrice > 0 {
			q.Price = avgPrice
		}
		planResult = resultFromQuoteV2(a, q, true)
	}

	portfolioMu.Lock()
	defer portfolioMu.Unlock()

	store := loadPortfolioUnlocked()
	now := time.Now()
	found := false
	for i := range store.Positions {
		if !strings.EqualFold(store.Positions[i].Ticker, ticker) {
			continue
		}
		pos := &store.Positions[i]
		if mode == "add" && avgPrice > 0 {
			oldQty := 0.0
			if pos.AvgBuyPrice > 0 {
				oldQty = pos.AmountARS / pos.AvgBuyPrice
			}
			newQty := amount / avgPrice
			newAmount := pos.AmountARS + amount
			newAvg := pos.AvgBuyPrice
			if oldQty+newQty > 0 {
				newAvg = ((oldQty * pos.AvgBuyPrice) + (newQty * avgPrice)) / (oldQty + newQty)
			}
			pos.AmountARS = newAmount
			pos.AvgBuyPrice = newAvg
			if notes != "" {
				pos.Notes = strings.TrimSpace(pos.Notes + " | Compra adicional " + time.Now().Format("02/01/2006") + ": " + notes)
			}
			// Una compra adicional cambia el riesgo promedio: regeneramos el plan con la nueva entrada media.
			stop, riskPct, source := deriveInitialStop(newAvg, planResult, manualStop)
			pos.Strategy = strategy
			pos.ExpectedDays = expectedDays
			pos.InitialStopPrice = stop
			pos.InitialRiskPct = riskPct
			pos.StopSource = source
			pos.EntryATR = planResult.ATR
			pos.EntrySupport = planResult.Support
			pos.EntryRSI = planResult.RSI
			pos.EntryScore = planResult.ScoreFinal
			pos.EntryUnderlyingScore = planResult.UnderlyingScore
			pos.EntryCCLProxy = planResult.CCLProxy
			pos.PlanCreatedAt = now
		} else {
			pos.AmountARS = amount
			pos.AvgBuyPrice = avgPrice
			pos.BuyDate = buyDate
			pos.Notes = notes
			stop, riskPct, source := deriveInitialStop(avgPrice, planResult, manualStop)
			pos.Strategy = strategy
			pos.ExpectedDays = expectedDays
			pos.InitialStopPrice = stop
			pos.InitialRiskPct = riskPct
			pos.StopSource = source
			pos.EntryATR = planResult.ATR
			pos.EntrySupport = planResult.Support
			pos.EntryRSI = planResult.RSI
			pos.EntryScore = planResult.ScoreFinal
			pos.EntryUnderlyingScore = planResult.UnderlyingScore
			pos.EntryCCLProxy = planResult.CCLProxy
			pos.PlanCreatedAt = now
		}
		pos.UpdatedAt = now
		found = true
		break
	}
	if !found {
		stop, riskPct, source := deriveInitialStop(avgPrice, planResult, manualStop)
		store.Positions = append(store.Positions, PortfolioPosition{
			Ticker: ticker, AmountARS: amount, AvgBuyPrice: avgPrice, BuyDate: buyDate, Notes: notes,
			Strategy: strategy, ExpectedDays: expectedDays, InitialStopPrice: stop, InitialRiskPct: riskPct,
			StopSource: source, EntryATR: planResult.ATR, EntrySupport: planResult.Support, EntryRSI: planResult.RSI,
			EntryScore: planResult.ScoreFinal, EntryUnderlyingScore: planResult.UnderlyingScore, EntryCCLProxy: planResult.CCLProxy,
			PlanCreatedAt: now, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := savePortfolioUnlocked(store); err != nil {
		log.Println("Error guardando portfolio:", err)
	}
	http.Redirect(w, r, "/#portfolio", http.StatusSeeOther)
}

func portfolioDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/#portfolio", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	ticker := strings.ToUpper(strings.TrimSpace(r.FormValue("ticker")))

	portfolioMu.Lock()
	defer portfolioMu.Unlock()

	store := loadPortfolioUnlocked()
	out := []PortfolioPosition{}
	for _, p := range store.Positions {
		if !strings.EqualFold(p.Ticker, ticker) {
			out = append(out, p)
		}
	}
	store.Positions = out
	if err := savePortfolioUnlocked(store); err != nil {
		log.Println("Error guardando portfolio:", err)
	}
	http.Redirect(w, r, "/#portfolio", http.StatusSeeOther)
}

func testAlertHandler(w http.ResponseWriter, r *http.Request) {
	if err := sendTestNotification(); err != nil {
		log.Println("Error enviando alerta de prueba:", err)
	}
	http.Redirect(w, r, "/#portfolio", http.StatusSeeOther)
}

func buildDashboard() Dashboard {
	allAssets := loadAllAssets()
	portfolioStore := loadPortfolio()
	portfolioTickers := map[string]bool{}
	for _, p := range portfolioStore.Positions {
		portfolioTickers[strings.ToUpper(strings.TrimSpace(p.Ticker))] = true
	}
	results, errorsList, cacheAt := getMarketResultsCached(allAssets, portfolioTickers)
	portfolio := buildPortfolioViews(portfolioStore, results)
	totalInv, totalCur := 0.0, 0.0
	for _, p := range portfolio {
		totalInv += p.Position.AmountARS
		totalCur += p.CurrentValue
	}
	totalPnL := totalCur - totalInv
	totalPnLPct := 0.0
	if totalInv > 0 {
		totalPnLPct = totalPnL / totalInv * 100
	}
	totalActions, totalCedears := 0, 0
	for _, a := range allAssets {
		if a.Type == "ACCION" {
			totalActions++
		}
		if a.Type == "CEDEAR" {
			totalCedears++
		}
	}
	cacheAge := "-"
	if !cacheAt.IsZero() {
		cacheAge = time.Since(cacheAt).Round(time.Second).String()
	}
	tgConfigured, tgStatus := telegramConfigurationStatus()
	return Dashboard{AppName: appName, GeneratedAt: time.Now().Format("02/01/2006 15:04:05"), Results: results, Portfolio: portfolio, AlertHistory: loadAlertHistory(), Summary: buildSummary(results), PortfolioSummary: buildPortfolioSummary(portfolio), TotalInvested: totalInv, TotalCurrent: totalCur, TotalPnL: totalPnL, TotalPnLPct: totalPnLPct, PortfolioFile: fullPath(portfolioFile), AlertStateFile: fullPath(alertStateFile), AlertHistoryFile: fullPath(alertHistoryFile), ServiceMode: isServiceMode, MonitorInterval: monitorInterval.String(), TotalAssets: len(allAssets), TotalActions: totalActions, TotalCedears: totalCedears, CacheAge: cacheAge, TelegramConfigured: tgConfigured, TelegramStatus: tgStatus, Errors: errorsList}
}

func getMarketResultsCached(allAssets []Asset, portfolioTickers map[string]bool) ([]Result, []string, time.Time) {
	marketCacheMu.Lock()
	if marketCache != nil && time.Since(marketCacheAt) < cacheTTL {
		results := append([]Result(nil), marketCache...)
		errs := append([]string(nil), marketErrs...)
		at := marketCacheAt
		marketCacheMu.Unlock()
		return results, errs, at
	}
	marketCacheMu.Unlock()

	// Evita dos scans completos simultáneos si se refresca la web al mismo tiempo.
	marketRefreshMu.Lock()
	defer marketRefreshMu.Unlock()

	marketCacheMu.Lock()
	if marketCache != nil && time.Since(marketCacheAt) < cacheTTL {
		results := append([]Result(nil), marketCache...)
		errs := append([]string(nil), marketErrs...)
		at := marketCacheAt
		marketCacheMu.Unlock()
		return results, errs, at
	}
	marketCacheMu.Unlock()

	results, errs := analyzeAll(allAssets, portfolioTickers)
	marketCacheMu.Lock()
	marketCache = append([]Result(nil), results...)
	marketErrs = append([]string(nil), errs...)
	marketCacheAt = time.Now()
	at := marketCacheAt
	marketCacheMu.Unlock()
	return results, errs, at
}

func analyzeAll(allAssets []Asset, portfolioTickers map[string]bool) ([]Result, []string) {
	quotes := fetchYahooQuotesBatch(allAssets)
	results := make([]Result, len(allAssets))
	deepJobs := []int{}
	for i, a := range allAssets {
		q := quotes[a.YahooTicker]
		if q.Ticker == "" {
			q = Quote{Ticker: a.YahooTicker, LastUpdate: time.Now(), Source: "Yahoo batch", Error: "sin dato batch"}
		}
		if shouldDeepAnalyze(a, portfolioTickers) {
			deepJobs = append(deepJobs, i)
			results[i] = analyzeLight(a, q)
			results[i].AnalysisMode = "Pendiente profundo"
		} else {
			results[i] = analyzeLight(a, q)
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, chartWorkers)
	for _, idx := range deepJobs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			if portfolioTickers[strings.ToUpper(allAssets[i].Ticker)] {
				results[i] = analyzeDeepForMonitor(allAssets[i], quotes[allAssets[i].YahooTicker])
			} else {
				results[i] = analyzeDeep(allAssets[i], quotes[allAssets[i].YahooTicker])
			}
			<-sem
		}(idx)
	}
	wg.Wait()
	for i := range results {
		if results[i].AnalysisMode == "" {
			results[i].AnalysisMode = "Liviano"
		}
	}
	updateLastPrices(results)
	errorsList := []string{}
	for _, r := range results {
		if r.Quote.Error != "" && strings.Contains(r.AnalysisMode, "Profundo") {
			errorsList = append(errorsList, r.Asset.Ticker+": "+r.Quote.Error)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ScoreFinal > results[j].ScoreFinal })
	return results, errorsList
}

func shouldDeepAnalyze(a Asset, portfolioTickers map[string]bool) bool {
	if portfolioTickers[strings.ToUpper(a.Ticker)] {
		return true
	}
	if a.Type == "ACCION" {
		return true
	}
	return isImportantCedear(a.Ticker)
}

func fetchYahooQuotesBatch(assets []Asset) map[string]Quote {
	symbols := []string{}
	seen := map[string]bool{}
	for _, a := range assets {
		if a.YahooTicker != "" && !seen[a.YahooTicker] {
			seen[a.YahooTicker] = true
			symbols = append(symbols, a.YahooTicker)
		}
	}
	out := make(map[string]Quote, len(symbols))
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan []string)
	for w := 0; w < quoteWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range jobs {
				partial := fetchYahooQuoteBatch(batch)
				mu.Lock()
				for k, v := range partial {
					out[k] = v
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < len(symbols); i += quoteBatchSize {
		end := i + quoteBatchSize
		if end > len(symbols) {
			end = len(symbols)
		}
		jobs <- symbols[i:end]
	}
	close(jobs)
	wg.Wait()
	return out
}

func fetchYahooQuoteBatch(symbols []string) map[string]Quote {
	out := map[string]Quote{}
	if len(symbols) == 0 {
		return out
	}
	u := "https://query1.finance.yahoo.com/v7/finance/quote?symbols=" + url.QueryEscape(strings.Join(symbols, ","))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 FelixDashboard/2.0")
	resp, err := client.Do(req)
	if err != nil {
		for _, s := range symbols {
			out[s] = quoteWithLastFallback(s, "Yahoo quote batch", err.Error())
		}
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		for _, s := range symbols {
			out[s] = quoteWithLastFallback(s, "Yahoo quote batch", fmt.Sprintf("status %d", resp.StatusCode))
		}
		return out
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return out
	}
	var data yahooQuoteResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return out
	}
	for _, r := range data.QuoteResponse.Result {
		out[r.Symbol] = Quote{Ticker: r.Symbol, Price: r.RegularMarketPrice, PreviousClose: r.RegularMarketPreviousClose, Currency: r.Currency, LastUpdate: time.Now(), Source: "Yahoo quote batch"}
	}
	for _, s := range symbols {
		if _, ok := out[s]; !ok {
			out[s] = quoteWithLastFallback(s, "Yahoo quote batch", "sin resultado")
		}
	}
	return out
}

func quoteWithLastFallback(ticker, source, errText string) Quote {
	last := loadLastPrices()
	if q, ok := last.Prices[ticker]; ok && q.Price > 0 {
		q.Source = source + " + último precio válido cacheado"
		q.Error = errText + "; usando último precio válido"
		return q
	}
	return Quote{Ticker: ticker, Source: source, LastUpdate: time.Now(), Error: errText}
}

func loadAllAssets() []Asset {
	actions := tickersFromRaw(accionesHardcoded)
	cedears := tickersFromRaw(cedearsHardcoded)
	all := []Asset{}
	seen := map[string]bool{}
	for _, t := range actions {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		all = append(all, Asset{Ticker: t, YahooTicker: yahooTicker(t), Name: "Acción " + t, Type: "ACCION", Sector: guessSectorByTicker(t, "ACCION")})
	}
	for _, t := range cedears {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		all = append(all, Asset{Ticker: t, YahooTicker: yahooTicker(t), Name: "CEDEAR " + t, Type: "CEDEAR", Sector: guessSectorByTicker(t, "CEDEAR"), Underlying: guessUnderlying(t)})
	}
	return all
}

func tickersFromRaw(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func yahooTicker(t string) string { return strings.ToUpper(strings.TrimSpace(t)) + ".BA" }

func guessUnderlying(ticker string) string {
	t := strings.ToUpper(strings.TrimSpace(ticker))
	// Primero respetamos tickers reales que naturalmente terminan en C/D (BAC, AMD, etc.).
	// Solo quitamos el sufijo C/D cuando el símbolo completo NO es un subyacente conocido
	// y el símbolo sin la última letra sí lo es. Esto evita el bug histórico AMD -> AM o BAC -> BA.
	if isKnownUnderlyingTickerV2(t) {
		return t
	}
	if len(t) > 2 && (strings.HasSuffix(t, "C") || strings.HasSuffix(t, "D")) {
		base := t[:len(t)-1]
		if isKnownUnderlyingTickerV2(base) {
			return base
		}
	}
	return t
}

func guessSectorByTicker(ticker string, assetType string) string {
	t := strings.ToUpper(ticker)
	if assetType == "ACCION" {
		switch t {
		case "GGAL", "BMA", "BBAR", "SUPV", "VALO":
			return "Bancos / Financiero Argentina"
		case "PAMP", "YPFD", "CEPU", "TGSU2", "TGNO4", "TRAN", "EDN", "METR":
			return "Energía / Utilities Argentina"
		case "ALUA", "TXAR":
			return "Materiales / Industria Argentina"
		case "BYMA":
			return "Mercado financiero"
		case "LOMA":
			return "Construcción / Cemento"
		case "CRES":
			return "Agro / Real Estate"
		case "TECO2":
			return "Telecomunicaciones"
		default:
			return "Acciones Argentina"
		}
	}
	switch guessUnderlying(t) {
	case "AAPL", "MSFT", "GOOGL", "GOOG", "META", "AMZN", "ADBE", "ORCL", "CRM", "NFLX", "SHOP", "UBER", "SPOT", "NOW", "TEAM", "DOCU", "SNOW":
		return "Tecnología USA"
	case "NVDA", "AMD", "AVGO", "QCOM", "INTC", "MU", "TSM", "ASML", "ADI", "LRCX", "AMAT", "TXN":
		return "Semiconductores"
	case "KO", "PEP", "WMT", "MCD", "SBUX", "PG", "COST", "NKE", "MDLZ":
		return "Consumo USA"
	case "JPM", "BAC", "C", "WFC", "GS", "MS", "V", "MA", "AXP", "PYPL", "SCHW", "BK":
		return "Financiero USA"
	case "XOM", "CVX", "SHEL", "BP", "OXY", "SLB", "HAL", "TTE":
		return "Energía USA"
	case "PFE", "JNJ", "MRK", "ABBV", "ABT", "LLY", "UNH", "BMY", "MDT", "NVS", "AZN", "GILD":
		return "Salud / Pharma"
	case "SPY", "QQQ", "DIA", "IWM", "EWZ", "EEM", "ARKK", "ACWI", "GLD", "SLV", "XLF", "XLK", "XLV", "XLY", "XLP", "XLU", "XLI", "XLE", "SMH", "IBIT", "ETHA", "TQQQ", "SPXL", "VXX", "USO", "URA", "CIBR", "IBB", "IVE", "IVV", "IVW", "VEA", "VIG":
		return "ETF"
	case "TSLA", "F", "GM", "TM", "HMC", "MBG":
		return "Autos"
	case "MELI", "BABA", "SE", "PDD", "EBAY", "ETSY":
		return "E-commerce"
	case "COIN", "MSTR", "RIOT", "HUT":
		return "Crypto / Blockchain"
	case "BA", "RTX", "LMT", "ITA", "AAL", "DAL", "UAL":
		return "Aerolíneas / Defensa"
	default:
		return "CEDEAR"
	}
}

func analyzeLight(a Asset, q Quote) Result {
	newsScore := 0
	change := 0.0
	if q.Price > 0 && q.PreviousClose > 0 {
		change = ((q.Price - q.PreviousClose) / q.PreviousClose) * 100
	}
	score := 0
	reasons := []string{}
	if q.Price <= 0 {
		score -= 6
		reasons = append(reasons, "No hay precio válido desde Yahoo; validar manualmente en Balanz.")
	} else {
		if change > 3 {
			score += 1
			reasons = append(reasons, "Variación positiva diaria relevante.")
		} else if change < -3 {
			score -= 1
			reasons = append(reasons, "Variación negativa diaria relevante.")
		} else {
			reasons = append(reasons, "Análisis liviano: se usa precio/variación batch para máxima velocidad.")
		}
	}
	riskScoreValue, riskText, riskReasons := riskScore(a, 0, q)
	reasons = append(reasons, riskReasons...)
	final := score + newsScore + riskScoreValue
	return Result{Asset: a, Quote: q, News: nil, ChangePct: change, ScoreTechnical: score, ScoreNews: newsScore, ScoreRisk: riskScoreValue, ScoreFinal: final, Recommendation: recommendation(final), Risk: riskText, Confidence: confidence(q, 0), Reasons: uniqueStrings(reasons), AnalysisMode: "Liviano"}
}

func analyzeDeep(a Asset, batchQuote Quote) Result {
	return analyzeDeepWithNews(a, batchQuote, true, false)
}

func analyzeDeepForMonitor(a Asset, batchQuote Quote) Result {
	return analyzeDeepWithNews(a, batchQuote, false, true)
}

func analyzeDeepWithNews(a Asset, batchQuote Quote, includeNews bool, forceContext bool) Result {
	q := fetchYahooChart(a.YahooTicker, deepChartRange, deepChartInterval)
	if q.Price <= 0 && batchQuote.Price > 0 {
		q.Price = batchQuote.Price
		q.PreviousClose = batchQuote.PreviousClose
		q.Currency = batchQuote.Currency
		q.Source = batchQuote.Source + " + chart incompleto"
		if q.Error == "" {
			q.Error = "chart incompleto"
		}
	}
	news := []NewsItem{}
	if includeNews && (a.Type == "ACCION" || isImportantCedear(a.Ticker)) {
		news = fetchNews(a)
	}
	newsScore := clampInt(scoreNews(news), -1, 1)
	rsi := calcRSI(q.Candles, 14)
	stoch := calcStochRSI(q.Candles, 14)
	ema20 := calcEMA(q.Candles, 20)
	ema50 := calcEMA(q.Candles, 50)
	ema200 := calcEMA(q.Candles, 200)
	macd, macdSignal, macdHist := calcMACD(q.Candles)
	atr := calcATR(q.Candles, 14)
	volAvg := avgVolume(q.Candles, 20)
	support, resistance := supportResistance(q.Candles, 60)
	volumeRatio := 0.0
	if volAvg > 0 {
		volumeRatio = lastVolume(q.Candles) / volAvg
	}
	change := 0.0
	if q.Price > 0 && q.PreviousClose > 0 {
		change = (q.Price - q.PreviousClose) / q.PreviousClose * 100
	}

	b, reasons := technicalScoreV2(q, rsi, stoch, ema20, ema50, ema200, macd, macdSignal, macdHist, atr, volumeRatio, support, resistance, nil)
	riskScoreValue, riskText, riskReasons := riskScore(a, rsi, q)
	reasons = append(reasons, riskReasons...)
	r := Result{
		Asset: a, Quote: q, News: news, RSI: rsi, StochRSI: stoch, EMA20: ema20, EMA50: ema50, EMA200: ema200,
		MACD: macd, MACDSignal: macdSignal, MACDHist: macdHist, ATR: atr, VolumeAvg20: volAvg, VolumeRatio: volumeRatio,
		Support: support, Resistance: resistance, ChangePct: change,
		ScoreTrend: b.Trend, ScoreMomentum: b.Momentum, ScoreVolume: b.Volume, ScoreLocation: b.Location,
		ScoreTechnical: b.Total, ScoreNews: newsScore, ScoreRisk: riskScoreValue, Risk: riskText,
		Confidence: confidence(q, volAvg), Reasons: reasons, AnalysisMode: "Profundo V2",
	}
	enrichResultV2(a, &r, forceContext)
	// Noticias se mantienen como contexto, pero con menos peso que la estructura técnica.
	if newsScore > 1 {
		r.Reasons = append(r.Reasons, "Noticias/sensación positiva detectada; se usa como contexto, no como disparador único.")
	}
	if newsScore < -1 {
		r.Reasons = append(r.Reasons, "Noticias/sensación negativa detectada; se usa como contexto, no como disparador único.")
	}
	if len(news) == 0 {
		r.Reasons = append(r.Reasons, "Sin noticias leídas o no se consultaron para este activo.")
	}
	r.ScoreFinal = r.ScoreTechnical + r.ScoreContext + r.ScoreNews + r.ScoreRisk
	r.Recommendation = recommendation(r.ScoreFinal)
	r.Reasons = uniqueStrings(r.Reasons)
	return r
}

func isImportantCedear(ticker string) bool {
	t := guessUnderlying(strings.ToUpper(ticker))
	important := map[string]bool{"AAPL": true, "MSFT": true, "NVDA": true, "AMD": true, "GOOGL": true, "GOOG": true, "AMZN": true, "META": true, "TSLA": true, "MELI": true, "SPY": true, "QQQ": true, "KO": true, "PEP": true, "V": true, "MA": true, "JPM": true, "XOM": true, "PLTR": true, "MSTR": true, "COIN": true, "IBIT": true, "ETHA": true, "TQQQ": true, "SPXL": true}
	return important[t]
}

func riskScore(a Asset, rsi float64, q Quote) (int, string, []string) {
	score := 0
	risk := "Medio"
	reasons := []string{}
	if a.Type == "CEDEAR" {
		score -= 1
		reasons = append(reasons, "CEDEAR: además del activo subyacente, influye CCL y liquidez local.")
	}
	sector := strings.ToLower(a.Sector)
	if strings.Contains(sector, "bancos") {
		score -= 1
		reasons = append(reasons, "Banco/financiero: sensible a tasas, política y riesgo país.")
	}
	if strings.Contains(sector, "semiconductores") {
		reasons = append(reasons, "Semiconductores: sensible a IA, China, tasas y valuaciones.")
	}
	if strings.Contains(sector, "energía") {
		reasons = append(reasons, "Energía: sensible a tarifas, petróleo, gas y regulación.")
	}
	if strings.Contains(sector, "crypto") {
		score -= 1
		reasons = append(reasons, "Activo vinculado a crypto/blockchain: volatilidad elevada.")
	}
	if rsi > 80 {
		score -= 1
		risk = "Alto"
	}
	if q.Error != "" {
		score -= 1
		risk = "Medio/Alto"
		reasons = append(reasons, "Fuente online con error o incompleta; validar en Balanz antes de operar.")
	}
	return score, risk, reasons
}

func recommendation(score int) string {
	switch {
	case score >= 9:
		return "COMPRAR MUCHO"
	case score >= 5:
		return "COMPRAR POCO"
	case score >= -4:
		return "NO HACER NADA"
	case score >= -8:
		return "VENDER POCO"
	default:
		return "VENDER MUCHO"
	}
}

func confidence(q Quote, volAvg float64) string {
	conf := 0
	if q.Price > 0 {
		conf += 2
	}
	if len(q.Candles) >= 100 {
		conf += 2
	}
	if volAvg > 0 {
		conf++
	}
	if q.Error != "" {
		conf -= 2
	}
	switch {
	case conf >= 5:
		return "Alta"
	case conf >= 3:
		return "Media"
	default:
		return "Baja"
	}
}

func fetchYahooChartRaw(ticker, period, interval string) Quote {
	q := Quote{Ticker: ticker, LastUpdate: time.Now(), Source: "Yahoo chart JSON público"}
	u := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?range=%s&interval=%s", url.QueryEscape(ticker), url.QueryEscape(period), url.QueryEscape(interval))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 FelixDashboard/2.0")
	resp, err := client.Do(req)
	if err != nil {
		return quoteWithLastFallback(ticker, "Yahoo chart JSON público", "error HTTP Yahoo: "+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return quoteWithLastFallback(ticker, "Yahoo chart JSON público", fmt.Sprintf("Yahoo status %d", resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return quoteWithLastFallback(ticker, "Yahoo chart JSON público", "error leyendo Yahoo: "+err.Error())
	}
	var data yahooChartResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return quoteWithLastFallback(ticker, "Yahoo chart JSON público", "error JSON Yahoo: "+err.Error())
	}
	if data.Chart.Error != nil {
		return quoteWithLastFallback(ticker, "Yahoo chart JSON público", "Yahoo chart error")
	}
	if len(data.Chart.Result) == 0 {
		return quoteWithLastFallback(ticker, "Yahoo chart JSON público", "Yahoo sin resultados")
	}
	res := data.Chart.Result[0]
	q.Currency = res.Meta.Currency
	q.Price = res.Meta.RegularMarketPrice
	q.PreviousClose = res.Meta.ChartPreviousClose
	if len(res.Timestamp) == 0 || len(res.Indicators.Quote) == 0 {
		if q.Price <= 0 {
			q.Error = "Yahoo sin velas"
		}
		return q
	}
	quote := res.Indicators.Quote[0]
	n := minInt(len(res.Timestamp), len(quote.Close))
	for i := 0; i < n; i++ {
		if quote.Close[i] == nil {
			continue
		}
		c := Candle{Time: time.Unix(res.Timestamp[i], 0), Close: *quote.Close[i]}
		if i < len(quote.Open) && quote.Open[i] != nil {
			c.Open = *quote.Open[i]
		}
		if i < len(quote.High) && quote.High[i] != nil {
			c.High = *quote.High[i]
		}
		if i < len(quote.Low) && quote.Low[i] != nil {
			c.Low = *quote.Low[i]
		}
		if i < len(quote.Volume) && quote.Volume[i] != nil {
			c.Volume = float64(*quote.Volume[i])
		}
		q.Candles = append(q.Candles, c)
	}
	if q.Price <= 0 && len(q.Candles) > 0 {
		q.Price = q.Candles[len(q.Candles)-1].Close
	}
	if q.PreviousClose <= 0 && len(q.Candles) > 1 {
		q.PreviousClose = q.Candles[len(q.Candles)-2].Close
	}
	return q
}

func fetchNewsUncached(a Asset) []NewsItem {
	query := a.Ticker + " " + a.Name + " stock market news finance"
	u := "https://news.google.com/rss/search?q=" + url.QueryEscape(query) + "&hl=es-419&gl=AR&ceid=AR:es-419"
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 FelixDashboard/2.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}
	items := []NewsItem{}
	for i, it := range feed.Channel.Items {
		if i >= 5 {
			break
		}
		title := htmlUnescape(it.Title)
		items = append(items, NewsItem{Title: title, Link: it.Link, Date: it.PubDate, Score: sentimentScore(title)})
	}
	return items
}

func scoreNews(news []NewsItem) int {
	total := 0
	for _, n := range news {
		total += n.Score
	}
	if total > 4 {
		return 3
	}
	if total > 1 {
		return 1
	}
	if total < -4 {
		return -3
	}
	if total < -1 {
		return -1
	}
	return 0
}

func sentimentScore(s string) int {
	t := strings.ToLower(s)
	positive := []string{"sube", "subió", "alza", "ganancia", "ganancias", "récord", "record", "mejora", "crece", "crecimiento", "bullish", "beat", "supera", "compra", "favorable", "acuerdo", "inversión", "inversion"}
	negative := []string{"cae", "cayó", "baja", "pérdida", "perdida", "crisis", "riesgo", "demanda", "juicio", "recorte", "bearish", "miss", "debil", "débil", "inflación", "inflacion", "guerra", "conflicto", "sanciones", "default", "deuda", "investigación", "investigacion"}
	score := 0
	for _, w := range positive {
		if strings.Contains(t, w) {
			score++
		}
	}
	for _, w := range negative {
		if strings.Contains(t, w) {
			score--
		}
	}
	return score
}

func loadPortfolio() PortfolioStore {
	portfolioMu.Lock()
	defer portfolioMu.Unlock()
	return loadPortfolioUnlocked()
}

func loadPortfolioUnlocked() PortfolioStore {
	b, err := os.ReadFile(fullPath(portfolioFile))
	if err != nil {
		return PortfolioStore{Positions: []PortfolioPosition{}}
	}
	var s PortfolioStore
	if err := json.Unmarshal(b, &s); err != nil {
		log.Println("portfolio.json inválido:", err)
		return PortfolioStore{Positions: []PortfolioPosition{}}
	}
	return s
}

func savePortfolioUnlocked(s PortfolioStore) error {
	return writeJSONAtomic(fullPath(portfolioFile), s)
}

func savePortfolio(s PortfolioStore) error {
	portfolioMu.Lock()
	defer portfolioMu.Unlock()
	return savePortfolioUnlocked(s)
}

func buildPortfolioViews(store PortfolioStore, results []Result) []PortfolioView {
	byTicker := map[string]Result{}
	for _, r := range results {
		byTicker[strings.ToUpper(r.Asset.Ticker)] = r
	}
	views := []PortfolioView{}
	for _, p := range store.Positions {
		ticker := strings.ToUpper(strings.TrimSpace(p.Ticker))
		r, ok := byTicker[ticker]
		if !ok {
			continue
		}
		entry := p.AvgBuyPrice
		if entry <= 0 {
			entry = r.Quote.Price
		}
		qty := 0.0
		if entry > 0 {
			qty = p.AmountARS / entry
		}
		cur := qty * r.Quote.Price
		pnl := cur - p.AmountARS
		pnlPct := 0.0
		if p.AmountARS > 0 {
			pnlPct = pnl / p.AmountARS * 100
		}
		days := holdingDays(p.BuyDate)
		exit := analyzePortfolioExitV2(p, r, pnlPct, days)
		views = append(views, PortfolioView{
			Position:            p,
			Asset:               r.Asset,
			Result:              r,
			Quantity:            qty,
			CurrentValue:        cur,
			PnL:                 pnl,
			PnLPct:              pnlPct,
			HoldingDays:         days,
			HoldingSessions:     exit.HoldingSessions,
			Alert:               exit.Signal,
			AlertClass:          exit.Class,
			ExitScore:           exit.Score,
			ExitAction:          exit.Action,
			ExitReasons:         exit.Reasons,
			SuggestedSellPct:    exit.SuggestedSellPct,
			MaxPriceSinceBuy:    exit.MaxPriceSinceBuy,
			DrawdownFromHighPct: exit.DrawdownFromHighPct,
			TrailingStopPrice:   exit.TrailingStopPrice,
			TrailingStopPct:     exit.TrailingStopPct,
			InitialStopPrice:    exit.InitialStopPrice,
			EffectiveStopPrice:  exit.EffectiveStopPrice,
			InitialRiskPct:      exit.InitialRiskPct,
			RMultiple:           exit.RMultiple,
			VelocityLabel:       exit.VelocityLabel,
			ReturnPerDayPct:     exit.ReturnPerDayPct,
			ExpectedDays:        exit.ExpectedDays,
			Strategy:            exit.Strategy,
			StopSource:          exit.StopSource,
			ExitImmediate:       exit.Immediate,
			BuyDateDisplay:      displayDate(p.BuyDate),
			PortfolioAdvice:     portfolioAdvice(r, exit),
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].CurrentValue > views[j].CurrentValue })
	return views
}

type PortfolioExit struct {
	Signal              string
	Class               string
	Score               int
	Action              string
	Reasons             []string
	SuggestedSellPct    float64
	MaxPriceSinceBuy    float64
	DrawdownFromHighPct float64
	TrailingStopPrice   float64
	TrailingStopPct     float64
	InitialStopPrice    float64
	EffectiveStopPrice  float64
	InitialRiskPct      float64
	RMultiple           float64
	VelocityLabel       string
	ReturnPerDayPct     float64
	ExpectedDays        int
	HoldingSessions     int
	Strategy            string
	StopSource          string
	Immediate           bool
}

func portfolioAdvice(r Result, exit PortfolioExit) string {
	if exit.Signal == "PROTEGER CAPITAL" || strings.Contains(exit.Signal, "VENDER") || strings.Contains(exit.Signal, "TOMAR") || strings.Contains(exit.Signal, "SALIR") || strings.Contains(exit.Signal, "STOP") || strings.Contains(exit.Signal, "ROTAR") {
		return exit.Signal
	}
	switch r.Recommendation {
	case "COMPRAR MUCHO":
		return "ACUMULAR MÁS - VALIDAR EN BALANZ"
	case "COMPRAR POCO":
		return "ACUMULAR POCO / MANTENER"
	case "VENDER MUCHO", "VENDER POCO":
		return "NO AGREGAR - VIGILAR"
	default:
		return "MANTENER"
	}
}

func maxPriceSinceBuy(c []Candle, buyDate string, currentPrice float64) float64 {
	maxPrice := currentPrice
	var start time.Time
	if buyDate != "" {
		if t, err := time.Parse("2006-01-02", buyDate); err == nil {
			start = t
		}
	}
	for _, candle := range c {
		if !start.IsZero() && candle.Time.Before(start) {
			continue
		}
		candidate := candle.High
		if candidate <= 0 {
			candidate = candle.Close
		}
		if candidate > maxPrice {
			maxPrice = candidate
		}
	}
	return maxPrice
}

func isLeveragedTicker(t string) bool {
	t = strings.ToUpper(strings.TrimSpace(t))
	switch t {
	case "SPXL", "TQQQ", "SQQQ", "SOXL", "SOXS", "TECL", "FNGU", "LABU", "YINN", "UDOW", "UPRO", "BITX":
		return true
	default:
		return false
	}
}

func buildPortfolioSummary(p []PortfolioView) string {
	if len(p) == 0 {
		return "Sin activos cargados"
	}
	take, protect, hold := 0, 0, 0
	for _, x := range p {
		if strings.Contains(x.Alert, "TOMAR") || strings.Contains(x.Alert, "VENDER") || strings.Contains(x.Alert, "SALIR") || strings.Contains(x.Alert, "STOP") || strings.Contains(x.Alert, "ROTAR") {
			take++
		} else if strings.Contains(x.Alert, "PROTEGER") {
			protect++
		} else {
			hold++
		}
	}
	return fmt.Sprintf("Activos: %d | Acción/salida: %d | Proteger: %d | Hold: %d", len(p), take, protect, hold)
}

func monitorLoop(ctx context.Context) {
	// En cloud evitamos martillar Yahoo cuando BYMA está cerrado.
	// Las señales de salida sólo son accionables mientras el mercado local está abierto.
	if isArgentinaMarketWindow() {
		runMonitorOnce()
	}
	t := time.NewTicker(monitorInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if isArgentinaMarketWindow() {
				runMonitorOnce()
			}
		}
	}
}

func runMonitorOnce() []PortfolioView {
	store := loadPortfolio()
	if len(store.Positions) == 0 {
		return nil
	}
	portfolio := buildPortfolioMonitorViews(store)
	alertMu.Lock()
	defer alertMu.Unlock()
	state := loadAlertStateUnlocked()
	history := loadAlertHistoryUnlocked()
	changed := false
	for _, p := range portfolio {
		now := time.Now().Local()
		prev, hadPrev := state[strings.ToUpper(p.Position.Ticker)]
		snap := AlertSnapshot{
			Ticker: strings.ToUpper(p.Position.Ticker), Alert: p.Alert, ExitScore: p.ExitScore,
			SuggestedSellPct: p.SuggestedSellPct, PnLPct: p.PnLPct, TrailingStop: p.EffectiveStopPrice,
			Price: p.Result.Quote.Price, FirstReason: firstReason(p.ExitReasons), UpdatedAt: now,
			LastNotifiedAt: prev.LastNotifiedAt, LastNotifiedSignal: prev.LastNotifiedSignal,
			LastNotifiedScore: prev.LastNotifiedScore, LastNotifiedPnLPct: prev.LastNotifiedPnLPct,
			LastNotifiedSellPct: prev.LastNotifiedSellPct, LastNotifiedReason: prev.LastNotifiedReason,
		}

		if snap.Alert == "HOLD - DEJAR CORRER" || snap.Alert == "SIN PRECIO - VALIDAR" {
			snap.PendingAlert = ""
			snap.PendingCount = 0
			snap.LastNotifiedSignal = ""
			state[snap.Ticker] = snap
			changed = true
			continue
		}

		// Stops duros se notifican ya. El resto necesita dos lecturas consecutivas (10 min aprox.)
		// para reducir falsos positivos por ruido intradía.
		if !p.ExitImmediate {
			if hadPrev && prev.PendingAlert == snap.Alert {
				snap.PendingCount = prev.PendingCount + 1
			} else {
				snap.PendingCount = 1
			}
			snap.PendingAlert = snap.Alert
			if snap.PendingCount < 2 {
				state[snap.Ticker] = snap
				changed = true
				continue
			}
		}

		compare := prev
		if compare.LastNotifiedSignal != "" {
			compare.Alert = compare.LastNotifiedSignal
			compare.ExitScore = compare.LastNotifiedScore
			compare.PnLPct = compare.LastNotifiedPnLPct
			compare.SuggestedSellPct = compare.LastNotifiedSellPct
			compare.FirstReason = compare.LastNotifiedReason
		} else if compare.LastNotifiedAt.IsZero() {
			compare.Alert = ""
		}
		mustNotify, why := shouldNotifyAlert(compare, snap, hadPrev && compare.Alert != "")
		if !p.ExitImmediate && why == "cambio de señal" {
			why = "señal confirmada en 2 lecturas"
		}
		if hadPrev && compare.Alert != "" && !prev.LastNotifiedAt.IsZero() && time.Since(prev.LastNotifiedAt) < alertCooldown && !isMoreUrgent(compare.Alert, snap.Alert) && why != "señal confirmada en 2 lecturas" && !p.ExitImmediate {
			mustNotify = false
		}
		if mustNotify {
			msg := fmt.Sprintf("%s: %s\nCambio: %s\nPrecio actual: %s\nRendimiento: %.2f%%\nR: %.2fR\nRuedas: %d/%d\nStop efectivo: %s\nSugerencia: %s\n\nMotivo principal: %s", p.Position.Ticker, p.Alert, why, money(p.Result.Quote.Price), p.PnLPct, p.RMultiple, p.HoldingSessions, p.ExpectedDays, money(p.EffectiveStopPrice), p.ExitAction, firstReason(p.ExitReasons))
			if err := notifyInvestmentAlert(p, why, msg); err != nil {
				log.Println("Error enviando notificación:", err)
			}
			snap.LastNotifiedAt = now
			snap.LastNotifiedSignal = snap.Alert
			snap.LastNotifiedScore = snap.ExitScore
			snap.LastNotifiedPnLPct = snap.PnLPct
			snap.LastNotifiedSellPct = snap.SuggestedSellPct
			snap.LastNotifiedReason = snap.FirstReason
			history = append([]AlertHistoryItem{{Time: now, Ticker: snap.Ticker, PreviousAlert: compare.Alert, NewAlert: snap.Alert, ChangeReason: why, Price: snap.Price, PnLPct: snap.PnLPct, ExitScore: snap.ExitScore, SuggestedSellPct: snap.SuggestedSellPct, Action: p.ExitAction, FirstReason: snap.FirstReason}}, history...)
			if len(history) > 100 {
				history = history[:100]
			}
			changed = true
		}
		state[snap.Ticker] = snap
	}
	if changed {
		saveAlertStateUnlocked(state)
		saveAlertHistoryUnlocked(history)
	} else {
		saveAlertStateUnlocked(state)
	}
	return portfolio
}

func shouldNotifyAlert(prev AlertSnapshot, cur AlertSnapshot, hadPrev bool) (bool, string) {
	if !hadPrev || prev.Alert == "" {
		return true, "primera señal importante detectada"
	}
	if prev.Alert != cur.Alert {
		return true, "cambio de señal"
	}
	if isMoreUrgent(prev.Alert, cur.Alert) {
		return true, "subió la gravedad de la señal"
	}
	if cur.ExitScore >= prev.ExitScore+2 {
		return true, "subió el exit score"
	}
	if math.Abs(cur.PnLPct-prev.PnLPct) >= 3 {
		return true, "cambió el rendimiento más de 3%"
	}
	if math.Abs(cur.SuggestedSellPct-prev.SuggestedSellPct) >= 25 {
		return true, "cambió el porcentaje sugerido de venta"
	}
	if cur.FirstReason != "" && prev.FirstReason != cur.FirstReason {
		return true, "cambió el motivo principal"
	}
	return false, "sin cambio relevante"
}

func alertRank(a string) int {
	switch a {
	case "PROTEGER GANANCIA":
		return 1
	case "TOMAR GANANCIA PARCIAL":
		return 2
	case "ROTAR CAPITAL / TIME STOP":
		return 2
	case "TOMAR GANANCIA FUERTE":
		return 3
	case "SALIR / VENDER MUCHO", "SALIR / FIN DE TENDENCIA":
		return 4
	case "PROTEGER CAPITAL":
		return 5
	case "STOP DINÁMICO - ASEGURAR":
		return 6
	case "STOP - TESIS INVALIDADA":
		return 7
	default:
		return 0
	}
}

func isMoreUrgent(prev, cur string) bool { return alertRank(cur) > alertRank(prev) }

func loadAlertState() map[string]AlertSnapshot {
	alertMu.Lock()
	defer alertMu.Unlock()
	return loadAlertStateUnlocked()
}

func loadAlertStateUnlocked() map[string]AlertSnapshot {
	b, err := os.ReadFile(fullPath(alertStateFile))
	if err != nil {
		return map[string]AlertSnapshot{}
	}
	m := map[string]AlertSnapshot{}
	if err := json.Unmarshal(b, &m); err == nil {
		return m
	}
	// Compatibilidad con versión vieja: map[string]time.Time.
	old := map[string]time.Time{}
	if err := json.Unmarshal(b, &old); err == nil {
		converted := map[string]AlertSnapshot{}
		for k, t := range old {
			parts := strings.Split(k, "|")
			ticker := k
			alert := ""
			if len(parts) >= 3 {
				ticker = parts[0]
				alert = parts[2]
			}
			converted[ticker] = AlertSnapshot{Ticker: ticker, Alert: alert, LastNotifiedAt: t, UpdatedAt: t}
		}
		return converted
	}
	return map[string]AlertSnapshot{}
}

func saveAlertStateUnlocked(m map[string]AlertSnapshot) {
	_ = writeJSONAtomic(fullPath(alertStateFile), m)
}

func loadAlertHistory() []AlertHistoryItem {
	alertMu.Lock()
	defer alertMu.Unlock()
	return loadAlertHistoryUnlocked()
}

func loadAlertHistoryUnlocked() []AlertHistoryItem {
	b, err := os.ReadFile(fullPath(alertHistoryFile))
	if err != nil {
		return []AlertHistoryItem{}
	}
	var h []AlertHistoryItem
	if err := json.Unmarshal(b, &h); err != nil {
		return []AlertHistoryItem{}
	}
	if len(h) > 100 {
		h = h[:100]
	}
	return h
}

func saveAlertHistoryUnlocked(h []AlertHistoryItem) {
	if len(h) > 100 {
		h = h[:100]
	}
	_ = writeJSONAtomic(fullPath(alertHistoryFile), h)
}

func firstReason(reasons []string) string {
	if len(reasons) == 0 {
		return "Señal generada por scoring técnico/noticias/riesgo."
	}
	return reasons[0]
}

func loadLastPrices() LastPriceStore {
	b, err := os.ReadFile(fullPath(lastPricesFile))
	if err != nil {
		return LastPriceStore{Prices: map[string]Quote{}}
	}
	var s LastPriceStore
	if err := json.Unmarshal(b, &s); err != nil || s.Prices == nil {
		return LastPriceStore{Prices: map[string]Quote{}}
	}
	return s
}

func updateLastPrices(results []Result) {
	store := loadLastPrices()
	if store.Prices == nil {
		store.Prices = map[string]Quote{}
	}
	for _, r := range results {
		if r.Quote.Price > 0 && r.Quote.Error == "" {
			// El cache persistente sólo necesita el último precio/metadatos para fallback.
			// No guardamos cientos de velas por activo: baja muchísimo el estado cloud
			// sin afectar RSI/MACD/EMA, que se calculan antes de llegar acá.
			q := r.Quote
			q.Candles = nil
			store.Prices[r.Asset.YahooTicker] = q
		}
	}
	_ = writeJSONAtomic(fullPath(lastPricesFile), store)
}

func calcEMA(c []Candle, period int) float64 {
	if len(c) < period {
		return 0
	}
	k := 2.0 / float64(period+1)
	ema := 0.0
	for i := 0; i < period; i++ {
		ema += c[i].Close
	}
	ema /= float64(period)
	for i := period; i < len(c); i++ {
		ema = c[i].Close*k + ema*(1-k)
	}
	return ema
}

func calcRSI(c []Candle, period int) float64 {
	if len(c) <= period {
		return 0
	}
	gain, loss := 0.0, 0.0
	for i := 1; i <= period; i++ {
		diff := c[i].Close - c[i-1].Close
		if diff >= 0 {
			gain += diff
		} else {
			loss -= diff
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	for i := period + 1; i < len(c); i++ {
		diff := c[i].Close - c[i-1].Close
		g, l := 0.0, 0.0
		if diff >= 0 {
			g = diff
		} else {
			l = -diff
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
	}
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

func calcStochRSI(c []Candle, period int) float64 {
	if len(c) < period*3 {
		return 0
	}
	rsis := []float64{}
	for i := period + 1; i <= len(c); i++ {
		rsis = append(rsis, calcRSI(c[:i], period))
	}
	if len(rsis) < period {
		return 0
	}
	window := rsis[len(rsis)-period:]
	lowest, highest, current := window[0], window[0], window[len(window)-1]
	for _, v := range window {
		if v < lowest {
			lowest = v
		}
		if v > highest {
			highest = v
		}
	}
	if highest-lowest == 0 {
		return 50
	}
	return ((current - lowest) / (highest - lowest)) * 100
}

func calcMACD(c []Candle) (float64, float64, float64) {
	if len(c) < 35 {
		return 0, 0, 0
	}
	macdLine := []float64{}
	for i := 26; i <= len(c); i++ {
		sub := c[:i]
		e12 := calcEMA(sub, 12)
		e26 := calcEMA(sub, 26)
		macdLine = append(macdLine, e12-e26)
	}
	if len(macdLine) < 9 {
		return 0, 0, 0
	}
	signal := emaFloat(macdLine, 9)
	macd := macdLine[len(macdLine)-1]
	return macd, signal, macd - signal
}

func emaFloat(values []float64, period int) float64 {
	if len(values) < period {
		return 0
	}
	k := 2.0 / float64(period+1)
	ema := 0.0
	for i := 0; i < period; i++ {
		ema += values[i]
	}
	ema /= float64(period)
	for i := period; i < len(values); i++ {
		ema = values[i]*k + ema*(1-k)
	}
	return ema
}

func calcATR(c []Candle, period int) float64 {
	if len(c) <= period {
		return 0
	}
	trs := []float64{}
	for i := 1; i < len(c); i++ {
		highLow := c[i].High - c[i].Low
		highClose := math.Abs(c[i].High - c[i-1].Close)
		lowClose := math.Abs(c[i].Low - c[i-1].Close)
		trs = append(trs, math.Max(highLow, math.Max(highClose, lowClose)))
	}
	if len(trs) < period {
		return 0
	}
	sum := 0.0
	for i := len(trs) - period; i < len(trs); i++ {
		sum += trs[i]
	}
	return sum / float64(period)
}

func avgVolume(c []Candle, period int) float64 {
	if len(c) < period {
		return 0
	}
	sum := 0.0
	for i := len(c) - period; i < len(c); i++ {
		sum += c[i].Volume
	}
	return sum / float64(period)
}

func lastVolume(c []Candle) float64 {
	if len(c) == 0 {
		return 0
	}
	return c[len(c)-1].Volume
}

func supportResistance(c []Candle, period int) (float64, float64) {
	// Excluye la vela actual. Si se incluye, el máximo de hoy se transforma en la
	// "resistencia" y nunca se puede detectar una ruptura real del rango previo.
	if len(c) < period+1 {
		return 0, 0
	}
	end := len(c) - 1
	start := end - period
	if start < 0 {
		start = 0
	}
	support := 0.0
	resistance := 0.0
	for i := start; i < end; i++ {
		if c[i].Low > 0 && (support == 0 || c[i].Low < support) {
			support = c[i].Low
		}
		if c[i].High > resistance {
			resistance = c[i].High
		}
	}
	return support, resistance
}

func buildSummary(results []Result) string {
	buyStrong, buySmall, sell, wait := 0, 0, 0, 0
	for _, r := range results {
		switch r.Recommendation {
		case "COMPRAR MUCHO":
			buyStrong++
		case "COMPRAR POCO":
			buySmall++
		case "VENDER POCO", "VENDER MUCHO":
			sell++
		default:
			wait++
		}
	}
	return fmt.Sprintf("Comprar mucho: %d | Comprar poco: %d | Esperar: %d | Vender: %d", buyStrong, buySmall, wait, sell)
}

func uniqueStrings(in []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, " ", "")
	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	} else {
		s = strings.ReplaceAll(s, ",", ".")
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func holdingDays(date string) int {
	if date == "" {
		return 0
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	return int(time.Since(t).Hours() / 24)
}

func displayDate(date string) string {
	if date == "" {
		return "-"
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("02/01/2006")
}

func formatLocalDateTime(t time.Time) string {
	if t.IsZero() {
		return time.Now().Local().Format("02/01/2006 15:04:05")
	}
	return t.Local().Format("02/01/2006 15:04:05")
}

func money(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("$ %.2f", v)
}

func pct(v float64) string {
	if v == 0 {
		return "0.00%"
	}
	return fmt.Sprintf("%.2f%%", v)
}

func classScore(score int) string {
	if score >= 9 {
		return "score strongbuy"
	}
	if score >= 5 {
		return "score buy"
	}
	if score <= -9 {
		return "score strongsell"
	}
	if score <= -5 {
		return "score sell"
	}
	return "score neutral"
}

func classRec(rec string) string {
	switch rec {
	case "COMPRAR MUCHO":
		return "badge strongbuy"
	case "COMPRAR POCO":
		return "badge buy"
	case "VENDER POCO":
		return "badge sell"
	case "VENDER MUCHO":
		return "badge strongsell"
	default:
		return "badge neutral"
	}
}

func classPnl(v float64) string {
	if v > 0 {
		return "pnl pos"
	}
	if v < 0 {
		return "pnl neg"
	}
	return "pnl flat"
}

func htmlUnescape(s string) string {
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
	return replacer.Replace(s)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeJSONAtomic(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

const pageHTML = `
<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>{{.AppName}}</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="180">
<style>
:root{--bg:#070b14;--panel:#0d1424;--panel2:#101a31;--text:#e8eefc;--muted:#8ea0c4;--line:#22304d;--green:#22c55e;--red:#ef4444;--yellow:#f59e0b;--blue:#38bdf8;--violet:#a78bfa}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;background:radial-gradient(circle at top left,#18305d 0,#070b14 38%,#05070d 100%);color:var(--text);font-family:Segoe UI,Roboto,Arial,sans-serif;font-size:14px}.header{padding:20px 22px;border-bottom:1px solid var(--line);background:rgba(7,11,20,.94);position:sticky;top:0;z-index:20;backdrop-filter:blur(10px)}.headerTop{display:flex;justify-content:space-between;align-items:center;gap:16px;flex-wrap:wrap}h1{margin:0;font-size:clamp(22px,3vw,30px);letter-spacing:-.03em}.sub{color:var(--muted);margin-top:6px;font-size:12px;line-height:1.45}.statusPill{padding:9px 12px;border-radius:999px;border:1px solid rgba(34,197,94,.4);background:rgba(34,197,94,.12);color:#86efac;font-weight:800;font-size:12px;white-space:nowrap}.wrap{padding:22px;max-width:1800px;margin:auto}.nav{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:16px}.nav a,.nav button,.toolbar button,.formBox button{color:#dbeafe;text-decoration:none;border:1px solid var(--line);background:rgba(13,20,36,.92);padding:10px 13px;border-radius:999px;font-weight:800;cursor:pointer;min-height:40px}.nav a:hover,.nav button:hover,.toolbar button:hover,.formBox button:hover{border-color:var(--blue);transform:translateY(-1px)}.helpBox{background:linear-gradient(135deg,rgba(56,189,248,.12),rgba(167,139,250,.10));border:1px solid rgba(125,211,252,.25);border-radius:20px;padding:16px 18px;margin-bottom:16px;color:#dbeafe;line-height:1.48}.cards{display:grid;grid-template-columns:repeat(4,minmax(180px,1fr));gap:14px;margin-bottom:18px}.card{background:linear-gradient(180deg,var(--panel),#0a1020);border:1px solid var(--line);border-radius:20px;padding:16px;box-shadow:0 14px 40px rgba(0,0,0,.28)}.card .k{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.06em}.card .v{font-size:20px;font-weight:900;margin-top:8px;line-height:1.25}.card .hint{color:var(--muted);font-size:12px;margin-top:8px;line-height:1.35}.sectionTitle{font-size:22px;margin:28px 0 12px}.toolbar,.formBox{display:flex;gap:10px;margin-bottom:14px;flex-wrap:wrap;align-items:center;background:rgba(13,20,36,.72);border:1px solid var(--line);border-radius:18px;padding:12px}input,select,button{background:#081020;color:var(--text);border:1px solid var(--line);border-radius:12px;padding:10px 12px;outline:none;font-family:inherit;min-height:40px}input,select{min-width:170px}select{max-width:100%}button{cursor:pointer;font-weight:800}button:hover,select:hover,input:focus{border-color:#38bdf8}.tableBox{overflow:auto;border:1px solid var(--line);border-radius:20px;background:rgba(13,20,36,.88);box-shadow:0 14px 40px rgba(0,0,0,.32);margin-bottom:20px}table{width:100%;border-collapse:separate;border-spacing:0;min-width:1380px}th{text-align:left;background:#0d1424;color:#b9c6e4;font-size:11px;text-transform:uppercase;letter-spacing:.06em;z-index:5}th,td{padding:13px 12px;border-bottom:1px solid rgba(34,48,77,.7);vertical-align:top}tr:hover{background:#111b32}.ticker{font-weight:900;color:white;font-size:18px}.name{color:var(--muted);font-size:12px;max-width:270px;margin-top:4px;line-height:1.35}.price{font-size:18px;font-weight:900}.muted{color:var(--muted);font-size:12px;line-height:1.35}.badge,.score{display:inline-block;padding:7px 10px;border-radius:999px;font-weight:900;font-size:12px;white-space:nowrap;margin-bottom:4px}.strongbuy,.hold{background:rgba(34,197,94,.18);color:#6ee7b7;border:1px solid rgba(34,197,94,.45)}.buy{background:rgba(56,189,248,.16);color:#7dd3fc;border:1px solid rgba(56,189,248,.4)}.neutral{background:rgba(148,163,184,.14);color:#cbd5e1;border:1px solid rgba(148,163,184,.35)}.protect{background:rgba(167,139,250,.16);color:#c4b5fd;border:1px solid rgba(167,139,250,.45)}.sell{background:rgba(245,158,11,.15);color:#fbbf24;border:1px solid rgba(245,158,11,.4)}.strongsell{background:rgba(239,68,68,.17);color:#fca5a5;border:1px solid rgba(239,68,68,.45)}.pnl{font-weight:900}.pnl.pos{color:#86efac}.pnl.neg{color:#fca5a5}.pnl.flat{color:#cbd5e1}.reasons{margin:0;padding-left:17px;color:#d7def2;font-size:12px;line-height:1.45;max-width:460px}.actionBox{background:rgba(8,16,32,.7);border:1px solid rgba(34,48,77,.85);border-radius:14px;padding:10px;max-width:340px}.actionBox b{display:block;margin-bottom:4px}.news{max-width:330px;font-size:12px;color:#cbd5e1;line-height:1.4}.news a{color:#93c5fd;text-decoration:none}.metricGrid{display:grid;grid-template-columns:repeat(2,minmax(82px,1fr));gap:6px;min-width:190px}.metric{background:rgba(8,16,32,.75);border:1px solid rgba(34,48,77,.8);border-radius:12px;padding:8px}.metric .mk{color:var(--muted);font-size:10px;text-transform:uppercase}.metric .mv{font-size:13px;font-weight:800;margin-top:3px}.miniGrid{display:grid;grid-template-columns:repeat(2,minmax(100px,1fr));gap:6px;margin-top:8px}.mini{background:rgba(8,16,32,.75);border:1px solid rgba(34,48,77,.8);border-radius:12px;padding:8px}.mini .mk{color:var(--muted);font-size:10px;text-transform:uppercase}.mini .mv{font-size:12px;font-weight:900;margin-top:3px}body.simple .proOnly{display:none}.err{margin-top:16px;background:rgba(239,68,68,.1);border:1px solid rgba(239,68,68,.35);padding:14px;border-radius:14px;color:#fecaca}.footer{color:var(--muted);padding:20px 24px 40px;font-size:12px;line-height:1.45}.mobileHint{display:none;color:var(--muted);font-size:12px;margin:-4px 0 10px}
@media(max-width:1100px){.cards{grid-template-columns:1fr 1fr}.mobileHint{display:block}}
@media(max-width:760px){.wrap{padding:14px}.header{padding:14px}.cards{grid-template-columns:1fr}.nav a,.nav button,.toolbar button,.formBox button{width:100%;text-align:center}.formBox input,.formBox select,.toolbar input,.toolbar select{width:100%;min-width:100%}.tableBox{border-radius:16px}table{min-width:1220px}.card .v{font-size:18px}.sectionTitle{font-size:20px}}
</style>
<script>
function filterTable(){const q=document.getElementById('q').value.toLowerCase();const type=document.getElementById('type').value;const rec=document.getElementById('rec').value;document.querySelectorAll('#marketRows tr').forEach(row=>{const txt=row.innerText.toLowerCase();let show=txt.includes(q);if(type&&row.getAttribute('data-type')!==type)show=false;if(rec&&row.getAttribute('data-rec')!==rec)show=false;row.style.display=show?'':'none'});}
function setMode(mode){document.body.className=mode;localStorage.setItem('dashboardMode',mode);const b=document.getElementById('modeBtn');if(b)b.innerText=mode==='simple'?'Modo simple':'Modo pro';}
function toggleMode(){setMode(document.body.className==='simple'?'pro':'simple')}
function showOnly(action){document.getElementById('rec').value=action;filterTable()}
function resetFilters(){document.getElementById('q').value='';document.getElementById('type').value='';document.getElementById('rec').value='';filterTable()}
window.onload=function(){setMode(localStorage.getItem('dashboardMode')||'simple')}
</script>
</head>
<body class="simple">
<div class="header">
  <div class="headerTop">
    <div>
      <h1>{{.AppName}}</h1>
      <div class="sub">Generado: {{.GeneratedAt}} · Servicio: {{if .ServiceMode}}activo{{else}}modo consola/web{{end}} · {{.TotalAssets}} activos hardcodeados: {{.TotalActions}} acciones + {{.TotalCedears}} CEDEARs · Caché: {{.CacheAge}}</div>
    </div>
    <div class="statusPill">● Dashboard online</div>
  </div>
</div>
<div class="wrap">
  <div class="nav">
    <a href="#portfolio">Mi cartera</a>
    <a href="#alerts">Últimas alertas</a>
    <a href="#market">Radar mercado</a>
    <a href="/test-alert">Probar alerta</a>
    <a href="/refresh">Forzar refresh</a>
    <a href="/api/data" target="_blank">API JSON</a>
    <a href="/backtest?ticker=GOOGL" target="_blank">Backtest V2</a>
    <button id="modeBtn" onclick="toggleMode()">Modo simple</button>
  </div>

  <div class="helpBox"><b>Motor V2:</b> swing trading con stop técnico inicial, R-multiple, velocidad del retorno, time stop, 4H + diario, subyacente USA y CCL proxy para CEDEARs. Los stops duros alertan inmediatamente; el resto requiere 2 lecturas consecutivas para bajar ruido.<br><b>Modo cartera:</b> al cargar/actualizar una compra se guarda una foto del plan de trade. Si dejás el stop vacío, se calcula automáticamente con ATR + soporte y límites de riesgo.<br><b>Telegram:</b> {{if .TelegramConfigured}}✅ configurado{{else}}⚠ no configurado{{end}} · {{.TelegramStatus}}<br><b>Cartera:</b> {{.PortfolioFile}} · <b>Alertas:</b> {{.AlertStateFile}} · <b>Historial:</b> {{.AlertHistoryFile}}</div>

  <div class="cards">
    <div class="card"><div class="k">Mi cartera</div><div class="v">{{.PortfolioSummary}}</div><div class="hint">Señales especiales de toma de ganancia.</div></div>
    <div class="card"><div class="k">Invertido</div><div class="v">{{money .TotalInvested}}</div><div class="hint">Capital cargado manualmente.</div></div>
    <div class="card"><div class="k">Valor actual estimado</div><div class="v">{{money .TotalCurrent}}</div><div class="hint">Calculado por cantidad estimada x precio actual.</div></div>
    <div class="card"><div class="k">Resultado</div><div class="v"><span class="{{classPnl .TotalPnL}}">{{money .TotalPnL}} · {{pct .TotalPnLPct}}</span></div><div class="hint">Rendimiento total estimado.</div></div>
  </div>

  <h2 id="portfolio" class="sectionTitle">Mi cartera - cosecha de ganancias</h2>
  <form class="formBox" method="post" action="/portfolio/save">
    <select name="ticker" required>{{range .Results}}<option value="{{.Asset.Ticker}}">{{.Asset.Ticker}} - {{.Asset.Name}}</option>{{end}}</select>
    <select name="mode"><option value="replace">Actualizar posición completa</option><option value="add">Agregar compra y recalcular promedio</option></select>
    <input name="amount_ars" placeholder="ARS de esta carga/compra" required>
    <input name="avg_buy_price" placeholder="Precio de compra ej: 6060" required>
    <input type="date" name="buy_date">
    <select name="strategy"><option value="SWING">SWING · objetivo ruedas/semanas</option><option value="POSITION">POSITION · semanas/meses</option></select>
    <input name="expected_days" type="number" min="5" max="365" placeholder="Horizonte ruedas (45 default)">
    <input name="initial_stop_price" placeholder="Stop manual opcional">
    <input name="notes" placeholder="Notas">
    <button type="submit">Guardar + crear plan</button>
  </form>
  <div class="mobileHint">En pantallas chicas podés deslizar la tabla de costado para ver todas las columnas.</div>
  <div class="tableBox">
    <table>
      <thead><tr><th>Activo</th><th>Desde / plan</th><th>Invertido</th><th>Precio compra</th><th>Precio actual</th><th>Valor actual</th><th>Rendimiento / R</th><th>Señal cartera</th><th>Protección V2</th><th>Razones</th><th>Acción</th></tr></thead>
      <tbody>{{range .Portfolio}}
      <tr>
        <td><div class="ticker">{{.Position.Ticker}}</div><div class="name">{{.Asset.Name}}</div><div class="muted">{{.Asset.Type}} · {{.Asset.Sector}}</div><div class="muted">Cant. estimada: {{printf "%.6f" .Quantity}}</div><div class="muted">Consejo: {{.PortfolioAdvice}}</div></td>
        <td>{{.BuyDateDisplay}}<div class="muted">{{.HoldingDays}} días calendario</div><div class="muted">{{.HoldingSessions}} / {{.ExpectedDays}} ruedas · {{.Strategy}}</div><div class="muted">Stop base: {{.StopSource}}</div></td>
        <td>{{money .Position.AmountARS}}</td>
        <td>{{money .Position.AvgBuyPrice}}</td>
        <td><b>{{money .Result.Quote.Price}}</b></td>
        <td><b>{{money .CurrentValue}}</b></td>
        <td><span class="{{classPnl .PnL}}">{{money .PnL}}<br>{{pct .PnLPct}}</span><div class="muted">{{printf "%.2fR" .RMultiple}}</div><div class="muted">{{.VelocityLabel}} · {{printf "%.2f%%/rueda" .ReturnPerDayPct}}</div></td>
        <td><span class="badge {{.AlertClass}}">{{.Alert}}</span><div class="muted">Exit score {{.ExitScore}}</div>{{if gt .SuggestedSellPct 0.0}}<div class="muted">Sugerido: vender aprox. {{printf "%.0f" .SuggestedSellPct}}%</div>{{end}}</td>
        <td><div class="miniGrid"><div class="mini"><div class="mk">Stop inicial</div><div class="mv">{{money .InitialStopPrice}}</div></div><div class="mini"><div class="mk">Stop efectivo</div><div class="mv">{{money .EffectiveStopPrice}}</div></div><div class="mini"><div class="mk">Riesgo inicial</div><div class="mv">{{pct .InitialRiskPct}}</div></div><div class="mini"><div class="mk">Máx desde compra</div><div class="mv">{{money .MaxPriceSinceBuy}}</div></div><div class="mini"><div class="mk">Caída desde máx</div><div class="mv">{{pct .DrawdownFromHighPct}}</div></div><div class="mini"><div class="mk">Trailing</div><div class="mv">{{money .TrailingStopPrice}} · {{pct .TrailingStopPct}}</div></div></div></td>
        <td><div class="actionBox"><b>Qué haría:</b>{{.ExitAction}}</div><ul class="reasons">{{range .ExitReasons}}<li>{{.}}</li>{{end}}</ul></td>
        <td><form method="post" action="/portfolio/delete" onsubmit="return confirm('¿Eliminar {{.Position.Ticker}} de tu cartera?')"><input type="hidden" name="ticker" value="{{.Position.Ticker}}"><button type="submit">Eliminar</button></form></td>
      </tr>{{else}}<tr><td colspan="11" class="muted">Todavía no cargaste activos. Usá el formulario de arriba.</td></tr>{{end}}</tbody>
    </table>
  </div>

  <h2 id="alerts" class="sectionTitle">Últimas alertas detectadas</h2>
  <div class="tableBox">
    <table>
      <thead><tr><th>Fecha</th><th>Activo</th><th>Antes</th><th>Nueva señal</th><th>Cambio</th><th>Precio</th><th>PnL</th><th>Score</th><th>Acción</th><th>Motivo</th></tr></thead>
      <tbody>{{range .AlertHistory}}
      <tr>
        <td>{{formatLocalTime .Time}}</td>
        <td><div class="ticker">{{.Ticker}}</div></td>
        <td><span class="badge neutral">{{.PreviousAlert}}</span></td>
        <td><span class="badge sell">{{.NewAlert}}</span>{{if gt .SuggestedSellPct 0.0}}<div class="muted">Vender aprox. {{printf "%.0f" .SuggestedSellPct}}%</div>{{end}}</td>
        <td>{{.ChangeReason}}</td>
        <td>{{money .Price}}</td>
        <td>{{pct .PnLPct}}</td>
        <td>{{.ExitScore}}</td>
        <td><div class="actionBox">{{.Action}}</div></td>
        <td>{{.FirstReason}}</td>
      </tr>{{else}}<tr><td colspan="10" class="muted">Todavía no hay alertas históricas registradas.</td></tr>{{end}}</tbody>
    </table>
  </div>

  <h2 id="market" class="sectionTitle">Radar mercado completo</h2>
  <div class="toolbar">
    <input id="q" onkeyup="filterTable()" placeholder="Buscar: GGAL, AAPL, bancos, energía, comprar, vender...">
    <select id="type" onchange="filterTable()"><option value="">Todos</option><option value="ACCION">Solo acciones</option><option value="CEDEAR">Solo CEDEARs</option></select>
    <select id="rec" onchange="filterTable()"><option value="">Todas las señales</option><option value="COMPRAR MUCHO">Comprar mucho</option><option value="COMPRAR POCO">Comprar poco</option><option value="NO HACER NADA">No hacer nada</option><option value="VENDER POCO">Vender poco</option><option value="VENDER MUCHO">Vender mucho</option></select>
    <button onclick="showOnly('COMPRAR MUCHO')">Oportunidades fuertes</button><button onclick="showOnly('COMPRAR POCO')">Compras leves</button><button onclick="showOnly('VENDER POCO')">Ventas</button><button onclick="resetFilters()">Limpiar</button>
  </div>

  <div class="cards">
    <div class="card"><div class="k">Resumen mercado</div><div class="v">{{.Summary}}</div></div>
    <div class="card"><div class="k">Total activos</div><div class="v">{{.TotalAssets}}</div><div class="hint">{{.TotalActions}} acciones + {{.TotalCedears}} CEDEARs</div></div>
    <div class="card"><div class="k">Fuente</div><div class="v">Yahoo + cache</div><div class="hint">Quote rápido + chart profundo selectivo + último precio válido.</div></div>
    <div class="card"><div class="k">Alertas</div><div class="v">Telegram + historial</div><div class="hint">Push al iPhone cuando cambia una señal relevante; cooldown: 6 horas.</div></div>
  </div>

  <div class="tableBox">
    <table>
      <thead><tr><th>Activo</th><th>Precio</th><th>Recomendación</th><th>Score</th><th>Razones</th><th class="proOnly">Indicadores</th><th>Riesgo</th><th>Noticias</th></tr></thead>
      <tbody id="marketRows">{{range .Results}}
      <tr data-type="{{.Asset.Type}}" data-rec="{{.Recommendation}}">
        <td><div class="ticker">{{.Asset.Ticker}}</div><div class="name">{{.Asset.Name}}</div><div class="muted">{{.Asset.Type}} · {{.Asset.Sector}}</div><div class="muted">{{.AnalysisMode}}</div>{{if .Quote.Error}}<div class="muted">⚠ {{.Quote.Error}}</div>{{end}}</td>
        <td><div class="price">{{money .Quote.Price}}</div><div class="muted">Var: {{pct .ChangePct}}</div><div class="muted">Confianza: {{.Confidence}}</div><div class="muted">{{.Quote.Source}}</div></td>
        <td><span class="{{classRec .Recommendation}}">{{.Recommendation}}</span></td>
        <td><div class="{{classScore .ScoreFinal}}">Final {{.ScoreFinal}}</div><div class="muted">Técnico {{.ScoreTechnical}} · Contexto {{.ScoreContext}} · Noticias {{.ScoreNews}} · Riesgo {{.ScoreRisk}}</div><div class="muted proOnly">Tend {{.ScoreTrend}} · Mom {{.ScoreMomentum}} · Vol {{.ScoreVolume}} · Zona {{.ScoreLocation}} · 4H {{.ScoreTiming}}</div></td>
        <td><ul class="reasons">{{range .Reasons}}<li>{{.}}</li>{{end}}</ul></td>
        <td class="proOnly"><div class="metricGrid"><div class="metric"><div class="mk">RSI diario</div><div class="mv">{{printf "%.2f" .RSI}}</div></div><div class="metric"><div class="mk">RSI 4H</div><div class="mv">{{printf "%.2f" .FastRSI}}</div></div><div class="metric"><div class="mk">MACD hist</div><div class="mv">{{printf "%.2f" .MACDHist}}</div></div><div class="metric"><div class="mk">4H hist</div><div class="mv">{{printf "%.2f" .FastMACDHist}}</div></div><div class="metric"><div class="mk">Underlying score</div><div class="mv">{{.UnderlyingScore}}</div></div><div class="metric"><div class="mk">CCL proxy 20d</div><div class="mv">{{pct .CCLChange20Pct}}</div></div></div></td>
        <td><span class="badge neutral">{{.Risk}}</span></td>
        <td class="news">{{range .News}}<div>• <a href="{{.Link}}" target="_blank">{{.Title}}</a></div>{{else}}<div class="muted">Sin noticias.</div>{{end}}</td>
      </tr>{{end}}</tbody>
    </table>
  </div>
  {{if .Errors}}<div class="err"><b>Advertencias:</b><ul>{{range .Errors}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
</div>
<div class="footer">Informativo. No ejecuta operaciones. Antes de comprar/vender, validá precio real, puntas, spread y liquidez en Balanz. Las señales ayudan a decidir, no garantizan ganancias.</div>
</body>
</html>
`
