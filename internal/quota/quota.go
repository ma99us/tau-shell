// Package quota reads locally available harness quota snapshots. Every source
// is optional: unavailable or malformed responses are hidden rather than
// guessed at.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// Runner returns the raw JSON of the quota endpoint.
type Runner func(ctx context.Context) ([]byte, error)

// ErrUnavailable means the quota could not be read (no gh, not signed in,
// no Copilot plan, or an unexpected response).
var ErrUnavailable = errors.New("copilot quota unavailable")

// ErrCodexUnavailable means Codex did not expose a usable read-only rate-limit
// snapshot through its local app-server protocol.
var ErrCodexUnavailable = errors.New("codex quota unavailable")

type Snapshot struct {
	Account     string  `json:"account"`
	Plan        string  `json:"plan"`
	Unlimited   bool    `json:"unlimited"`
	Entitlement float64 `json:"entitlement"`
	Used        float64 `json:"used"`
	Remaining   float64 `json:"remaining"`
	PercentUsed float64 `json:"percentUsed"`
	ResetDate   string  `json:"resetDate"`
	CycleDays   int     `json:"cycleDays"`
	DaysLeft    int     `json:"daysLeft"`
	Projected   float64 `json:"projected"`
	OverLimit   bool    `json:"overLimit"`
	OveragesOK  bool    `json:"overagesPermitted"`
	CanProject  bool    `json:"canProject"`
}

type response struct {
	Login          string `json:"login"`
	AccessTypeSKU  string `json:"access_type_sku"`
	Plan           string `json:"copilot_plan"`
	ResetDateUTC   string `json:"quota_reset_date_utc"`
	ResetDate      string `json:"quota_reset_date"`
	QuotaSnapshots struct {
		Chat    *quotaSnapshot `json:"chat"`
		Premium *quotaSnapshot `json:"premium_interactions"`
	} `json:"quota_snapshots"`
}

type quotaSnapshot struct {
	Unlimited       bool    `json:"unlimited"`
	Entitlement     float64 `json:"entitlement"`
	QuotaRemaining  float64 `json:"quota_remaining"`
	Remaining       float64 `json:"remaining"`
	PercentRemain   float64 `json:"percent_remaining"`
	OveragePermited bool    `json:"overage_permitted"`
	CreditsUsed     float64 `json:"credits_used"`
}

// Fetch reads and parses the quota; now is injected for projection tests.
func Fetch(ctx context.Context, run Runner, now time.Time) (Snapshot, error) {
	data, err := run(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return Parse(data, now)
}

func Parse(data []byte, now time.Time) (Snapshot, error) {
	var parsed response
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Snapshot{}, fmt.Errorf("%w: unexpected response", ErrUnavailable)
	}
	quota := parsed.QuotaSnapshots.Premium
	if parsed.AccessTypeSKU == "free_limited_copilot" {
		quota = parsed.QuotaSnapshots.Chat
	}
	if quota == nil {
		return Snapshot{}, fmt.Errorf("%w: no applicable quota in response", ErrUnavailable)
	}
	remaining := quota.QuotaRemaining
	if remaining == 0 && quota.Remaining > 0 {
		remaining = quota.Remaining
	}
	snapshot := Snapshot{
		Account:     parsed.Login,
		Plan:        parsed.Plan,
		Unlimited:   quota.Unlimited,
		Entitlement: quota.Entitlement,
		Remaining:   remaining,
		OveragesOK:  quota.OveragePermited,
		ResetDate:   parsed.ResetDate,
	}
	if snapshot.Unlimited || quota.Entitlement <= 0 {
		return snapshot, nil
	}
	snapshot.Used = quota.CreditsUsed
	if snapshot.Used == 0 {
		snapshot.Used = quota.Entitlement - remaining
	}
	snapshot.PercentUsed = 100 - quota.PercentRemain
	snapshot.OverLimit = snapshot.Used > quota.Entitlement

	reset, err := time.Parse(time.RFC3339, parsed.ResetDateUTC)
	if err != nil {
		return snapshot, nil
	}
	snapshot.ResetDate = reset.Format("2006-01-02")
	start := reset.AddDate(0, -1, 0)
	total := reset.Sub(start).Hours() / 24
	elapsed := now.UTC().Sub(start).Hours() / 24
	snapshot.CycleDays = int(math.Round(total))
	snapshot.DaysLeft = max(0, int(math.Ceil(reset.Sub(now.UTC()).Hours()/24)))
	// Too little history makes a projection meaningless.
	if elapsed >= 1 && total > 0 && snapshot.Used > 0 {
		snapshot.CanProject = true
		snapshot.Projected = math.Round(snapshot.Used / elapsed * total)
	}
	return snapshot, nil
}
