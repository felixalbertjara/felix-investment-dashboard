//go:build !windows

package main

import (
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

func dashboardAlreadyRunning() bool {
	conn, err := net.DialTimeout("tcp", port, 350*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// En cloud no se abre navegador local. Tailscale Serve publica el backend privado.
func openDashboardBrowser() error { return nil }

// buildPortfolioMonitorViews mantiene la misma lógica eficiente de V2:
// cada 5 minutos analiza profundamente solamente lo que realmente está en cartera.
func buildPortfolioMonitorViews(store PortfolioStore) []PortfolioView {
	if len(store.Positions) == 0 {
		return nil
	}

	allAssets := loadAllAssets()
	byTicker := make(map[string]Asset, len(allAssets))
	for _, a := range allAssets {
		byTicker[strings.ToUpper(a.Ticker)] = a
	}

	selected := make([]Asset, 0, len(store.Positions))
	seen := map[string]bool{}
	for _, p := range store.Positions {
		t := strings.ToUpper(strings.TrimSpace(p.Ticker))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		if a, ok := byTicker[t]; ok {
			selected = append(selected, a)
		}
	}
	if len(selected) == 0 {
		return nil
	}

	quotes := fetchYahooQuotesBatch(selected)
	results := make([]Result, len(selected))
	var wg sync.WaitGroup
	sem := make(chan struct{}, chartWorkers)
	for i := range selected {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a := selected[i]
			results[i] = analyzeDeepForMonitor(a, quotes[a.YahooTicker])
		}(i)
	}
	wg.Wait()

	updateLastPrices(results)
	sort.Slice(results, func(i, j int) bool { return results[i].Asset.Ticker < results[j].Asset.Ticker })
	return buildPortfolioViews(store, results)
}
