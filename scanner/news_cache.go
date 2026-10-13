package main

import (
	"strings"
	"sync"
	"time"
)

const newsCacheTTL = 30 * time.Minute

type cachedNews struct {
	At    time.Time
	Items []NewsItem
}

var (
	newsCacheMu sync.Mutex
	newsCache   = map[string]cachedNews{}
)

// Las noticias no necesitan descargarse cada vez que se refresca el radar.
// Esto baja bastante el número de requests y evita castigar Google News.
func fetchNews(a Asset) []NewsItem {
	key := strings.ToUpper(strings.TrimSpace(a.Ticker))
	newsCacheMu.Lock()
	if c, ok := newsCache[key]; ok && time.Since(c.At) < newsCacheTTL {
		items := append([]NewsItem(nil), c.Items...)
		newsCacheMu.Unlock()
		return items
	}
	newsCacheMu.Unlock()

	items := fetchNewsUncached(a)
	newsCacheMu.Lock()
	newsCache[key] = cachedNews{At: time.Now(), Items: append([]NewsItem(nil), items...)}
	newsCacheMu.Unlock()
	return items
}
