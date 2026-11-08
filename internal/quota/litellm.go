package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrLiteLLMUnavailable means Claude Code is not routed through a LiteLLM proxy
// that exposes this key's team-member budget.
var ErrLiteLLMUnavailable = errors.New("litellm quota unavailable")

// LiteLLMConfig is the proxy endpoint and credential Claude Code itself uses.
// Key is only ever sent to BaseURL and is never logged or returned to the UI.
type LiteLLMConfig struct {
	BaseURL string
	Key     string
}

// LiteLLMGetter performs an authenticated GET against the proxy.
type LiteLLMGetter func(ctx context.Context, path string) ([]byte, error)

// LiteLLMSnapshot is the caller's own spend and budget for the current cycle.
// Budget is 0 when the proxy reports no limit. BudgetSource is "member" for a
// team-member budget or "team" when only the team has a limit.
type LiteLLMSnapshot struct {
	Host         string  `json:"host"`
	Spend        float64 `json:"spend"`
	Budget       float64 `json:"budget"`
	BudgetSource string  `json:"budgetSource"`
	ResetAt      string  `json:"resetAt,omitempty"`
	DaysLeft     int     `json:"daysLeft"`
	Projected    float64 `json:"projected"`
	CanProject   bool    `json:"canProject"`
	TeamSpend    float64 `json:"teamSpend"`
	TeamBudget   float64 `json:"teamBudget"`
	TeamName     string  `json:"teamName,omitempty"`
}

// ResolveLiteLLMConfig finds the proxy Claude Code uses. Claude Code settings
// env wins over the process environment, matching Claude Code's own order. The
// public Anthropic API is not a LiteLLM proxy and is skipped.
func ResolveLiteLLMConfig(getenv func(string) string, readFile func(string) ([]byte, error), home string) (LiteLLMConfig, error) {
	env := map[string]string{}
	dir := getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(home, ".claude")
	}
	if data, err := readFile(filepath.Join(dir, "settings.json")); err == nil {
		var settings struct {
			Env map[string]any `json:"env"`
		}
		if json.Unmarshal(data, &settings) == nil {
			for name, value := range settings.Env {
				if text, ok := value.(string); ok {
					env[name] = text
				}
			}
		}
	}
	pick := func(names ...string) string {
		for _, name := range names {
			if value := strings.TrimSpace(env[name]); value != "" {
				return value
			}
		}
		for _, name := range names {
			if value := strings.TrimSpace(getenv(name)); value != "" {
				return value
			}
		}
		return ""
	}
	base := pick("ANTHROPIC_BASE_URL", "ANTHROPIC_FOUNDRY_BASE_URL")
	key := pick("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_FOUNDRY_API_KEY")
	if base == "" || key == "" {
		return LiteLLMConfig{}, fmt.Errorf("%w: no proxy endpoint configured", ErrLiteLLMUnavailable)
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return LiteLLMConfig{}, fmt.Errorf("%w: invalid proxy endpoint", ErrLiteLLMUnavailable)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "api.anthropic.com" {
		return LiteLLMConfig{}, fmt.Errorf("%w: direct Anthropic API", ErrLiteLLMUnavailable)
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(host)) {
		return LiteLLMConfig{}, fmt.Errorf("%w: proxy endpoint must use https", ErrLiteLLMUnavailable)
	}
	return LiteLLMConfig{BaseURL: strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.Path, "/"), Key: key}, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// HTTPGetter sends read-only GETs to the proxy. Redirects are refused so the
// credential cannot be forwarded to another host.
func (config LiteLLMConfig) HTTPGetter() LiteLLMGetter {
	client := &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return func(ctx context.Context, path string) ([]byte, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.BaseURL+path, nil)
		if err != nil {
			return nil, errors.New("invalid request")
		}
		request.Header.Set("Authorization", "Bearer "+config.Key)
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return nil, errors.New("proxy request failed")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("proxy returned HTTP %d for %s", response.StatusCode, path)
		}
		return io.ReadAll(io.LimitReader(response.Body, 4<<20))
	}
}

// FetchLiteLLM reads the caller's key, then its team, and keeps only the
// caller's own membership row (the team response also lists other members).
func FetchLiteLLM(ctx context.Context, get LiteLLMGetter, host string, now time.Time) (LiteLLMSnapshot, error) {
	keyData, err := get(ctx, "/key/info")
	if err != nil {
		return LiteLLMSnapshot{}, fmt.Errorf("%w: %v", ErrLiteLLMUnavailable, err)
	}
	var key struct {
		Info struct {
			UserID string `json:"user_id"`
			TeamID string `json:"team_id"`
		} `json:"info"`
	}
	if json.Unmarshal(keyData, &key) != nil || key.Info.UserID == "" || key.Info.TeamID == "" {
		return LiteLLMSnapshot{}, fmt.Errorf("%w: key is not attached to a team", ErrLiteLLMUnavailable)
	}
	teamData, err := get(ctx, "/team/info?team_id="+url.QueryEscape(key.Info.TeamID))
	if err != nil {
		return LiteLLMSnapshot{}, fmt.Errorf("%w: %v", ErrLiteLLMUnavailable, err)
	}
	return ParseLiteLLMTeam(teamData, key.Info.UserID, host, now)
}

