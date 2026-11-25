package models

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/mei"
)

func snapshotEntries(t *testing.T) []mei.Entry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "mei", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Entries []mei.Entry `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil || len(file.Entries) == 0 {
		t.Fatalf("snapshot: %d entries, %v", len(file.Entries), err)
	}
	return file.Entries
}

func TestRankMatchesHarnessIDsToMEI(t *testing.T) {
	entries := snapshotEntries(t)
	cases := []struct {
		candidate Candidate
		want      string // MEI model name; empty means unranked
	}{
		{Candidate{ID: "claude-opus-5.5"}, "Opus 5.5"},
		{Candidate{ID: "claude-opus-5-5"}, "Opus 5.5"},
		{Candidate{ID: "claude-sonnet-5.5"}, "Sonnet 5.5"},
		{Candidate{ID: "claude-haiku-5-5-20251001"}, "Haiku 5.5"},
		{Candidate{ID: "gpt-6.2-sol"}, "6.2 Sol"},
		{Candidate{ID: "gpt-6.1-terra"}, "6.1 Terra"},
		{Candidate{ID: "GPT-6-Luna", MatchKey: "gpt-6-luna"}, "6 Luna"},
		{Candidate{ID: "Claude Opus 5.5", MatchKey: "github-copilot/claude-opus-5-5"}, "Opus 5.5"},
		{Candidate{ID: "DeepSeek V4.1 Flash", MatchKey: "opencode/deepseek-v4.1-flash-free"}, "V4.1 Flash"},
		{Candidate{ID: "opus"}, "Opus 5.5"}, // family alias: newest anthropic entry of that family
		{Candidate{ID: "opus", MatchKey: "claude-opus-5-5"}, "Opus 5.5"},
		{Candidate{ID: "sonnet"}, "Sonnet 5.5"},
		{Candidate{ID: "glm-5.3-flash-free"}, ""},
		{Candidate{ID: "gpt-5.3-codex"}, ""},
		{Candidate{ID: "gemini-3.8-flash"}, ""},
		{Candidate{ID: "default"}, ""},
		{Candidate{ID: "auto"}, ""},
	}
	for _, test := range cases {
		options := Rank([]Candidate{test.candidate}, entries)
		got := ""
		if options[0].Ranked {
			got = options[0].MEIName
		}
		if got != test.want {
			t.Errorf("%+v ranked as %q, want %q", test.candidate, got, test.want)
		}
	}
}

func TestRankOrdersRankedFirstAndKeepsTheRest(t *testing.T) {
	entries := snapshotEntries(t)
	options := Rank([]Candidate{{ID: "auto"}, {ID: "claude-haiku-5.5"}, {ID: "claude-opus-5.5"}, {ID: "AUTO"}, {ID: "zzz"}, {ID: " "}}, entries)
	var ids []string
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	if strings.Join(ids, ",") != "claude-opus-5.5,claude-haiku-5.5,auto,zzz" {
		t.Fatalf("order = %v", ids)
	}
	if !options[0].Ranked || options[0].Score <= options[1].Score || options[0].Confidence == "" {
		t.Fatalf("options = %+v", options[:2])
	}
}

func TestRankWithoutMEIDataKeepsOrderUnranked(t *testing.T) {
	options := Rank([]Candidate{{ID: "b"}, {ID: "a", Label: "A label"}}, nil)
	if len(options) != 2 || options[0].ID != "b" || options[1].Label != "A label" || options[0].Ranked || options[1].Ranked {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseCodexModels(t *testing.T) {
	output := []byte("warning: something\n" + `{"models":[{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","visibility":"list"},{"slug":"hidden","display_name":"H","visibility":"hide"},{"slug":"","visibility":"list"}]}` + "\ntrailing")
	result, err := parseCodexModels(output)
	if err != nil || !result.Authoritative || len(result.Candidates) != 1 || result.Candidates[0].ID != "gpt-6.1-sol" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	for _, bad := range []string{"", "not json", `{"models":[]}`} {
		if _, err := parseCodexModels([]byte(bad)); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestParseOpenCodeModels(t *testing.T) {
	output := "opencode/beam-free\n{\n  \"id\": \"beam-free\",\n  \"providerID\": \"opencode\",\n  \"name\": \"Beam Free\",\n  \"status\": \"active\"\n}\n" +
		"anthropic/claude-opus-5-5\n{\n  \"id\": \"claude-opus-5-5\",\n  \"providerID\": \"anthropic\",\n  \"name\": \"Claude Opus 5.5\",\n  \"status\": \"active\"\n}\n" +
		"x/old\n{\n  \"id\": \"old\",\n  \"providerID\": \"x\",\n  \"name\": \"Old\",\n  \"status\": \"deprecated\"\n}\n" +
		"x/broken\n{ not json\n"
	result, err := parseOpenCodeModels([]byte(output))
	if err != nil || len(result.Candidates) != 2 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	second := result.Candidates[1]
	if second.ID != "Claude Opus 5.5" || second.MatchKey != "claude-opus-5-5" || second.Label != "Claude Opus 5.5 (anthropic)" {
		t.Fatalf("second = %+v", second)
	}
	if _, err := parseOpenCodeModels([]byte("garbage")); err == nil {
		t.Fatal("expected an error for output without models")
	}
}

func TestParseClaudeSettings(t *testing.T) {
	result, err := parseClaudeSettings([]byte(`{"model":"claude-opus-5-5","env":{"ANTHROPIC_DEFAULT_OPUS_MODEL":"claude-opus-5-5","ANTHROPIC_DEFAULT_HAIKU_MODEL":"claude-haiku-5-5","OTHER":"x"}}`))
	if err != nil || result.Authoritative || len(result.Candidates) != 3 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if result.Candidates[0].ID != "haiku" || result.Candidates[0].MatchKey != "claude-haiku-5-5" || result.Candidates[2].ID != "claude-opus-5-5" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if result, _ := parseClaudeSettings([]byte(`{"model":"opusplan"}`)); len(result.Candidates) != 0 {
		t.Fatalf("opusplan should not be a candidate: %+v", result)
	}
	if _, err := parseClaudeSettings([]byte("{broken")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseCopilotConfig(t *testing.T) {
	result, err := parseCopilotConfig([]byte("// User settings\n// Do not edit\n" + `{"recentModelIds":["gpt-6.2-sol"," ","claude-sonnet-5.5"],"other":1}`))
	if err != nil || len(result.Candidates) != 2 || result.Candidates[0].ID != "gpt-6.2-sol" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestProbeMissingFilesAreNotErrors(t *testing.T) {
	home := t.TempDir()
	for _, id := range []string{harnessClaude, harnessCopilot} {
		result, err := Probe(context.Background(), id, "", Env{Home: home})
		if err != nil || len(result.Candidates) != 0 {
			t.Errorf("%s: %+v, %v", id, result, err)
		}
	}
	if _, err := Probe(context.Background(), "gsd", "", Env{}); err == nil {
		t.Error("gsd has no probe")
	}
}

func TestCombine(t *testing.T) {
	manifest := []string{"a", "b"}
	if got := Combine(manifest, Result{Authoritative: true, Candidates: []Candidate{{ID: "x"}}}); len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("authoritative = %+v", got)
	}
	if got := Combine(manifest, Result{Authoritative: true}); len(got) != 2 {
		t.Fatalf("empty authoritative must fall back to the manifest: %+v", got)
	}
	if got := Combine(manifest, Result{Candidates: []Candidate{{ID: "x"}}}); len(got) != 3 || got[0].ID != "a" || got[2].ID != "x" {
		t.Fatalf("additive = %+v", got)
	}
}

func TestServiceCachesRefreshesAndKeepsGoodResultOnFailure(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	updates := make(chan string, 8)
	run := func(ctx context.Context, path string, args ...string) ([]byte, error) {
		calls.Add(1)
		if fail.Load() {
			return nil, errors.New("exit status 1")
		}
		return []byte(`{"models":[{"slug":"gpt-6.1-sol","display_name":"GPT","visibility":"list"}]}`), nil
	}
	service := NewService(Env{Run: run, Executable: func(context.Context, string) string { return "codex" }}, func(id string) { updates <- id })
	clock := time.Now()
	service.now = func() time.Time { return clock }
	wait := func() {
		t.Helper()
		select {
		case <-updates:
		case <-time.After(5 * time.Second):
			t.Fatal("probe did not finish")
		}
	}
	first := service.Get(context.Background(), harnessCodex, false)
	if !first.Probing || len(first.Result.Candidates) != 0 {
		t.Fatalf("first = %+v", first)
	}
	wait()
	second := service.Get(context.Background(), harnessCodex, false)
	if second.Probing || len(second.Result.Candidates) != 1 || calls.Load() != 1 {
		t.Fatalf("second = %+v calls=%d", second, calls.Load())
	}
	clock = clock.Add(25 * time.Hour)
	fail.Store(true)
	service.Get(context.Background(), harnessCodex, false)
	wait()
	third := service.Get(context.Background(), harnessCodex, false)
	if third.Error == "" || len(third.Result.Candidates) != 1 || calls.Load() != 2 {
		t.Fatalf("a failed probe must keep the last good result: %+v calls=%d", third, calls.Load())
	}
	if got := service.Get(context.Background(), "gsd", false); got.Probing || got.Error != "" {
		t.Fatalf("harness without a probe: %+v", got)
	}
}
