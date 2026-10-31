// Package quota reads the signed-in user's Copilot premium-request quota
// through the GitHub CLI. The endpoint (/copilot_internal/user) is undocumented
// and may change, so every failure is reported as "unavailable" rather than
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

type Snapshot struct {
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
	Plan           string `json:"copilot_plan"`
	ResetDateUTC   string `json:"quota_reset_date_utc"`
	ResetDate      string `json:"quota_reset_date"`
	QuotaSnapshots struct {
		Premium *struct {
			Unlimited       bool    `json:"unlimited"`
			Entitlement     float64 `json:"entitlement"`
			QuotaRemaining  float64 `json:"quota_remaining"`
			PercentRemain   float64 `json:"percent_remaining"`
			OveragePermited bool    `json:"overage_permitted"`
			CreditsUsed     float64 `json:"credits_used"`
		} `json:"premium_interactions"`
	} `json:"quota_snapshots"`
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
	premium := parsed.QuotaSnapshots.Premium
	if premium == nil {
		return Snapshot{}, fmt.Errorf("%w: no premium quota in response", ErrUnavailable)
	}
	snapshot := Snapshot{
		Plan:        parsed.Plan,
		Unlimited:   premium.Unlimited,
		Entitlement: premium.Entitlement,
		Remaining:   premium.QuotaRemaining,
		OveragesOK:  premium.OveragePermited,
		ResetDate:   parsed.ResetDate,
	}
	if snapshot.Unlimited || premium.Entitlement <= 0 {
		return snapshot, nil
	}
	snapshot.Used = premium.CreditsUsed
	if snapshot.Used == 0 {
		snapshot.Used = premium.Entitlement - premium.QuotaRemaining
	}
	snapshot.PercentUsed = 100 - premium.PercentRemain
	snapshot.OverLimit = snapshot.Used > premium.Entitlement

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
