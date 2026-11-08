//go:build windows

package main

import (
	"context"
	"errors"
	"os"
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

// GetCodexQuota returns locally reported Codex rate-limit windows, or nil when
// disabled or unavailable. The cached snapshot avoids starting app-server
// processes for every Home poll.
func (application *App) GetCodexQuota(refresh bool) *quota.CodexSnapshot {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil || !settings.Quota.CodexEnabled() {
		return nil
	}
	application.quotaMu.Lock()
	defer application.quotaMu.Unlock()
	if !refresh && !application.codexQuotaAt.IsZero() && time.Since(application.codexQuotaAt) < quotaCacheTTL {
		return application.codexQuotaValue
	}
	run := application.codexQuotaRunner
	if run == nil {
		run = quota.CodexAppServerRunner
	}
	snapshot, err := quota.FetchCodex(context.Background(), run, time.Now())
	application.codexQuotaAt = time.Now()
	if err != nil {
		if !application.codexQuotaLogged {
			diagnostics.Warn("Codex quota unavailable", err)
			application.codexQuotaLogged = true
		}
		application.codexQuotaValue = nil
		return nil
	}
	application.codexQuotaLogged = false
	application.codexQuotaValue = &snapshot
	return application.codexQuotaValue
}

// GetClaudeQuota returns the caller's LiteLLM team-member spend and budget when
// Claude Code is routed through a LiteLLM proxy, or nil when disabled or not
// available. The credential is read in the host from Claude Code's own
// settings and is never returned to the UI.
func (application *App) GetClaudeQuota(refresh bool) *quota.LiteLLMSnapshot {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil || !settings.Quota.ClaudeEnabled() {
		return nil
	}
	application.quotaMu.Lock()
	defer application.quotaMu.Unlock()
	if !refresh && !application.claudeQuotaAt.IsZero() && time.Since(application.claudeQuotaAt) < quotaCacheTTL {
		return application.claudeQuotaValue
	}
	application.claudeQuotaAt = time.Now()
	get, host := application.claudeQuotaGetter, "proxy"
	var fetchErr error
	if get == nil {
		home, _ := os.UserHomeDir()
		var config quota.LiteLLMConfig
		config, fetchErr = quota.ResolveLiteLLMConfig(os.Getenv, os.ReadFile, home)
		if fetchErr == nil {
			get, host = config.HTTPGetter(), config.Host()
		}
	}
	var snapshot quota.LiteLLMSnapshot
	if fetchErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		snapshot, fetchErr = quota.FetchLiteLLM(ctx, get, host, time.Now())
	}
	if fetchErr != nil {
		if !application.claudeQuotaLogged && !errors.Is(fetchErr, quota.ErrLiteLLMUnavailable) {
			diagnostics.Warn("Claude proxy quota unavailable", fetchErr)
			application.claudeQuotaLogged = true
		}
		application.claudeQuotaValue = nil
		return nil
	}
	application.claudeQuotaLogged = false
	application.claudeQuotaValue = &snapshot
	return application.claudeQuotaValue
}
