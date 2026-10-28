package sessionhost

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/usage"
)

const ingestThrottle = 2 * time.Second

// openUsage attaches the usage ledger and price store. A ledger that cannot be
// opened disables usage tracking but never blocks sessions.
func (manager *Manager) openUsage(databasePath string) {
	ledger, err := usage.OpenLedger(databasePath)
	if err != nil {
		diagnostics.Warn("usage ledger unavailable", err)
		return
	}
	manager.ledger = ledger
	manager.prices = usage.NewPriceStore(filepath.Join(filepath.Dir(databasePath), "prices.json"), usage.Fetcher{})
	manager.lastIngest = make(map[string]time.Time)
}

func (manager *Manager) usageSettings() appconfig.Usage {
	config, err := appconfig.Load(filepath.Join(filepath.Dir(manager.configDirectory), appconfig.FileName))
	if err != nil {
		diagnostics.Warn("settings unreadable; using usage defaults", err)
		return appconfig.Usage{}
	}
	return config.Usage
}

func priceOverrides(settings appconfig.Usage) usage.PriceTable {
	if len(settings.ClaudePrices) == 0 {
		return nil
	}
	table := make(usage.PriceTable, len(settings.ClaudePrices))
	for model, override := range settings.ClaudePrices {
		price := usage.Price{
			Input:        override.Input,
			Output:       override.Output,
			CacheWrite5m: override.CacheWrite5m,
			CacheWrite1h: override.CacheWrite1h,
			CacheRead:    override.CacheRead,
		}
		if price.CacheWrite5m == 0 {
			price.CacheWrite5m = override.Input * 1.25
		}
		if price.CacheWrite1h == 0 {
			price.CacheWrite1h = override.Input * 2
		}
		if price.CacheRead == 0 {
			price.CacheRead = override.Input * 0.1
		}
		table[strings.ToLower(model)] = price
	}
	return table
}

// ingestSession reads new harness-log lines for one session. force bypasses the
// throttle. Failures are logged; totals then reflect what was already stored.
func (manager *Manager) ingestSession(ctx context.Context, id string, force bool) {
	if manager.ledger == nil {
		return
	}
	manager.usageMu.Lock()
	if last, ok := manager.lastIngest[id]; ok && !force && time.Since(last) < ingestThrottle {
		manager.usageMu.Unlock()
		return
	}
	manager.lastIngest[id] = time.Now()
	manager.usageMu.Unlock()

	config, err := session.FindConfig(manager.configDirectory, id)
	if err != nil {
		return
	}
	runtime, err := manager.store.Runtime(ctx, id)
	if err != nil || runtime.HarnessSessionID == "" {
		return
	}
	for _, path := range usage.LogFiles(config.Tool, runtime.HarnessSessionID) {
		if err := manager.ledger.IngestFile(id, config.Tool, runtime.HarnessSessionID, path); err != nil {
			diagnostics.Warn("usage ingest failed", err, "session_id", id)
		}
	}
}

// SessionUsage returns the session's all-time token and cost totals.
func (manager *Manager) SessionUsage(ctx context.Context, id string) (usage.SessionUsage, error) {
	if manager.ledger == nil {
		return usage.SessionUsage{}, nil
	}
	manager.ingestSession(ctx, id, false)
	settings := manager.usageSettings()
	table, info := manager.prices.Table(priceOverrides(settings), time.Now())
	result, err := manager.ledger.SessionUsage(id, table)
	result.Prices = info
	return result, err
}

// UsageSummary returns per-session totals and the monthly totals.
func (manager *Manager) UsageSummary(ctx context.Context) (usage.Summary, error) {
	if manager.ledger == nil {
		return usage.Summary{}, nil
	}
	configs, err := session.ListConfigs(manager.configDirectory)
	if err != nil {
		return usage.Summary{}, err
	}
	for _, config := range configs {
		manager.ingestSession(ctx, config.ID, false)
	}
	settings := manager.usageSettings()
	table, info := manager.prices.Table(priceOverrides(settings), time.Now())
	summary, err := manager.ledger.Summary(time.Now(), settings.StartDay(), table)
	summary.Prices = info
	return summary, err
}

// StartPriceUpdates refreshes Claude prices now if the cache is stale, then
// daily, until ctx ends. It is not started by New so tests never use the network.
func (manager *Manager) StartPriceUpdates(ctx context.Context) {
	if manager.prices == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	manager.usageMu.Lock()
	manager.cancelPrices = cancel
	manager.usageMu.Unlock()
	go func() {
		refresh := func() {
			if !manager.usageSettings().PriceUpdatesEnabled() || !manager.prices.NeedsRefresh(time.Now()) {
				return
			}
			if err := manager.prices.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
				diagnostics.Warn("price refresh failed; keeping previous prices", err)
			}
		}
		refresh()
		ticker := time.NewTicker(usage.PriceRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

func (manager *Manager) closeUsage() error {
	manager.usageMu.Lock()
	cancel := manager.cancelPrices
	manager.usageMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if manager.ledger != nil {
		return manager.ledger.Close()
	}
	return nil
}
