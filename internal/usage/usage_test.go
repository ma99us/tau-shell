package usage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

const pricingFixture = `
| Model | Base input tokens | 5m cache writes | 1h cache writes | Cache hits and refreshes | Output tokens |
| :--- | :--- | :--- | :--- | :--- | :--- |
| Claude Fable 5.1 | $10 / MTok | $12.50 / MTok | $20 / MTok | $0.25 / MTok<sup>1</sup> | $50 / MTok |
| Claude Mythos 5.1 ([limited availability](https://example.test/a)) | $10 / MTok | $12.50 / MTok | $20 / MTok | $0.25 / MTok<sup>1</sup> | $50 / MTok |
| Claude Opus 5.5 | $4 / MTok | $5 / MTok | $8 / MTok | $0.20 / MTok<sup>2</sup> | $20 / MTok |
| Claude Sonnet 5 | $2 / MTok<sup>3</sup> | $2.50 / MTok | $4 / MTok | $0.20 / MTok | $10 / MTok<sup>3</sup> |
| Claude Haiku 5.5 (for prompts up to 100,000 tokens) | $0.10 / MTok | $0.125 / MTok | $0.20 / MTok | $0.01 / MTok | $0.50 / MTok |
| Claude Haiku 5.5 (for prompts over 100,000 tokens) | $0.50 / MTok | $0.625 / MTok | $1 / MTok | $0.05 / MTok | $2.50 / MTok |
| Claude Broken 9 | $x | $1 | $1 | $1 | $1 |
`

func TestNormalizeModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5":            "opus-5.5",
		"claude-opus-5":              "opus-5",
		"claude-sonnet-4-5-20250929": "sonnet-4.5",
		"claude-sonnet-4-20250514":   "sonnet-4",
		"claude-opus-5-5[1m]":        "opus-5.5",
		"claude-haiku-5-5#large":     "haiku-5.5>100k",
	}
	for model, want := range cases {
		if got, ok := NormalizeModel(model); !ok || got != want {
			t.Errorf("NormalizeModel(%q) = %q, %v; want %q", model, got, ok, want)
		}
	}
	if _, ok := NormalizeModel("gpt-5"); ok {
		t.Error("non-Claude model was recognized")
	}
}

func TestParseAnthropicPricing(t *testing.T) {
	table, err := ParseAnthropicPricing(pricingFixture)
	if err != nil {
		t.Fatal(err)
	}
	if got := table["opus-5.5"]; got != (Price{Input: 4, CacheWrite5m: 5, CacheWrite1h: 8, CacheRead: 0.2, Output: 20}) {
		t.Errorf("opus-5.5 = %+v", got)
	}
	if got := table["haiku-5.5>100k"].Input; got != 0.5 {
		t.Errorf("large haiku input = %v", got)
	}
	if got := table["haiku-5.5"].Input; got != 0.1 {
		t.Errorf("base haiku input = %v", got)
	}
	if _, ok := table["mythos-5.1"]; !ok {
		t.Error("link text after the model name broke parsing")
	}
	if got := table["sonnet-5"].Output; got != 10 {
		t.Errorf("superscript broke sonnet-5 output = %v", got)
	}
	if _, err := ParseAnthropicPricing("| Claude Opus 5 | $1 |"); err == nil {
		t.Error("malformed table was accepted")
	}
}

