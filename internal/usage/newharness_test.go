package usage

import (
	"path/filepath"
	"testing"

	"github.com/ma99us/tau-shell/internal/session"
)

func TestParseGSDLineReadsOwnCost(t *testing.T) {
	line := []byte(`{"type":"message","id":"a","timestamp":"2026-10-27T10:00:00Z","message":{"role":"assistant","model":"claude-x","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.25}}}}`)
	record, ok := ParseGSDLine(line)
	if !ok || record.Model != "claude-x" || record.Input != 10 || record.Output != 5 || record.CacheRead != 3 || record.CacheWrite5m != 2 || record.USD != 0.25 || !record.Priced {
		t.Fatalf("record = %+v ok=%v", record, ok)
	}
	if _, ok := ParseGSDLine([]byte(`{"type":"message","message":{"role":"user","content":"usage assistant"}}`)); ok {
		t.Fatal("user line must not count")
	}
}

func TestCodexParserTracksModelAndDedupesByTotal(t *testing.T) {
	parser := newCodexParser("")
	count := `{"timestamp":"2026-10-27T10:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":150},"last_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":50,"reasoning_output_tokens":10}}}}`
	if _, ok := parser.parse([]byte(count)); ok {
		t.Fatal("record without a known model must be skipped")
	}
	parser.parse([]byte(`{"type":"turn_context","payload":{"model":"gpt-5.5"}}`))
	record, ok := parser.parse([]byte(count))
	if !ok || record.Model != "gpt-5.5" || record.Input != 60 || record.CacheRead != 40 || record.Output != 50 || record.MessageID != "total:150" {
		t.Fatalf("record = %+v ok=%v", record, ok)
	}
	if _, ok := parser.parse([]byte(`{"type":"event_msg","payload":{"type":"token_count","info":null}}`)); ok {
		t.Fatal("null info must be skipped")
	}
	table := BuiltinPrices()
	if _, ok := table.Lookup("gpt-5.5"); !ok {
		t.Fatal("gpt-5.5 must be priced")
	}
	if _, ok := table.Lookup("gpt-unknown"); ok {
		t.Fatal("unknown model must be unpriced")
	}
}

func TestRecordsOwnCostCoversLoggingHarnesses(t *testing.T) {
	for tool, want := range map[session.Harness]bool{
		session.CopilotCLI: true, session.GSD: true, session.OpenCode: true,
		session.ClaudeCode: false, session.Codex: false,
	} {
		if RecordsOwnCost(tool) != want {
			t.Errorf("RecordsOwnCost(%s) = %v", tool, !want)
		}
	}
}

func TestLedgerIngestsCodexRolloutAcrossReads(t *testing.T) {
	ledger, err := OpenLedger(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	first := `{"type":"turn_context","payload":{"model":"gpt-5.5"}}` + "\n" +
		`{"timestamp":"2026-10-27T10:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":10},"last_token_usage":{"input_tokens":6,"cached_input_tokens":0,"output_tokens":4}}}}` + "\n"
	second := `{"timestamp":"2026-10-27T10:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":30},"last_token_usage":{"input_tokens":12,"cached_input_tokens":0,"output_tokens":8}}}}` + "\n"
	appendFile(t, path, first)
	if err := ledger.IngestFile("s1", session.Codex, "cx", path); err != nil {
		t.Fatal(err)
	}
	appendFile(t, path, second)
	if err := ledger.IngestFile("s1", session.Codex, "cx", path); err != nil {
		t.Fatal(err)
	}
	usage, err := ledger.SessionUsage("s1", BuiltinPrices())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Total.Input != 18 || usage.Total.Output != 12 || usage.Total.Unpriced || usage.Total.USD <= 0 {
		t.Fatalf("total = %+v", usage.Total)
	}
}
