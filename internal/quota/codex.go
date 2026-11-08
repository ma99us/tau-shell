package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CodexRunner returns the result of Codex's read-only account/rateLimits/read
// RPC. TauShell never passes credentials to it or calls account-mutating RPCs.
type CodexRunner func(context.Context) ([]byte, error)

// CodexWindow is one rate-limit window supplied by Codex. UsedPercent comes
// directly from Codex; it is deliberately not turned into a cost projection.
type CodexWindow struct {
	LimitID       string  `json:"limitId"`
	Label         string  `json:"label"`
	UsedPercent   float64 `json:"usedPercent"`
	WindowMinutes int     `json:"windowMinutes"`
	ResetAt       string  `json:"resetAt,omitempty"`
}

// CodexSnapshot contains the signed-in plan label and every reported window.
// The protocol can report multiple limit IDs, so no primary/secondary order is
// assumed.
type CodexSnapshot struct {
	Plan    string        `json:"plan"`
	Windows []CodexWindow `json:"windows"`
}

// FetchCodex reads and parses the local app-server snapshot.
func FetchCodex(ctx context.Context, run CodexRunner, now time.Time) (CodexSnapshot, error) {
	data, err := run(ctx)
	if err != nil {
		return CodexSnapshot{}, fmt.Errorf("%w: %v", ErrCodexUnavailable, err)
	}
	return ParseCodexRateLimits(data, now)
}

// ParseCodexRateLimits accepts the account/rateLimits/read result in either a
// JSON-RPC envelope or its bare result form. Codex has changed field casing in
// experimental protocol revisions, so this parser accepts camelCase and
// snake_case while requiring real percentage and duration values.
func ParseCodexRateLimits(data []byte, now time.Time) (CodexSnapshot, error) {
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return CodexSnapshot{}, fmt.Errorf("%w: unexpected response", ErrCodexUnavailable)
	}
	if result, ok := object(root["result"]); ok {
		root = result
	}
	plan := stringValue(root, "planType", "plan_type", "plan")
	limits, ok := object(value(root, "rateLimitsByLimitId", "rate_limits_by_limit_id"))
	if !ok {
		return CodexSnapshot{}, fmt.Errorf("%w: missing limit map", ErrCodexUnavailable)
	}
	snapshot := CodexSnapshot{Plan: plan, Windows: []CodexWindow{}}
	for limitID, raw := range limits {
		limit, ok := object(raw)
		if !ok {
			continue
		}
		name := stringValue(limit, "name", "limitName", "limit_name")
		if name == "" {
			name = limitID
		}
		for _, candidate := range []any{value(limit, "primary"), value(limit, "secondary"), value(limit, "window"), value(limit, "windows")} {
			snapshot.Windows = append(snapshot.Windows, codexWindows(limitID, name, candidate, now)...)
		}
	}
	if len(snapshot.Windows) == 0 {
		return CodexSnapshot{}, fmt.Errorf("%w: no usable windows", ErrCodexUnavailable)
	}
	sort.Slice(snapshot.Windows, func(i, j int) bool {
		if snapshot.Windows[i].WindowMinutes != snapshot.Windows[j].WindowMinutes {
			return snapshot.Windows[i].WindowMinutes < snapshot.Windows[j].WindowMinutes
		}
		return snapshot.Windows[i].LimitID < snapshot.Windows[j].LimitID
	})
	return snapshot, nil
}

func codexWindows(limitID, name string, raw any, now time.Time) []CodexWindow {
	if list, ok := raw.([]any); ok {
		var out []CodexWindow
		for _, item := range list {
			out = append(out, codexWindows(limitID, name, item, now)...)
		}
		return out
	}
	window, ok := object(raw)
	if !ok {
		return nil
	}
	used, hasUsed := numberValue(window, "usedPercent", "used_percent")
	duration, hasDuration := numberValue(window, "windowDurationMins", "window_duration_mins", "windowMinutes", "window_minutes")
	if !hasUsed || !hasDuration || math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 || math.IsNaN(duration) || math.IsInf(duration, 0) || math.Trunc(duration) != duration || duration <= 0 {
		return nil
	}
	reset := codexReset(value(window, "resetsAt", "resets_at", "resetAt", "reset_at"), now)
	label := durationLabel(int(duration))
	if name != "" && name != limitID {
		label = name + " " + label
	}
	return []CodexWindow{{LimitID: limitID, Label: label, UsedPercent: used, WindowMinutes: int(duration), ResetAt: reset}}
}

func object(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func value(object map[string]any, names ...string) any {
	for _, name := range names {
		if value, ok := object[name]; ok {
			return value
		}
	}
	return nil
}

func stringValue(object map[string]any, names ...string) string {
	value, _ := value(object, names...).(string)
	return value
}

func numberValue(object map[string]any, names ...string) (float64, bool) {
	switch value := value(object, names...).(type) {
	case float64:
		return value, true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func codexReset(value any, now time.Time) string {
	switch value := value.(type) {
	case float64:
		return time.Unix(int64(value), 0).In(now.Location()).Format(time.RFC3339)
	case string:
		if stamp, err := time.Parse(time.RFC3339, value); err == nil {
			return stamp.In(now.Location()).Format(time.RFC3339)
		}
	}
	return ""
}

func durationLabel(minutes int) string {
	switch {
	case minutes%10080 == 0:
		return fmt.Sprintf("%dw", minutes/10080)
	case minutes%1440 == 0:
		return fmt.Sprintf("%dd", minutes/1440)
	case minutes%60 == 0:
		return fmt.Sprintf("%dh", minutes/60)
	default:
		return strings.TrimSpace(fmt.Sprintf("%dm", minutes))
	}
}
