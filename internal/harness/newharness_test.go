package harness

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

func TestNewHarnessArgumentShapes(t *testing.T) {
	gsdPath := GSDSessionPath("abc")
	for _, test := range []struct {
		harness   session.Harness
		discovers bool
		start     []string
		resume    []string
		launch    []string
	}{
		{session.GSD, false, []string{"--model", "m", "--session", gsdPath}, []string{"--model", "m", "--session", gsdPath}, []string{"--model", "m"}},
		{session.Codex, true, []string{"--model", "m"}, []string{"resume", "abc", "--model", "m"}, []string{"--model", "m"}},
		{session.OpenCode, true, []string{"--model", "m"}, []string{"--model", "m", "--session", "abc"}, []string{"--model", "m"}},
	} {
		adapter, err := For(test.harness)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.DiscoversSessionID() != test.discovers {
			t.Errorf("%s DiscoversSessionID = %v", test.harness, adapter.DiscoversSessionID())
		}
		if got := adapter.Argv(test.launch, false, "abc"); !reflect.DeepEqual(got, test.start) {
			t.Errorf("%s start argv = %#v, want %#v", test.harness, got, test.start)
		}
		if got := adapter.Argv(test.launch, true, "abc"); !reflect.DeepEqual(got, test.resume) {
			t.Errorf("%s resume argv = %#v, want %#v", test.harness, got, test.resume)
		}
	}
}

func TestReservedArgsForNewHarnesses(t *testing.T) {
	for _, test := range []struct {
		harness session.Harness
		args    []string
	}{
		{session.GSD, []string{"--session", "x"}},
		{session.Codex, []string{"resume"}},
		{session.OpenCode, []string{"--session=x"}},
		{session.OpenCode, []string{"-c"}},
	} {
		if err := session.ValidateLaunchArgs(test.harness, test.args); err == nil {
			t.Errorf("%s accepted reserved args %v", test.harness, test.args)
		}
	}
}

func writeRollout(t *testing.T, root, id, cwd string, source string, lines int) {
	t.Helper()
	dir := filepath.Join(root, "2026", "10", "27")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"` + filepath.ToSlash(cwd) + `","source":` + source + `}}` + "\n"
	for range lines {
		body += `{"type":"event_msg","payload":{}}` + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-10-27T10-00-00-"+id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexDiscoverySkipsTakenAndSubagentsAndReportsAmbiguity(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	since := time.Now().Add(-time.Minute)
	writeRollout(t, root, "own", project, `"cli"`, 1)
	writeRollout(t, root, "sub", project, `{"subagent":"review"}`, 1)
	id, err := discoverCodexSessionIn(root, project, since, nil)
	if err != nil || id != "own" {
		t.Fatalf("discover = %q, %v", id, err)
	}
	writeRollout(t, root, "other", project, `"cli"`, 1)
	if _, err := discoverCodexSessionIn(root, project, since, nil); !errors.Is(err, ErrAmbiguousSession) {
		t.Fatalf("two candidates error = %v, want ambiguity", err)
	}
	id, err = discoverCodexSessionIn(root, project, since, map[string]bool{"other": true})
	if err != nil || id != "own" {
		t.Fatalf("with taken: %q, %v", id, err)
	}
	if err := checkCodexResumeIn(root, "missing"); err == nil {
		t.Fatal("resume of unknown Codex ID must fail")
	}
	if err := checkCodexResumeIn(root, "own"); err != nil {
		t.Fatalf("resume of recorded conversation: %v", err)
	}
}

func TestOpenCodeDatabaseDiscoveryAndMessages(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	dbPath := openCodeDatabasePath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	now := time.Now().UnixMilli()
	for _, statement := range []string{
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, parent_id TEXT, time_created INTEGER)`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, time_created INTEGER, time_updated INTEGER, data TEXT)`,
	} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writer.Exec(`INSERT INTO session VALUES ('ses_1', ?, NULL, ?)`, filepath.ToSlash(project), now); err != nil {
		t.Fatal(err)
	}
	data := `{"role":"assistant","modelID":"m1","cost":0.5,"tokens":{"input":10,"output":4,"reasoning":1,"cache":{"read":3,"write":2}},"time":{"completed":5}}`
	if _, err := writer.Exec(`INSERT INTO message VALUES ('msg_1', 'ses_1', ?, ?, ?)`, now, now+7, data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	id, err := discoverOpenCodeSession(project, time.Now().Add(-time.Minute), nil)
	if err != nil || id != "ses_1" {
		t.Fatalf("discover = %q, %v", id, err)
	}
	if id, _ := discoverOpenCodeSession(project, time.Now().Add(-time.Minute), map[string]bool{"ses_1": true}); id != "" {
		t.Fatalf("taken ID was returned: %q", id)
	}
	messages, version, err := OpenCodeMessages("ses_1")
	if err != nil || len(messages) != 1 || version != now+7 {
		t.Fatalf("messages = %+v version %d err %v", messages, version, err)
	}
	if m := messages[0]; m.Model != "m1" || m.Input != 10 || m.Reasoning != 1 || m.CacheRead != 3 || m.CacheWrite != 2 || m.USD != 0.5 {
		t.Fatalf("message = %+v", m)
	}
	if err := checkOpenCodeResume("ses_1"); err != nil {
		t.Fatalf("resume check: %v", err)
	}
	if err := checkOpenCodeResume("nope"); err == nil {
		t.Fatal("resume of an unknown OpenCode session must fail")
	}
}
