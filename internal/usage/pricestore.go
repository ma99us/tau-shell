package usage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PriceRefreshInterval is how often prices are re-read from the web.
const PriceRefreshInterval = 24 * time.Hour

// PriceInfo says where the prices in use came from.
type PriceInfo struct {
	Source    string `json:"source"`
	UpdatedAt string `json:"updatedAt"`
	Stale     bool   `json:"stale"`
	Overrides bool   `json:"overrides"`
}

// PriceStore resolves the effective Claude price table: config overrides win,
// then the fetched cache, then the built-in defaults.
type PriceStore struct {
	path    string
	fetcher Fetcher
	mu      sync.Mutex
	cached  FetchedPrices
}

func NewPriceStore(path string, fetcher Fetcher) *PriceStore {
	return &PriceStore{path: path, fetcher: fetcher, cached: LoadPriceCache(path)}
}

// Table returns the effective price table and a description of its source.
func (store *PriceStore) Table(overrides PriceTable, now time.Time) (PriceTable, PriceInfo) {
	store.mu.Lock()
	cached := store.cached
	store.mu.Unlock()
	table := Merged(BuiltinPrices(), cached.Models, overrides)
	info := PriceInfo{Source: "built-in " + BuiltinPricesDate, Overrides: len(overrides) > 0}
	if len(cached.Models) > 0 {
		info.Source = cached.Source
		info.UpdatedAt = cached.FetchedAt.Local().Format("2006-01-02")
		info.Stale = cached.Stale(now, 14*24*time.Hour)
	}
	return table, info
}

// NeedsRefresh reports whether the cache is missing or older than the refresh
// interval.
func (store *PriceStore) NeedsRefresh(now time.Time) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.cached.Stale(now, PriceRefreshInterval)
}

// Refresh reads prices from the web and replaces the cache. On any failure the
// previous cache is kept.
func (store *PriceStore) Refresh(ctx context.Context) error {
	fetched, err := store.fetcher.Fetch(ctx)
	if err != nil {
		return err
	}
	if err := SavePriceCache(store.path, fetched); err != nil {
		return fmt.Errorf("save price cache: %w", err)
	}
	store.mu.Lock()
	store.cached = fetched
	store.mu.Unlock()
	return nil
}
