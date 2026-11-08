package quota

import (
	"context"
	"errors"
	"testing"
	"time"
)

const litellmTeam = `{"team_info":{"team_alias":"Vega","max_budget":5000,"spend":700,"budget_reset_at":"2026-12-01T00:00:00Z","budget_duration":"1mo"},
"team_memberships":[
 {"user_id":"other","spend":50,"litellm_budget_table":{"max_budget":3000}},
 {"user_id":"me","spend":660,"litellm_budget_table":{"max_budget":3000,"budget_duration":"1mo","budget_reset_at":"2026-12-01T00:00:00Z"}}]}`

func TestParseLiteLLMTeamUsesOwnMembership(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	got, err := ParseLiteLLMTeam([]byte(litellmTeam), "me", "proxy.example", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spend != 660 || got.Budget != 3000 || got.BudgetSource != "member" || got.TeamBudget != 5000 || got.TeamSpend != 700 {
		t.Fatalf("snapshot = %+v", got)
	}
	if !got.CanProject || got.Projected < 2400 || got.Projected > 2600 || got.DaysLeft != 22 {
		t.Fatalf("projection = %+v", got)
	}
	if _, err := ParseLiteLLMTeam([]byte(litellmTeam), "nobody", "h", now); !errors.Is(err, ErrLiteLLMUnavailable) {
		t.Fatalf("missing membership err = %v", err)
	}
}

func TestParseLiteLLMTeamFallsBackToTeamBudget(t *testing.T) {
	data := `{"team_info":{"max_budget":5000,"spend":1,"budget_reset_at":"2026-12-01T00:00:00Z","budget_duration":"1mo"},"team_memberships":[{"user_id":"me","spend":10,"litellm_budget_table":{"max_budget":null}}]}`
	got, err := ParseLiteLLMTeam([]byte(data), "me", "h", time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	if err != nil || got.Budget != 5000 || got.BudgetSource != "team" {
		t.Fatalf("snapshot = %+v, err = %v", got, err)
	}
}

func TestFetchLiteLLMRequiresTeamKey(t *testing.T) {
	get := func(_ context.Context, path string) ([]byte, error) {
		if path == "/key/info" {
			return []byte(`{"info":{"user_id":"me","team_id":""}}`), nil
		}
		return nil, errors.New("unexpected " + path)
	}
	if _, err := FetchLiteLLM(context.Background(), get, "h", time.Now()); !errors.Is(err, ErrLiteLLMUnavailable) {
		t.Fatalf("err = %v", err)
	}
	get = func(_ context.Context, path string) ([]byte, error) {
		if path == "/key/info" {
			return []byte(`{"info":{"user_id":"me","team_id":"t 1"}}`), nil
		}
		if path != "/team/info?team_id=t+1" {
			return nil, errors.New("unexpected " + path)
		}
		return []byte(litellmTeam), nil
	}
	if got, err := FetchLiteLLM(context.Background(), get, "h", time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)); err != nil || got.Spend != 660 {
		t.Fatalf("snapshot = %+v, err = %v", got, err)
	}
}

func TestResolveLiteLLMConfig(t *testing.T) {
	env := map[string]string{}
	getenv := func(name string) string { return env[name] }
	files := map[string]string{}
	read := func(path string) ([]byte, error) {
		if data, ok := files[path]; ok {
			return []byte(data), nil
		}
		return nil, errors.New("missing")
	}
	if _, err := ResolveLiteLLMConfig(getenv, read, `C:\home`); !errors.Is(err, ErrLiteLLMUnavailable) {
		t.Fatalf("empty err = %v", err)
	}
	files[`C:\home\.claude\settings.json`] = `{"env":{"ANTHROPIC_FOUNDRY_BASE_URL":"https://proxy.example/","ANTHROPIC_FOUNDRY_API_KEY":"secret"}}`
	cfg, err := ResolveLiteLLMConfig(getenv, read, `C:\home`)
	if err != nil || cfg.BaseURL != "https://proxy.example" || cfg.Key != "secret" || cfg.Host() != "proxy.example" {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
	files[`C:\home\.claude\settings.json`] = `{"env":{"ANTHROPIC_BASE_URL":"https://api.anthropic.com","ANTHROPIC_API_KEY":"k"}}`
	if _, err := ResolveLiteLLMConfig(getenv, read, `C:\home`); !errors.Is(err, ErrLiteLLMUnavailable) {
		t.Fatalf("anthropic err = %v", err)
	}
	files[`C:\home\.claude\settings.json`] = `{"env":{"ANTHROPIC_BASE_URL":"http://proxy.example","ANTHROPIC_API_KEY":"k"}}`
	if _, err := ResolveLiteLLMConfig(getenv, read, `C:\home`); !errors.Is(err, ErrLiteLLMUnavailable) {
		t.Fatalf("plain http to a remote host must be refused, err = %v", err)
	}
	files[`C:\home\.claude\settings.json`] = `{"env":{"ANTHROPIC_BASE_URL":"http://localhost:4000","ANTHROPIC_API_KEY":"k"}}`
	if cfg, err := ResolveLiteLLMConfig(getenv, read, `C:\home`); err != nil || cfg.BaseURL != "http://localhost:4000" {
		t.Fatalf("loopback cfg = %+v, err = %v", cfg, err)
	}
}
