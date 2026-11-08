//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const quotaTestBody = `{"copilot_plan":"individual","quota_reset_date_utc":"2999-01-01T00:00:00.000Z","quota_snapshots":{"premium_interactions":{"entitlement":100,"credits_used":10,"quota_remaining":90,"percent_remaining":90}}}`

func TestGetCopilotQuotaCachesAndHonorsOptOut(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "sessions")
	calls := 0
	application := &App{configDirectory: configDir, quotaRunner: func(context.Context) ([]byte, error) {
		calls++
		return []byte(quotaTestBody), nil
	}}

	if got := application.GetCopilotQuota(false); got == nil || got.Entitlement != 100 {
		t.Fatalf("quota = %+v", got)
	}
	application.GetCopilotQuota(false)
	if calls != 1 {
		t.Fatalf("calls = %d, want cached single call", calls)
	}
	application.GetCopilotQuota(true)
	if calls != 2 {
		t.Fatalf("calls = %d, want refresh to refetch", calls)
	}

	if err := os.WriteFile(application.settingsPath(), []byte("version: 1\nquota:\n  copilot: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := application.GetCopilotQuota(true); got != nil || calls != 2 {
		t.Fatalf("disabled quota = %+v, calls = %d", got, calls)
	}
}

func TestGetClaudeQuotaCachesAndHonorsOptOut(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "sessions")
	calls := 0
	application := &App{configDirectory: configDir, claudeQuotaGetter: func(_ context.Context, path string) ([]byte, error) {
		calls++
		if path == "/key/info" {
			return []byte(`{"info":{"user_id":"me","team_id":"t"}}`), nil
		}
		return []byte(`{"team_info":{"max_budget":5000,"spend":1},"team_memberships":[{"user_id":"me","spend":12,"litellm_budget_table":{"max_budget":3000}}]}`), nil
	}}

	if got := application.GetClaudeQuota(false); got == nil || got.Spend != 12 || got.Budget != 3000 {
		t.Fatalf("quota = %+v", got)
	}
	application.GetClaudeQuota(false)
	if calls != 2 {
		t.Fatalf("calls = %d, want one cached fetch (2 requests)", calls)
	}

	if err := os.WriteFile(application.settingsPath(), []byte("version: 1\nquota:\n  claude: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := application.GetClaudeQuota(true); got != nil || calls != 2 {
		t.Fatalf("disabled quota = %+v, calls = %d", got, calls)
	}
}

func TestGetCodexQuotaCachesAndHonorsOptOut(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "sessions")
	calls := 0
	application := &App{configDirectory: configDir, codexQuotaRunner: func(context.Context) ([]byte, error) {
		calls++
		return []byte(`{"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":25,"windowDurationMins":300}}}}`), nil
	}}

	if got := application.GetCodexQuota(false); got == nil || len(got.Windows) != 1 || got.Windows[0].UsedPercent != 25 {
		t.Fatalf("quota = %+v", got)
	}
	application.GetCodexQuota(false)
	if calls != 1 {
		t.Fatalf("calls = %d, want cached single call", calls)
	}
	application.GetCodexQuota(true)
	if calls != 2 {
		t.Fatalf("calls = %d, want refresh to refetch", calls)
	}

	if err := os.WriteFile(application.settingsPath(), []byte("version: 1\nquota:\n  codex: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := application.GetCodexQuota(true); got != nil || calls != 2 {
		t.Fatalf("disabled quota = %+v, calls = %d", got, calls)
	}
}
