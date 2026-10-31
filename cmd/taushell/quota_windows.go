//go:build windows

package main

import (
	"context"
	"time"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/quota"
)

const quotaCacheTTL = 5 * time.Minute

// GetCopilotQuota returns the Copilot premium-request quota, or nil when the
// chip is disabled or the quota cannot be read (no gh, signed out, no plan).
// Results are cached so Home polling does not spawn gh repeatedly.
func (application *App) GetCopilotQuota(refresh bool) *quota.Snapshot {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil || !settings.Quota.CopilotEnabled() {
		return nil
	}
	application.quotaMu.Lock()
	defer application.quotaMu.Unlock()
	if !refresh && !application.quotaAt.IsZero() && time.Since(application.quotaAt) < quotaCacheTTL {
		return application.quotaValue
	}
	run := application.quotaRunner
	if run == nil {
		run = quota.GHRunner
	}
	snapshot, err := quota.Fetch(context.Background(), run, time.Now())
	application.quotaAt = time.Now()
	if err != nil {
		if !application.quotaLogged {
			diagnostics.Warn("copilot quota unavailable", err)
			application.quotaLogged = true
		}
		application.quotaValue = nil
		return nil
	}
	application.quotaLogged = false
	application.quotaValue = &snapshot
	return application.quotaValue
}
