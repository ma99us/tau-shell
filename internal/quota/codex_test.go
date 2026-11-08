package quota

import (
	"errors"
	"testing"
	"time"
)

func TestParseCodexRateLimits(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.FixedZone("local", -5*60*60))
	data := []byte(`{"result":{"planType":"plus","rateLimitsByLimitId":{"codex":{"name":"Codex","primary":{"usedPercent":35,"windowDurationMins":300,"resetsAt":1794168000},"secondary":{"usedPercent":70,"windowDurationMins":10080}}}}}`)
	snapshot, err := ParseCodexRateLimits(data, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan != "plus" || len(snapshot.Windows) != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if got := snapshot.Windows[0]; got.WindowMinutes != 300 || got.Label != "Codex 5h" || got.UsedPercent != 35 || got.ResetAt == "" {
		t.Fatalf("five-hour window = %+v", got)
	}
	if got := snapshot.Windows[1]; got.WindowMinutes != 10080 || got.Label != "Codex 1w" || got.UsedPercent != 70 || got.ResetAt != "" {
		t.Fatalf("weekly window = %+v", got)
	}
}

func TestParseCodexRateLimitsRejectsUnsupportedWindows(t *testing.T) {
	for _, data := range []string{
		`{"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":"NaN","windowDurationMins":300}}}}`,
		`{"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":20,"windowDurationMins":0}}}}`,
		`{"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":20,"windowDurationMins":300.5}}}}`,
	} {
		if _, err := ParseCodexRateLimits([]byte(data), time.Now()); !errors.Is(err, ErrCodexUnavailable) {
			t.Errorf("ParseCodexRateLimits(%s) error = %v, want unavailable", data, err)
		}
	}
}
