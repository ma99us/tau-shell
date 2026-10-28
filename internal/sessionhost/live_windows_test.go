package sessionhost

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/usage"
)

var ansiPattern = regexp.MustCompile(`\x1b(\[[0-9;?<>=]*[ -/]*[@-~]|\][^\x07\x1b]*(\x07|\x1b\\)|[()][A-Za-z0-9]|[=>78])`)

type liveOutput struct {
	mu   sync.Mutex
	text strings.Builder
}

func (output *liveOutput) add(event Event, id string) {
	if event.Kind != EventOutput || event.SessionID != id {
		return
	}
	data, err := base64.StdEncoding.DecodeString(event.DataBase64)
	if err != nil {
		return
	}
	output.mu.Lock()
	output.text.WriteString(string(data))
	output.mu.Unlock()
}

func (output *liveOutput) clean() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return ansiPattern.ReplaceAllString(output.text.String(), "")
}

func waitFor(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return condition()
}

// TestLiveHarness drives a real harness through the session manager and
// ConPTY. It spends a few model calls and writes to the harness's real
// session store, so it only runs when TAUSHELL_LIVE names a harness:
// gsd, opencode or codex. TAUSHELL_LIVE_ARGS adds launch arguments.
func TestLiveHarness(t *testing.T) {
	tool := session.Harness(os.Getenv("TAUSHELL_LIVE"))
	if tool == "" {
		t.Skip("set TAUSHELL_LIVE=gsd|opencode|codex to run")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "sessions")
	workingDir := filepath.Join(root, "project")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var args []string
	if raw := os.Getenv("TAUSHELL_LIVE_ARGS"); raw != "" {
		args = strings.Fields(raw)
	}
	if _, err := session.SaveConfig(configDir, session.Config{ID: "live", Name: "live", Tool: tool, WorkingDir: workingDir, LaunchArgs: args}); err != nil {
		t.Fatal(err)
	}
	output := &liveOutput{}
	manager, err := New(configDir, filepath.Join(root, "state.db"), func(event Event) { output.add(event, "live") })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())
	ctx := context.Background()
	report := func(format string, a ...any) { t.Logf("[%s] "+format, append([]any{tool}, a...)...) }
	dump := func(label string) {
		text := output.clean()
		if len(text) > 1800 {
			text = text[len(text)-1800:]
		}
		report("%s screen tail:\n%s", label, text)
	}

	snapshot, err := manager.Start(ctx, "live", false, 120, 40)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	report("started: status=%s hasResumeId=%v", snapshot.Status, snapshot.HasResumeID)
	time.Sleep(12 * time.Second)
	dump("after startup")

	if err := manager.Write("live", []byte("Reply with exactly the single word PONGTAU and nothing else.")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := manager.Write("live", []byte("\r")); err != nil {
		t.Fatal(err)
	}
	answered := waitFor(120*time.Second, func() bool { return strings.Count(output.clean(), "PONGTAU") >= 3 })
	report("model answered in the terminal: %v", answered)
	dump("after prompt")

	runtime, _ := manager.store.Runtime(ctx, "live")
	report("harness session id after prompt: %q", runtime.HarnessSessionID)
	hasID := waitFor(30*time.Second, func() bool {
		runtime, _ = manager.store.Runtime(ctx, "live")
		return runtime.HarnessSessionID != ""
	})
	report("resume id recorded: %v (%q)", hasID, runtime.HarnessSessionID)
	if model, err := manager.HarnessModel(ctx, "live"); true {
		report("current model: %q err=%v", model, err)
	}
	var tokens int64
	waitFor(30*time.Second, func() bool {
		usageTotals, err := manager.SessionUsage(ctx, "live")
		if err != nil {
			return false
		}
		tokens = usageTotals.Total.Tokens
		if tokens > 0 {
			report("usage: tokens=%d usd=%.4f unpriced=%v", usageTotals.Total.Tokens, usageTotals.Total.USD, usageTotals.Total.Unpriced)
		}
		return tokens > 0
	})
	files := usage.LogFiles(tool, runtime.HarnessSessionID)
	report("usage log files: %v", files)

	stopStarted := time.Now()
	if err := manager.Stop(ctx, "live"); err != nil {
		t.Errorf("stop failed: %v", err)
	}
	stopped := manager.sessions["live"].snapshot()
	report("stop took %s: status=%s lastError=%q", time.Since(stopStarted).Round(100*time.Millisecond), stopped.Status, stopped.LastError)
	dump("after stop")

	if !hasID {
		report("no resume id: restore is not possible")
		return
	}
	before := strings.Count(output.clean(), "PONGTAU")
	restored, err := manager.Start(ctx, "live", true, 120, 40)
	if err != nil {
		report("RESTORE FAILED: %v", err)
		return
	}
	report("restored: status=%s note=%q", restored.Status, restored.ResumeNote)
	time.Sleep(12 * time.Second)
	if err := manager.Write("live", []byte("What single word did I ask you to reply with earlier? Answer with that word only.")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	_ = manager.Write("live", []byte("\r"))
	recalled := waitFor(120*time.Second, func() bool { return strings.Count(output.clean(), "PONGTAU") >= before+3 })
	report("conversation recalled after restore: %v", recalled)
	dump("after restore prompt")
	_ = manager.Stop(ctx, "live")
}
