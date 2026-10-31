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
