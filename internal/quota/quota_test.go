package quota

import (
	"context"
	"errors"
	"testing"
	"time"
)

const sample = `{"copilot_plan":"business","quota_reset_date":"2026-11-01","quota_reset_date_utc":"2026-11-01T00:00:00.000Z",
"quota_snapshots":{"premium_interactions":{"entitlement":4500,"quota_remaining":683.3,"percent_remaining":15.1,"credits_used":3817,"overage_permitted":true,"unlimited":false}}}`

func TestParseProjectsMonthEndUsage(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	snapshot, err := Parse([]byte(sample), now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Entitlement != 4500 || snapshot.Used != 3817 || snapshot.ResetDate != "2026-11-01" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if !snapshot.CanProject || snapshot.CycleDays != 31 || snapshot.DaysLeft != 16 {
		t.Fatalf("unexpected cycle: %+v", snapshot)
	}
	// 3817 used over 15 of 31 days.
	if snapshot.Projected != 7888 {
		t.Fatalf("projected = %v, want 7888", snapshot.Projected)
	}
}

func TestParseDoesNotProjectTooEarly(t *testing.T) {
	snapshot, err := Parse([]byte(sample), time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC))
	if err != nil || snapshot.CanProject {
		t.Fatalf("snapshot = %+v err = %v, want no projection on day one", snapshot, err)
	}
}

func TestParseUnlimitedAndMissing(t *testing.T) {
	unlimited := `{"quota_snapshots":{"premium_interactions":{"unlimited":true}}}`
	snapshot, err := Parse([]byte(unlimited), time.Now())
	if err != nil || !snapshot.Unlimited || snapshot.CanProject {
		t.Fatalf("unlimited snapshot = %+v err = %v", snapshot, err)
	}
	for _, bad := range []string{`{}`, `not json`, `{"quota_snapshots":{}}`} {
		if _, err := Parse([]byte(bad), time.Now()); !errors.Is(err, ErrUnavailable) {
			t.Errorf("Parse(%q) err = %v, want ErrUnavailable", bad, err)
		}
	}
}

func TestFetchWrapsRunnerFailure(t *testing.T) {
	_, err := Fetch(context.Background(), func(context.Context) ([]byte, error) { return nil, errors.New("no gh") }, time.Now())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