// ParseLiteLLMTeam extracts the member's cycle spend and budget from /team/info.
func ParseLiteLLMTeam(data []byte, userID, host string, now time.Time) (LiteLLMSnapshot, error) {
	var team struct {
		TeamInfo struct {
			Alias        string  `json:"team_alias"`
			MaxBudget    float64 `json:"max_budget"`
			Spend        float64 `json:"spend"`
			BudgetReset  string  `json:"budget_reset_at"`
			BudgetPeriod string  `json:"budget_duration"`
		} `json:"team_info"`
		Memberships []struct {
			UserID string  `json:"user_id"`
			Spend  float64 `json:"spend"`
			Budget *struct {
				MaxBudget    *float64 `json:"max_budget"`
				BudgetPeriod string   `json:"budget_duration"`
				BudgetReset  string   `json:"budget_reset_at"`
			} `json:"litellm_budget_table"`
		} `json:"team_memberships"`
	}
	if json.Unmarshal(data, &team) != nil {
		return LiteLLMSnapshot{}, fmt.Errorf("%w: unexpected response", ErrLiteLLMUnavailable)
	}
	snapshot := LiteLLMSnapshot{
		Host:         host,
		BudgetSource: "none",
		TeamSpend:    team.TeamInfo.Spend,
		TeamBudget:   team.TeamInfo.MaxBudget,
		TeamName:     team.TeamInfo.Alias,
	}
	resetText, period := team.TeamInfo.BudgetReset, team.TeamInfo.BudgetPeriod
	found := false
	for _, member := range team.Memberships {
		if member.UserID != userID {
			continue
		}
		found = true
		snapshot.Spend = member.Spend
		if member.Budget != nil {
			if member.Budget.MaxBudget != nil && *member.Budget.MaxBudget > 0 {
				snapshot.Budget = *member.Budget.MaxBudget
				snapshot.BudgetSource = "member"
			}
			if member.Budget.BudgetReset != "" {
				resetText = member.Budget.BudgetReset
			}
			if member.Budget.BudgetPeriod != "" {
				period = member.Budget.BudgetPeriod
			}
		}
	}
	if !found {
		return LiteLLMSnapshot{}, fmt.Errorf("%w: no membership for this key's user", ErrLiteLLMUnavailable)
	}
	if snapshot.Budget == 0 && team.TeamInfo.MaxBudget > 0 {
		snapshot.Budget = team.TeamInfo.MaxBudget
		snapshot.BudgetSource = "team"
	}
	if reset, err := time.Parse(time.RFC3339Nano, resetText); err == nil {
		snapshot.ResetAt = reset.UTC().Format(time.RFC3339)
		snapshot.DaysLeft = int(reset.Sub(now).Hours()/24 + 0.999)
		if snapshot.DaysLeft < 0 {
			snapshot.DaysLeft = 0
		}
		if start, ok := cycleStart(reset, period); ok {
			elapsed := now.Sub(start).Hours() / 24
			length := reset.Sub(start).Hours() / 24
			if elapsed >= 1 && length > 0 {
				snapshot.Projected = snapshot.Spend / elapsed * length
				snapshot.CanProject = true
			}
		}
	}
	return snapshot, nil
}

// cycleStart derives the start of the current budget cycle from the next reset
// time and a LiteLLM duration such as "30d", "1mo" or "24h".
func cycleStart(reset time.Time, period string) (time.Time, bool) {
	period = strings.TrimSpace(period)
	switch {
	case strings.HasSuffix(period, "mo"):
		if n, err := strconv.Atoi(strings.TrimSuffix(period, "mo")); err == nil && n > 0 {
			return reset.AddDate(0, -n, 0), true
		}
	case strings.HasSuffix(period, "d"):
		if n, err := strconv.Atoi(strings.TrimSuffix(period, "d")); err == nil && n > 0 {
			return reset.AddDate(0, 0, -n), true
		}
	case strings.HasSuffix(period, "h"):
		if n, err := strconv.Atoi(strings.TrimSuffix(period, "h")); err == nil && n > 0 {
			return reset.Add(-time.Duration(n) * time.Hour), true
		}
	}
	return time.Time{}, false
}

// Host returns the proxy host for display.
func (config LiteLLMConfig) Host() string {
	if parsed, err := url.Parse(config.BaseURL); err == nil {
		return parsed.Host
	}
	return ""
}