func TestParseLiteLLMPrices(t *testing.T) {
	data := `{
		"claude-opus-5-5": {"litellm_provider":"anthropic","input_cost_per_token":4e-6,"output_cost_per_token":2e-5,"cache_creation_input_token_cost":5e-6,"cache_creation_input_token_cost_above_1hr":8e-6,"cache_read_input_token_cost":2e-7},
		"claude-sonnet-5": {"litellm_provider":"anthropic","input_cost_per_token":2e-6,"output_cost_per_token":1e-5},
		"claude-haiku-4-5": {"litellm_provider":"anthropic","input_cost_per_token":1e-6,"output_cost_per_token":5e-6},
		"bedrock/claude-opus-4": {"litellm_provider":"bedrock","input_cost_per_token":9e-6,"output_cost_per_token":9e-5},
		"gpt-5": {"litellm_provider":"openai","input_cost_per_token":1e-6,"output_cost_per_token":8e-6}
	}`
	table, err := ParseLiteLLMPrices(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := table["opus-5.5"]; got.Input != 4 || got.Output != 20 || got.CacheRead != 0.2 || got.CacheWrite1h != 8 {
		t.Errorf("opus-5.5 = %+v", got)
	}
	if got := table["sonnet-5"]; got.CacheWrite5m != 2.5 || got.CacheWrite1h != 4 {
		t.Errorf("derived sonnet-5 cache prices = %+v", got)
	}
	if _, ok := table["opus-4"]; ok {
		t.Error("non-Anthropic entry was accepted")
	}
}

func TestFetchFallsBackToLiteLLMAndKeepsCache(t *testing.T) {
	litellm := `{"claude-opus-5-5":{"litellm_provider":"anthropic","input_cost_per_token":4e-6,"output_cost_per_token":2e-5},
		"claude-sonnet-5":{"litellm_provider":"anthropic","input_cost_per_token":2e-6,"output_cost_per_token":1e-5},
		"claude-haiku-4-5":{"litellm_provider":"anthropic","input_cost_per_token":1e-6,"output_cost_per_token":5e-6}}`
	broken := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pricing.md":
			if broken {
				fmt.Fprint(w, "redesigned page")
				return
			}
			fmt.Fprint(w, pricingFixture)
		case "/litellm.json":
			fmt.Fprint(w, litellm)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := Fetcher{AnthropicURL: server.URL + "/pricing.md", LiteLLMURL: server.URL + "/litellm.json", allowHTTP: true}
	cachePath := filepath.Join(t.TempDir(), "prices.json")
	store := NewPriceStore(cachePath, fetcher)
	if !store.NeedsRefresh(time.Now()) {
		t.Fatal("empty cache should need a refresh")
	}
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, info := store.Table(nil, time.Now()); info.Source != SourceLiteLLM {
		t.Fatalf("source = %q, want litellm fallback", info.Source)
	}
	if store.NeedsRefresh(time.Now()) {
		t.Error("fresh cache should not need a refresh")
	}
	if !store.NeedsRefresh(time.Now().Add(25 * time.Hour)) {
		t.Error("day-old cache should need a refresh")
	}

	broken = false
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	table, info := store.Table(PriceTable{"opus-5.5": {Input: 1, CacheWrite5m: 1, CacheWrite1h: 1, CacheRead: 1, Output: 1}}, time.Now())
	if info.Source != SourceAnthropic || !info.Overrides {
		t.Errorf("info = %+v", info)
	}
	if table["opus-5.5"].Input != 1 {
		t.Error("config override must win over fetched prices")
	}
	if table["opus-4.5"].Input != 5 {
		t.Error("built-in prices must remain for models the fetch lacks")
	}

	server.Close()
	if err := store.Refresh(context.Background()); err == nil {
		t.Error("refresh against a dead server should fail")
	}
	if _, info := store.Table(nil, time.Now()); info.Source != SourceAnthropic {
		t.Error("a failed refresh must keep the last good cache")
	}
	if reopened := NewPriceStore(cachePath, fetcher); reopened.NeedsRefresh(time.Now()) {
		t.Error("cache did not persist")
	}
}

func TestFetchRefusesPlainHTTP(t *testing.T) {
	if _, err := (Fetcher{AnthropicURL: "http://example.test/p", LiteLLMURL: "http://example.test/l"}).Fetch(context.Background()); err == nil {
		t.Error("plain HTTP price sources must be refused")
	}
}

func TestCorruptPriceCacheFallsBackToBuiltin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(path, []byte(`{"models":{"opus-5.5":{"input":-1}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	table, info := NewPriceStore(path, Fetcher{}).Table(nil, time.Now())
	if !strings.HasPrefix(info.Source, "built-in") || table["opus-5.5"].Input != 4 {
		t.Errorf("info = %+v", info)
	}
}

func TestMonthStart(t *testing.T) {
	zone := time.UTC
	cases := []struct {
		now      time.Time
		day      int
		wantDate string
	}{
		{time.Date(2026, 10, 28, 8, 0, 0, 0, zone), 1, "2026-10-01"},
		{time.Date(2026, 10, 14, 8, 0, 0, 0, zone), 15, "2026-09-15"},
		{time.Date(2026, 10, 15, 0, 0, 0, 0, zone), 15, "2026-10-15"},
		{time.Date(2026, 1, 3, 8, 0, 0, 0, zone), 10, "2025-12-10"},
	}
	for _, c := range cases {
		if got := MonthStart(c.now, c.day).Format(dayLayout); got != c.wantDate {
			t.Errorf("MonthStart(%v, %d) = %s, want %s", c.now, c.day, got, c.wantDate)
		}
	}
}

func claudeLine(id, model, timestamp string, input, output, read, write5m, write1h int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":%q,"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":%d,"ephemeral_1h_input_tokens":%d}}}}`+"\n",
		timestamp, id, model, input, output, read, write5m+write1h, write5m, write1h)
}

func TestParseClaudeLine(t *testing.T) {
	record, ok := ParseClaudeLine([]byte(claudeLine("m1", "claude-opus-5-5", "2026-10-20T12:00:00Z", 10, 20, 30, 40, 50)))
	if !ok || record.Input != 10 || record.Output != 20 || record.CacheRead != 30 || record.CacheWrite5m != 40 || record.CacheWrite1h != 50 {
		t.Fatalf("record = %+v, ok = %v", record, ok)
	}
	if _, ok := ParseClaudeLine([]byte(claudeLine("m2", "<synthetic>", "2026-10-20T12:00:00Z", 5, 5, 0, 0, 0))); ok {
		t.Error("synthetic message was counted")
	}
	if _, ok := ParseClaudeLine([]byte(claudeLine("m3", "claude-opus-5-5", "2026-10-20T12:00:00Z", 0, 0, 0, 0, 0))); ok {
		t.Error("zero usage was counted")
	}
	large, _ := ParseClaudeLine([]byte(claudeLine("m4", "claude-haiku-5-5", "2026-10-20T12:00:00Z", 1, 1, 150000, 0, 0)))
	if !strings.HasSuffix(large.Model, largePromptSuffix) {
		t.Errorf("large Haiku prompt model = %q", large.Model)
	}
}

func TestParseCopilotLine(t *testing.T) {
	line := `{"type":"session.usage_record","data":{"usage":{"model":"claude-opus-5.5","inputTokens":37900,"outputTokens":304,"cacheReadTokens":0,"cacheWriteTokens":37896,"isByok":false,"copilotUsage":{"tokenDetails":[{"tokenCount":4,"tokenType":"input"},{"tokenCount":0,"tokenType":"cache_read"},{"tokenCount":37896,"tokenType":"cache_write"},{"tokenCount":304,"tokenType":"output"}],"totalNanoAiu":19557600000}}},"timestamp":"2026-10-23T17:10:15.489Z"}`
	record, ok := ParseCopilotLine([]byte(line))
	if !ok || record.Input != 4 || record.Output != 304 || record.CacheWrite5m != 37896 || !record.Priced {
		t.Fatalf("record = %+v, ok = %v", record, ok)
	}
	if want := 0.195576; record.USD < want-1e-9 || record.USD > want+1e-9 {
		t.Errorf("USD = %v, want %v", record.USD, want)
	}
	byok := strings.Replace(line, `"isByok":false`, `"isByok":true`, 1)
	if record, ok := ParseCopilotLine([]byte(byok)); !ok || record.Priced {
		t.Errorf("BYOK usage must be counted but unpriced: %+v", record)
	}
}

func openTestLedger(t *testing.T, path string) *Ledger {
	t.Helper()
	ledger, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	return ledger
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerResumesWithoutDoubleCounting(t *testing.T) {
	directory := t.TempDir()
	dbPath := filepath.Join(directory, "state.db")
	log := filepath.Join(directory, "claude.jsonl")
	prices := BuiltinPrices()

	// A streamed reply: the second chunk of m1 replaces the first.
	appendFile(t, log, claudeLine("m1", "claude-opus-5-5", "2026-10-20T12:00:00Z", 100, 10, 0, 1000, 0))
	appendFile(t, log, claudeLine("m1", "claude-opus-5-5", "2026-10-20T12:00:01Z", 100, 50, 0, 1000, 0))
	ledger := openTestLedger(t, dbPath)
	if err := ledger.IngestFile("s1", session.ClaudeCode, "h1", log); err != nil {
		t.Fatal(err)
	}
	if err := ledger.IngestFile("s1", session.ClaudeCode, "h1", log); err != nil {
		t.Fatal(err)
	}
	got, err := ledger.SessionUsage("s1", prices)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total.Input != 100 || got.Total.Output != 50 || got.Total.CacheWrite != 1000 {
		t.Fatalf("after first ingest: %+v", got.Total)
	}
	wantUSD := (100*4 + 50*20 + 1000*5) / 1e6
	if diff := got.Total.USD - wantUSD; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("USD = %v, want %v", got.Total.USD, wantUSD)
	}

	// Restart: reopen the database and continue the same message plus a new one.
	_ = ledger.Close()
	ledger = openTestLedger(t, dbPath)
	appendFile(t, log, claudeLine("m1", "claude-opus-5-5", "2026-10-20T12:00:02Z", 100, 60, 0, 1000, 0))
	appendFile(t, log, claudeLine("m2", "claude-sonnet-5", "2026-10-20T12:05:00Z", 10, 10, 0, 0, 0))
	appendFile(t, log, `{"type":"assistant","partial`)
	if err := ledger.IngestFile("s1", session.ClaudeCode, "h1", log); err != nil {
		t.Fatal(err)
	}
	got, _ = ledger.SessionUsage("s1", prices)
	if got.Total.Input != 110 || got.Total.Output != 70 || got.Total.CacheWrite != 1000 {
		t.Fatalf("after restart: %+v", got.Total)
	}
	if len(got.Models) != 2 || len(got.Conversations) != 1 {
		t.Errorf("models = %+v, conversations = %+v", got.Models, got.Conversations)
	}

	// Start fresh adds a second conversation; the session total keeps growing.
	second := filepath.Join(directory, "claude2.jsonl")
	appendFile(t, second, claudeLine("n1", "claude-opus-5-5", "2026-10-21T09:00:00Z", 1, 1, 0, 0, 0))
	if err := ledger.IngestFile("s1", session.ClaudeCode, "h2", second); err != nil {
		t.Fatal(err)
	}
	got, _ = ledger.SessionUsage("s1", prices)
	if got.Total.Input != 111 || len(got.Conversations) != 2 {
		t.Errorf("after fresh: %+v / %d conversations", got.Total, len(got.Conversations))
	}

	// A deleted harness log keeps its totals.
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if err := ledger.IngestFile("s1", session.ClaudeCode, "h2", second); err != nil {
		t.Fatal(err)
	}
	if got, _ = ledger.SessionUsage("s1", prices); got.Total.Input != 111 {
		t.Errorf("totals lost with the log: %+v", got.Total)
	}

	// A truncated log is rescanned without keeping its old contribution.
	if err := os.WriteFile(log, []byte(claudeLine("m9", "claude-opus-5-5", "2026-10-22T09:00:00Z", 7, 0, 0, 0, 0)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ledger.IngestFile("s1", session.ClaudeCode, "h1", log); err != nil {
		t.Fatal(err)
	}
	if got, _ = ledger.SessionUsage("s1", prices); got.Total.Input != 8 {
		t.Errorf("after truncation: %+v", got.Total)
	}
}

func TestSummaryMonthlyTotalsKeepDeletedSessions(t *testing.T) {
	directory := t.TempDir()
	ledger := openTestLedger(t, filepath.Join(directory, "state.db"))
	prices := BuiltinPrices()

	claudeLog := filepath.Join(directory, "c.jsonl")
	appendFile(t, claudeLog, claudeLine("a", "claude-opus-5-5", "2026-10-05T12:00:00Z", 1_000_000, 0, 0, 0, 0))
	appendFile(t, claudeLog, claudeLine("b", "claude-opus-5-5", "2026-09-20T12:00:00Z", 1_000_000, 0, 0, 0, 0))
	appendFile(t, claudeLog, claudeLine("c", "claude-mystery-1", "2026-10-06T12:00:00Z", 5, 0, 0, 0, 0))
	copilotLog := filepath.Join(directory, "e.jsonl")
	appendFile(t, copilotLog, `{"type":"session.usage_record","data":{"usage":{"model":"x","copilotUsage":{"tokenDetails":[{"tokenCount":10,"tokenType":"output"}],"totalNanoAiu":100000000000}}},"timestamp":"2026-10-07T12:00:00Z"}`+"\n")

	if err := ledger.IngestFile("claude-session", session.ClaudeCode, "h1", claudeLog); err != nil {
		t.Fatal(err)
	}
	if err := ledger.IngestFile("copilot-session", session.CopilotCLI, "h2", copilotLog); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Tombstone("copilot-session", time.Now()); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 28, 8, 0, 0, 0, time.Local)
	summary, err := ledger.Summary(now, 1, prices)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := summary.Sessions["copilot-session"]; present || len(summary.Sessions) != 1 {
		t.Errorf("deleted session listed: %+v", summary.Sessions)
	}
	if got := summary.MonthByTool[string(session.CopilotCLI)].USD; got != 1 {
		t.Errorf("deleted Copilot session dropped from the month: %v", got)
	}
	if got := summary.MonthByTool[string(session.ClaudeCode)]; got.USD != 4 || !got.Unpriced {
		t.Errorf("Claude month = %+v, want $4 with an unpriced remainder", got)
	}
	if summary.Month.USD != 5 || summary.PrevMonth.USD != 4 {
		t.Errorf("month %v, previous %v", summary.Month.USD, summary.PrevMonth.USD)
	}
	if got := summary.Sessions["claude-session"].USD; got != 8 {
		t.Errorf("session total = %v, want the unreset sum 8", got)
	}

	// The reused ID starts at zero.
	if got, _ := ledger.SessionUsage("copilot-session", prices); got.Total.Tokens != 0 {
		t.Errorf("reused ID inherited usage: %+v", got.Total)
	}
}
