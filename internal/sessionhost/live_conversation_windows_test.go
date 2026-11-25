package sessionhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

// TestLiveConversationSwitch checks that a conversation the harness starts
// itself (/clear) is adopted by TauShell. It spends a few model calls and
// writes to the harness's real session store, so it only runs when
// TAUSHELL_LIVE_CONV names a harness: claude-code or copilot-cli.
func TestLiveConversationSwitch(t *testing.T) {
	tool := session.Harness(os.Getenv("TAUSHELL_LIVE_CONV"))
	if tool == "" {
		t.Skip("set TAUSHELL_LIVE_CONV=claude-code|copilot-cli to run")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "sessions")
	workingDir := filepath.Join(root, "project")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := session.SaveConfig(configDir, session.Config{ID: "live", Name: "live", Tool: tool, WorkingDir: workingDir}); err != nil {
		t.Fatal(err)
	}
	output := &liveOutput{}
	manager, err := New(configDir, filepath.Join(root, "state.db"), func(event Event) { output.add(event, "live") })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())
	ctx := context.Background()
	var seen []string
	t.Cleanup(func() {
		home, _ := os.UserHomeDir()
		for _, id := range seen {
			matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", id+".jsonl"))
			for _, path := range matches {
				_ = os.Remove(path)
			}
			if tool == session.CopilotCLI {
				_ = os.RemoveAll(filepath.Join(home, ".copilot", "session-state", id))
			}
		}
	})
	send := func(text string) {
		if err := manager.Write("live", []byte(text)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
	}
	current := func() string {
		runtime, _ := manager.store.Runtime(ctx, "live")
		return runtime.HarnessSessionID
	}
	if _, err := manager.Start(ctx, "live", false, 120, 40); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			text := output.since(0)
			if len(text) > 3000 {
				text = text[len(text)-3000:]
			}
			t.Logf("screen tail:\n%s", text)
		}
	})
	defer manager.Stop(ctx, "live")
	time.Sleep(6 * time.Second)
	if output.hasAny("trust") {
		_ = manager.Write("live", []byte("\x1b[B"))
		time.Sleep(time.Second)
		_ = manager.Write("live", []byte("\r"))
		time.Sleep(4 * time.Second)
	}
	send("Reply with exactly the single word ONETAU and nothing else.")
	if !waitFor(90*time.Second, func() bool { return current() != "" }) {
		t.Fatalf("no conversation id recorded after the first prompt; indicators=%v", output.indicators())
	}
	first := current()
	seen = append(seen, first)
	t.Logf("first conversation: %s", first)
	time.Sleep(15 * time.Second)

	command := os.Getenv("TAUSHELL_LIVE_CONV_CMD")
	if command == "" {
		command = "/clear"
	}
	send(command)
	time.Sleep(4 * time.Second)
	send("Reply with exactly the single word TWOTAU and nothing else.")
	switched := waitFor(120*time.Second, func() bool { return current() != first })
	second := current()
	seen = append(seen, second)
	t.Logf("after the reset command: %s (switched=%v)", second, switched)
	if !switched && tool == session.CopilotCLI {
		home, _ := os.UserHomeDir()
		dirs, _ := filepath.Glob(filepath.Join(home, ".copilot", "session-state", "*", "workspace.yaml"))
		for _, path := range dirs {
			data, _ := os.ReadFile(path)
			if strings.Contains(string(data), "project") && strings.Contains(string(data), filepath.Base(root)) {
				dir := filepath.Dir(path)
				events, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
				t.Logf("copilot dir %s events=%d bytes workspace=%s", filepath.Base(dir), len(events), strings.ReplaceAll(string(data), "\n", " | "))
				seen = append(seen, filepath.Base(dir))
			}
		}
	}
	if !switched {
		t.Fatalf("TauShell did not adopt the conversation started by the reset command; indicators=%v", output.indicators())
	}
	list, err := manager.Conversations(ctx, "live")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, item := range list {
		found[item.HarnessSessionID] = true
		t.Logf("history: %s current=%v tokens=%d", item.HarnessSessionID, item.Current, item.Tokens)
	}
	if !found[second] {
		t.Error("the new conversation is missing from History")
	}
	if !found[first] {
		t.Error("the interrupted conversation is missing from History")
	}

	// Resume the first conversation while the session is running.
	if _, err := manager.ResumeConversation(ctx, "live", first, 120, 40); err != nil {
		t.Fatalf("resume on a running session: %v", err)
	}
	if got := current(); got != first {
		t.Errorf("after resume the current conversation is %s, want %s", got, first)
	}
	time.Sleep(20 * time.Second)
	if got := current(); got != first {
		t.Errorf("tracking moved the resumed session to %s, want %s", got, first)
	}
	t.Logf("resumed running session; indicators=%v", output.indicators())
}
