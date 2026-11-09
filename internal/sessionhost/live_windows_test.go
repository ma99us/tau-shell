package sessionhost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/presets"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/skills"
	"github.com/ma99us/tau-shell/internal/worktree"
)

var liveANSI = regexp.MustCompile(`\x1b(\[[0-9;?<>=]*[ -/]*[@-~]|\][^\x07\x1b]*(\x07|\x1b\\)|[()][A-Za-z0-9]|[=>78])`)

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
	output.text.Write(data)
	output.mu.Unlock()
}

func (output *liveOutput) count(fragment string) int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return strings.Count(liveANSI.ReplaceAllString(output.text.String(), ""), fragment)
}

func (output *liveOutput) mark() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.text.Len()
}

func (output *liveOutput) hasSince(mark int, fragments ...string) bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	text := output.text.String()
	if mark < 0 || mark > len(text) {
		return false
	}
	text = strings.ToLower(liveANSI.ReplaceAllString(text[mark:], ""))
	for _, fragment := range fragments {
		if strings.Contains(text, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func (output *liveOutput) hasAny(fragments ...string) bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	text := strings.ToLower(liveANSI.ReplaceAllString(output.text.String(), ""))
	for _, fragment := range fragments {
		if strings.Contains(text, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func (output *liveOutput) indicators() []string {
	output.mu.Lock()
	defer output.mu.Unlock()
	text := strings.ToLower(liveANSI.ReplaceAllString(output.text.String(), ""))
	var found []string
	for _, indicator := range []string{"error", "unknown option", "unrecognized", "trust", "yes", "no", "enter", "continue", "exit", "folder", "directory", "press", "select", "login", "not logged", "add-dir", "sandbox", "not a git", "permission"} {
		if strings.Contains(text, indicator) {
			found = append(found, indicator)
		}
	}
	return found
}

func codexAssistantTokenCount(sessionID, token string) (int, error) {
	path := harness.CodexRolloutFile(sessionID)
	if path == "" {
		return 0, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	count := 0
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var entry struct {
				Type    string `json:"type"`
				Payload struct {
					Type    string `json:"type"`
					Role    string `json:"role"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Type == "response_item" && entry.Payload.Type == "message" && entry.Payload.Role == "assistant" {
				for _, item := range entry.Payload.Content {
					if strings.Contains(item.Text, token) {
						count++
					}
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return count, nil
		}
		if err != nil {
			return count, err
		}
	}
}

func codexRolloutSignals(sessionID string) (map[string]int, bool, error) {
	path := harness.CodexRolloutFile(sessionID)
	signals := make(map[string]int)
	if path == "" {
		return signals, false, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return signals, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	slowCall := false
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		if entry.Type == "event_msg" && entry.Payload.Type != "" {
			signals[entry.Payload.Type]++
		}
		if entry.Type == "response_item" && entry.Payload.Type != "" {
			signals[entry.Payload.Type]++
			if strings.Contains(entry.Payload.Type, "call") &&
				strings.Contains(strings.ToLower(string(scanner.Bytes())), "start-sleep") {
				slowCall = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return signals, slowCall, nil
}

func codexSignalCount(signals map[string]int, fragment string) int {
	total := 0
	for name, count := range signals {
		if strings.Contains(strings.ToLower(name), strings.ToLower(fragment)) {
			total += count
		}
	}
	return total
}

func codexAlternateModel(ctx context.Context, current string) (string, error) {
	output, err := exec.CommandContext(ctx, "codex", "debug", "models").Output()
	if err != nil {
		return "", err
	}
	var catalog struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	if err := json.Unmarshal(output, &catalog); err != nil {
		return "", err
	}
	for _, model := range catalog.Models {
		if model.Slug != current && strings.HasPrefix(model.Slug, "gpt-") && model.Slug != "gpt-reserve" {
			return model.Slug, nil
		}
	}
	return "", errors.New("Codex has no alternate selectable GPT model in its current catalog")
}

func gsdSessionRecordSummary(sessionID string) (map[string]int, []string, map[string]int, string, error) {
	file, err := os.Open(harness.GSDSessionPath(sessionID))
	if err != nil {
		return nil, nil, nil, "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	types, models, stopReasons := map[string]int{}, map[string]bool{}, map[string]int{}
	thinkingLevel := ""
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry struct {
			Type          string `json:"type"`
			ModelID       string `json:"modelId"`
			ThinkingLevel string `json:"thinkingLevel"`
			Message       struct {
				Role       string `json:"role"`
				Model      string `json:"model"`
				StopReason string `json:"stopReason"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Type == "" {
			continue
		}
		types[entry.Type]++
		if entry.Type == "thinking_level_change" && entry.ThinkingLevel != "" {
			thinkingLevel = entry.ThinkingLevel
		}
		switch entry.Type {
		case "model_change":
			if entry.ModelID != "" {
				models[entry.ModelID] = true
			}
		case "message":
			if entry.Message.Role == "assistant" && entry.Message.Model != "" {
				models[entry.Message.Model] = true
			}
			if entry.Message.StopReason != "" {
				stopReasons[entry.Message.StopReason]++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, nil, "", err
	}
	modelIDs := make([]string, 0, len(models))
	for model := range models {
		modelIDs = append(modelIDs, model)
	}
	return types, modelIDs, stopReasons, thinkingLevel, nil
}

func gsdAssistantMessages(sessionID, token string) (int, int, error) {
	file, err := os.Open(harness.GSDSessionPath(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	messages, matching := 0, 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role       string `json:"role"`
				StopReason string `json:"stopReason"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Type != "message" || entry.Message.Role != "assistant" {
			continue
		}
		if entry.Message.StopReason == "" || entry.Message.StopReason == "toolUse" {
			continue
		}
		messages++
		if token != "" && bytes.Contains(line, []byte(token)) {
			matching++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return messages, matching, nil
}

func gsdToolCallCount(sessionID string) (int, error) {
	file, err := os.Open(harness.GSDSessionPath(sessionID))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	count := 0
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "message" {
			continue
		}
		for _, content := range entry.Message.Content {
			if content.Type == "toolCall" {
				count++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func gsdJSONLFiles(root string) []string {
	var paths []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".jsonl") {
			if relative, err := filepath.Rel(root, path); err == nil {
				paths = append(paths, relative)
			}
		}
		return nil
	})
	return paths
}

func createCodexTrustProfile(projectDir string) (string, string, error) {
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		codexHome = filepath.Join(home, ".codex")
	}
	file, err := os.CreateTemp(codexHome, "taushell-live-*.config.toml")
	if err != nil {
		return "", "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return "", "", errors.Join(err, removeErr)
		}
		return "", "", err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".config.toml")
	config := "[projects." + strconv.Quote(projectDir) + "]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return "", "", errors.Join(err, removeErr)
		}
		return "", "", err
	}
	return name, path, nil
}

func liveWorktree(t *testing.T, repo string) string {
	t.Helper()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"config", "user.email", "live@example.invalid"},
		{"config", "user.name", "live"},
		{"commit", "--allow-empty", "--quiet", "-m", "init"},
	} {
		if _, err := presets.RunGit(ctx, repo, nil, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	result, err := worktree.Create(ctx, worktree.Spec{Dir: repo, Name: "live-wt", Branch: "taushell/live-wt"})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	info, ok := worktree.Inspect(ctx, result.Path)
	if !ok || info.Name != "live-wt" || info.Branch != "taushell/live-wt" {
		t.Fatalf("worktree not detected as linked: %+v ok=%v", info, ok)
	}
	t.Logf("running harness in linked worktree %s (main %s)", result.Path, result.MainRepo)
	return result.Path
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
	var gsdSessionPath string
	if tool == session.GSD {
		t.Cleanup(func() {
			if gsdSessionPath == "" {
				return
			}
			if err := os.Remove(gsdSessionPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("remove disposable GSD session: %v", err)
			}
		})
	}
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TAUSHELL_LIVE_WORKTREE") == "1" {
		// Run the harness inside a TauShell-created linked worktree of a disposable repo.
		workingDir = liveWorktree(t, workingDir)
	}
	if tool == session.GSD {
		skillDir := filepath.Join(workingDir, ".agents", "skills", "taushell-live-probe")
		if err := os.MkdirAll(skillDir, 0o700); err != nil {
			t.Fatal(err)
		}
		skill := "---\nname: taushell-live-probe\ndescription: Return a fixed token when invoked by the TauShell compatibility probe.\n---\n" +
			"When the user asks you to use this skill, reply with exactly GSDSKILLTAU and nothing else."
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o600); err != nil {
			t.Fatal(err)
		}
		projectState := filepath.Join(workingDir, ".gsd")
		if err := os.MkdirAll(projectState, 0o700); err != nil {
			t.Fatal(err)
		}
		preferences := "---\nversion: 1\nskill_discovery: auto\nalways_use_skills:\n  - taushell-live-probe\n---\n"
		if err := os.WriteFile(filepath.Join(projectState, "PREFERENCES.md"), []byte(preferences), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var codexAdapter harness.Adapter
	var existingCodexIDs map[string]bool
	var codexProfile, codexProfilePath string
	var err error
	if tool == session.Codex {
		codexAdapter, err = harness.For(tool)
		if err != nil {
			t.Fatal(err)
		}
		existingCodexIDs, err = codexAdapter.ExistingSessionIDs(workingDir)
		if err != nil {
			t.Fatalf("snapshot existing Codex conversations: %v", err)
		}
		command := exec.Command("git", "init", "--quiet", workingDir)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("initialize disposable Codex project: %v (%s)", err, strings.TrimSpace(string(output)))
		}
		codexProfile, codexProfilePath, err = createCodexTrustProfile(workingDir)
		if err != nil {
			t.Fatalf("trust disposable Codex project: %v", err)
		}
		t.Cleanup(func() {
			if err := os.Remove(codexProfilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("remove temporary Codex trust profile: %v", err)
			}
		})
		skillDir := filepath.Join(root, "skill-source", "skills", "taushell-live-probe")
		if err := os.MkdirAll(skillDir, 0o700); err != nil {
			t.Fatal(err)
		}
		skill := "---\nname: taushell-live-probe\ndescription: Return a fixed token when invoked by the TauShell compatibility probe.\n---\n" +
			"When the user asks you to use this skill, reply with exactly CODEXSKILLTAU and nothing else."
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var args []string
	if raw := os.Getenv("TAUSHELL_LIVE_ARGS"); raw != "" {
		args = strings.Fields(raw)
	} else if tool == session.Codex && codexAdapter != nil {
		args = codexAdapter.DefaultLaunchArgs()
	}
	if tool == session.Codex {
		args = append(args, "--profile", codexProfile)
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
	if tool == session.Codex {
		library, err := skills.NewLibrary(filepath.Join(root, "tau-shell"), skills.Config{
			Enabled:      true,
			LocalSources: []string{filepath.Join(root, "skill-source")},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		manager.SetSkills(library)
		t.Cleanup(func() {
			createdIDs, err := codexAdapter.ExistingSessionIDs(workingDir)
			if err != nil {
				t.Errorf("find disposable Codex conversations for cleanup: %v", err)
				return
			}
			for id := range createdIDs {
				if existingCodexIDs[id] {
					continue
				}
				path := harness.CodexRolloutFile(id)
				if path == "" {
					t.Errorf("no rollout path found for disposable Codex conversation")
					continue
				}
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("remove disposable Codex rollout: %v", err)
				}
			}
		})
	}
	ctx := context.Background()
	report := func(format string, a ...any) { t.Logf("[%s] "+format, append([]any{tool}, a...)...) }

	if tool == session.GSD {
		adapter, err := harness.For(tool)
		if err != nil {
			t.Fatal(err)
		}
		report("GSD default launch args: %q", adapter.DefaultLaunchArgs())
	}
	snapshot, err := manager.Start(ctx, "live", false, 120, 40)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	report("started: status=%s hasResumeId=%v", snapshot.Status, snapshot.HasResumeID)
	time.Sleep(2 * time.Second)
	if tool == session.Codex && output.hasAny("trust") {
		if err := manager.Stop(ctx, "live"); err != nil {
			t.Errorf("stop after Codex folder trust prompt: %v", err)
		}
		time.Sleep(3 * time.Second)
		t.Skipf("Codex requested interactive folder trust for the disposable project; unattended compatibility checks need a pre-trusted folder (screen markers: %v)", output.indicators())
	}
	if tool == session.Codex && !manager.sessions["live"].isRunning() {
		report("Codex exited during startup; diagnostic indicators=%v", output.indicators())
		time.Sleep(3 * time.Second)
		t.Fatal("Codex exited before accepting a prompt")
	}
	time.Sleep(10 * time.Second)

	if tool == session.Codex {
		if err := manager.Write("live", []byte("Use the taushell-live-probe skill and follow its instructions. If it is unavailable in your loaded skills, reply exactly SKILLMISSINGTAU. Do not search the filesystem or use tools.")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		runtime, _ := manager.store.Runtime(ctx, "live")
		waitFor(60*time.Second, func() bool {
			runtime, _ = manager.store.Runtime(ctx, "live")
			return runtime.HarnessSessionID != "" || !manager.sessions["live"].isRunning()
		})
		runtime, _ = manager.store.Runtime(ctx, "live")
		hasID := runtime.HarnessSessionID != ""
		report("resume id recorded after skill probe: %v", hasID)
		if !hasID {
			if err := manager.Stop(ctx, "live"); err != nil {
				t.Errorf("stop after failed Codex launch: %v", err)
			}
			time.Sleep(3 * time.Second)
			t.Error("Codex did not create a resumable conversation after the skill probe")
			return
		}
		skillCount := 0
		var activityDuring string
		var skillReadErr error
		skillAnswered := waitFor(120*time.Second, func() bool {
			activity, _ := manager.HarnessActivity(ctx, "live")
			if activity == harness.ActivityWorking {
				activityDuring = activity
			}
			count, err := codexAssistantTokenCount(runtime.HarnessSessionID, "CODEXSKILLTAU")
			if err != nil {
				skillReadErr = err
				return true
			}
			return count > skillCount
		})
		count, countErr := codexAssistantTokenCount(runtime.HarnessSessionID, "CODEXSKILLTAU")
		report("TauShell custom-skill probe answered: %v", countErr == nil && skillAnswered && count > skillCount)
		if skillReadErr != nil {
			t.Errorf("read Codex skill response: %v", skillReadErr)
		} else if countErr != nil {
			t.Errorf("read Codex skill response: %v", countErr)
		} else if !skillAnswered || count <= skillCount {
			t.Error("Codex did not follow TauShell's per-launch custom skill")
		}
		activityIdle := waitFor(15*time.Second, func() bool {
			activity, _ := manager.HarnessActivity(ctx, "live")
			return activity == harness.ActivityIdle
		})
		report("Codex activity observed: working=%v idle=%v", activityDuring == harness.ActivityWorking, activityIdle)
		if activityDuring != harness.ActivityWorking {
			t.Error("TauShell did not observe Codex's working activity during the skill turn")
		}
		if !activityIdle {
			t.Error("TauShell did not observe Codex's idle activity after the skill turn")
		}
	}

	runtime, _ := manager.store.Runtime(ctx, "live")
	if tool == session.GSD && runtime.HarnessSessionID != "" {
		gsdSessionPath = harness.GSDSessionPath(runtime.HarnessSessionID)
	}
	pongCount := 0
	if tool == session.Codex {
		pongCount, err = codexAssistantTokenCount(runtime.HarnessSessionID, "PONGTAU")
		if err != nil {
			t.Fatal(err)
		}
	} else if tool == session.GSD {
		pongCount, _, err = gsdAssistantMessages(runtime.HarnessSessionID, "PONGTAU")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Write("live", []byte("Reply with exactly the single word PONGTAU and nothing else.")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := manager.Write("live", []byte("\r")); err != nil {
		t.Fatal(err)
	}
	answered := false
	gsdWorkingObserved := false
	var answerReadErr error
	if tool == session.Codex {
		answered = waitFor(120*time.Second, func() bool {
			count, err := codexAssistantTokenCount(runtime.HarnessSessionID, "PONGTAU")
			if err != nil {
				answerReadErr = err
				return true
			}
			return count > pongCount
		})
	} else if tool == session.GSD {
		answered = waitFor(120*time.Second, func() bool {
			activity, activityErr := manager.HarnessActivity(ctx, "live")
			if activityErr == nil && activity == harness.ActivityWorking {
				gsdWorkingObserved = true
			}
			count, matching, err := gsdAssistantMessages(runtime.HarnessSessionID, "PONGTAU")
			if err != nil {
				answerReadErr = err
				return true
			}
			return count > pongCount && matching > 0
		})
	} else {
		answered = waitFor(120*time.Second, func() bool { return output.count("PONGTAU") >= 3 })
	}
	report("model response recorded: %v", answered && answerReadErr == nil)
	if answerReadErr != nil {
		t.Errorf("read Codex first-turn response: %v", answerReadErr)
	}
	if tool == session.Codex && (!answered || answerReadErr != nil) {
		t.Error("Codex did not produce the expected first-turn response")
	}
	if tool == session.GSD && (!answered || answerReadErr != nil) {
		t.Fatal("GSD did not record the expected assistant reply to the first prompt")
	}
	if tool == session.GSD {
		idleObserved := waitFor(15*time.Second, func() bool {
			activity, err := manager.HarnessActivity(ctx, "live")
			return err == nil && activity == harness.ActivityIdle
		})
		report("GSD activity observed: working=%v idle=%v", gsdWorkingObserved, idleObserved)
		if !gsdWorkingObserved || !idleObserved {
			t.Errorf("GSD activity detection failed: working=%v idle=%v", gsdWorkingObserved, idleObserved)
		}
	}

	waitFor(30*time.Second, func() bool {
		runtime, _ = manager.store.Runtime(ctx, "live")
		return runtime.HarnessSessionID != "" || !manager.sessions["live"].isRunning()
	})
	runtime, _ = manager.store.Runtime(ctx, "live")
	hasID := runtime.HarnessSessionID != ""
	report("resume id recorded: %v", hasID)
	if tool == session.Codex && !hasID {
		if err := manager.Stop(ctx, "live"); err != nil {
			t.Errorf("stop after missing Codex conversation ID: %v", err)
		}
		time.Sleep(3 * time.Second)
		t.Error("Codex did not record a conversation ID")
		return
	}
	if model, err := manager.HarnessModel(ctx, "live"); true {
		report("current model: %q err=%v", model, err)
		if tool == session.GSD && (err != nil || model == "") {
			t.Errorf("GSD current model was not detected: model=%q err=%v", model, err)
		}
	}
	if tool == session.GSD {
		types, models, stopReasons, thinkingLevel, err := gsdSessionRecordSummary(runtime.HarnessSessionID)
		report("GSD session log metadata: path=%q types=%v modelIDs=%v stopReasons=%v thinkingLevel=%q error=%v", harness.GSDSessionPath(runtime.HarnessSessionID), types, models, stopReasons, thinkingLevel, err)
		if err != nil {
			t.Errorf("read GSD session log metadata: %v", err)
		}
		if strings.Contains(os.Getenv("TAUSHELL_LIVE_ARGS"), "--thinking") && thinkingLevel == "" {
			t.Error("GSD did not record the requested thinking level")
		}
		report("GSD session files in disposable test root: %v", gsdJSONLFiles(root))
	}
	if tool == session.Codex {
		activity, err := manager.HarnessActivity(ctx, "live")
		report("harness activity after first turn: %q available=%v", activity, err == nil && activity != "")
	}
	var tokens int64
	usageRecorded := waitFor(30*time.Second, func() bool {
		usageTotals, err := manager.SessionUsage(ctx, "live")
		if err != nil {
			return false
		}
		tokens = usageTotals.Total.Tokens
		return tokens > 0
	})
	report("usage recorded: %v", usageRecorded)

	stopStarted := time.Now()
	if err := manager.Stop(ctx, "live"); err != nil {
		t.Errorf("stop failed: %v", err)
	}
	stopped := manager.sessions["live"].snapshot()
	report("stop took %s: status=%s errorPresent=%v", time.Since(stopStarted).Round(100*time.Millisecond), stopped.Status, stopped.LastError != "")

	if !hasID {
		report("no resume id: restore is not possible")
		return
	}
	restored, err := manager.Start(ctx, "live", true, 120, 40)
	if err != nil {
		report("restore failed: %v", err)
		if tool == session.Codex {
			t.Errorf("Codex restore failed: %v", err)
		}
		return
	}
	report("restored: status=%s note=%q", restored.Status, restored.ResumeNote)
	restoredRuntime, err := manager.store.Runtime(ctx, "live")
	if err != nil {
		t.Fatalf("read restored Codex runtime: %v", err)
	}
	if tool == session.Codex && restoredRuntime.HarnessSessionID != runtime.HarnessSessionID {
		t.Errorf("restore selected conversation %q, want %q", restoredRuntime.HarnessSessionID, runtime.HarnessSessionID)
	}
	time.Sleep(12 * time.Second)
	before := 0
	if tool == session.Codex {
		before, err = codexAssistantTokenCount(runtime.HarnessSessionID, "PONGTAU")
		if err != nil {
			t.Fatal(err)
		}
	} else if tool == session.GSD {
		_, before, err = gsdAssistantMessages(runtime.HarnessSessionID, "PONGTAU")
		if err != nil {
			t.Fatal(err)
		}
	} else {
		before = output.count("PONGTAU")
	}
	if err := manager.Write("live", []byte("What single word did I ask you to reply with earlier? Answer with that word only.")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := manager.Write("live", []byte("\r")); err != nil {
		t.Fatal(err)
	}
	recalled := false
	var recallReadErr error
	if tool == session.Codex {
		recalled = waitFor(120*time.Second, func() bool {
			count, err := codexAssistantTokenCount(runtime.HarnessSessionID, "PONGTAU")
			if err != nil {
				recallReadErr = err
				return true
			}
			return count > before
		})
	} else if tool == session.GSD {
		recalled = waitFor(120*time.Second, func() bool {
			_, count, err := gsdAssistantMessages(runtime.HarnessSessionID, "PONGTAU")
			if err != nil {
				recallReadErr = err
				return true
			}
			return count > before
		})
	} else {
		recalled = waitFor(120*time.Second, func() bool { return output.count("PONGTAU") >= before+3 })
	}
	report("conversation recalled after restore: %v", recalled && recallReadErr == nil)
	if recallReadErr != nil {
		t.Errorf("read Codex restored response: %v", recallReadErr)
	}
	if tool == session.Codex && (!recalled || recallReadErr != nil) {
		t.Error("Codex did not recall the first-turn sentinel after restore")
	}
	if tool == session.GSD && (!recalled || recallReadErr != nil) {
		t.Error("GSD did not recall the first-turn sentinel after restore")
	}
	if tool == session.GSD && os.Getenv("TAUSHELL_LIVE_GSD_SKILLS") != "1" {
		report("GSD custom-skill probe skipped; set TAUSHELL_LIVE_GSD_SKILLS=1 to enable")
	}
	if tool == session.GSD && os.Getenv("TAUSHELL_LIVE_GSD_SKILLS") == "1" {
		beforeMessages, beforeSkill, err := gsdAssistantMessages(runtime.HarnessSessionID, "GSDSKILLTAU")
		if err != nil {
			t.Fatalf("read GSD skill baseline: %v", err)
		}
		if err := manager.Write("live", []byte("Use the taushell-live-probe skill and follow its instructions.")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		skillCompleted := waitFor(90*time.Second, func() bool {
			count, _, err := gsdAssistantMessages(runtime.HarnessSessionID, "")
			return err == nil && count > beforeMessages
		})
		if !skillCompleted {
			report("GSD skill probe did not finish")
			if err := manager.Stop(ctx, "live"); err != nil {
				t.Errorf("stop after GSD skill probe timed out: %v", err)
			}
			t.Fatal("GSD did not complete the custom-skill probe turn")
		}
		_, skillMatches, err := gsdAssistantMessages(runtime.HarnessSessionID, "GSDSKILLTAU")
		skillAnswered := err == nil && skillMatches > beforeSkill
		report("GSD project skill returned its sentinel after restore: %v (readError=%v)", skillAnswered, err)
		if err != nil {
			t.Errorf("read GSD skill response: %v", err)
		} else if !skillAnswered {
			t.Error("GSD did not follow its project-local custom skill")
		}
	}

	if tool == session.GSD {
		modelBefore, err := manager.HarnessModel(ctx, "live")
		if err != nil || modelBefore == "" {
			t.Errorf("read GSD model before picker probe: model=%q err=%v", modelBefore, err)
		} else {
			selectGSDModel := func(modelID string) (bool, error) {
				selectionMark := output.mark()
				if err := manager.Write("live", []byte("/model")); err != nil {
					return false, err
				}
				time.Sleep(500 * time.Millisecond)
				if err := manager.Write("live", []byte("\r")); err != nil {
					return false, err
				}
				pickerOpened := waitFor(10*time.Second, func() bool {
					return output.hasSince(selectionMark, "only showing models with configured credentials", "scope:")
				})
				if !pickerOpened {
					return false, nil
				}
				if err := manager.Write("live", []byte("\t")); err != nil {
					return false, err
				}
				time.Sleep(300 * time.Millisecond)
				if err := manager.Write("live", []byte(modelID)); err != nil {
					return false, err
				}
				time.Sleep(300 * time.Millisecond)
				if err := manager.Write("live", []byte("\r")); err != nil {
					return false, err
				}
				selected := waitFor(10*time.Second, func() bool {
					return output.hasSince(selectionMark, "model: "+modelID)
				})
				return selected, nil
			}
			modelChoice := "mai-code-1-flash-picker"
			modelSelected, err := selectGSDModel(modelChoice)
			if err != nil {
				t.Fatalf("select GSD model: %v", err)
			}
			modelAfter, modelErr := manager.HarnessModel(ctx, "live")
			modelSwitched := modelSelected && modelErr == nil && strings.HasSuffix(modelAfter, modelChoice)
			report("GSD /model picker switched models: %v (before=%q selected=%q after=%q error=%v)", modelSwitched, modelBefore, modelChoice, modelAfter, modelErr)
			if !modelSwitched {
				t.Error("GSD /model picker did not select the requested model")
				if err := manager.Write("live", []byte("\x1b")); err != nil {
					t.Errorf("close GSD model picker after unsuccessful selection: %v", err)
				}
			} else {
				modelRestored, err := selectGSDModel("mai-code-1.1-flash")
				if err != nil {
					t.Fatalf("restore GSD test model: %v", err)
				}
				modelAfter, modelErr := manager.HarnessModel(ctx, "live")
				report("GSD /model picker restored the working model: %v (restored=%q error=%v)", modelRestored && modelErr == nil && strings.HasSuffix(modelAfter, "mai-code-1.1-flash"), modelAfter, modelErr)
				if !modelRestored || modelErr != nil || !strings.HasSuffix(modelAfter, "mai-code-1.1-flash") {
					t.Error("GSD /model picker did not restore the known-working test model")
				}
			}
		}

		padding := strings.Repeat(" context-padding", 12000)
		_, beforeCompactReply, err := gsdAssistantMessages(runtime.HarnessSessionID, "COMPACTPREPARED")
		if err != nil {
			t.Fatalf("read GSD compaction preparation baseline: %v", err)
		}
		padding += " Reply with exactly COMPACTPREPARED."
		for len(padding) > 0 {
			chunkSize := min(len(padding), 16<<10)
			if err := manager.Write("live", []byte(padding[:chunkSize])); err != nil {
				t.Fatalf("send GSD compaction preparation prompt: %v", err)
			}
			padding = padding[chunkSize:]
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		prepared := waitFor(120*time.Second, func() bool {
			_, matching, err := gsdAssistantMessages(runtime.HarnessSessionID, "COMPACTPREPARED")
			return err == nil && matching > beforeCompactReply
		})
		if !prepared {
			t.Fatal("GSD did not complete the context-padding turn needed to verify compaction")
		}

		beforeCompact, _, _, _, err := gsdSessionRecordSummary(runtime.HarnessSessionID)
		if err != nil {
			t.Fatalf("read GSD compaction baseline: %v", err)
		}
		compactMark := output.mark()
		if err := manager.Write("live", []byte("/compact")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		compacted := waitFor(45*time.Second, func() bool {
			types, _, _, _, err := gsdSessionRecordSummary(runtime.HarnessSessionID)
			return err == nil && (types["compaction"] > beforeCompact["compaction"] ||
				output.hasSince(compactMark, "context compacted", "conversation compacted", "compaction complete"))
		})
		report("GSD /compact completed: %v", compacted)
		if !compacted {
			t.Error("GSD /compact did not produce an observable compaction result")
		}

		beforeTools, err := gsdToolCallCount(runtime.HarnessSessionID)
		if err != nil {
			t.Fatalf("read GSD interrupt baseline: %v", err)
		}
		beforeInterruptMessages, _, err := gsdAssistantMessages(runtime.HarnessSessionID, "")
		if err != nil {
			t.Fatalf("read GSD interrupt message baseline: %v", err)
		}
		if err := manager.Write("live", []byte("Use your terminal tool to run `powershell -NoProfile -Command \"Start-Sleep -Seconds 30\"`. Do not modify files. After it exits, reply with exactly GSDSLEPTAU.")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		slowCallStarted := waitFor(45*time.Second, func() bool {
			count, err := gsdToolCallCount(runtime.HarnessSessionID)
			return err == nil && count > beforeTools
		})
		report("GSD started the controlled sleep command: %v", slowCallStarted)
		if !slowCallStarted {
			t.Error("GSD did not start the controlled sleep command")
		} else {
			interruptStarted := time.Now()
			if err := manager.Write("live", []byte("\x1b")); err != nil {
				t.Fatalf("send GSD interrupt key: %v", err)
			}
			interrupted := waitFor(30*time.Second, func() bool {
				count, _, err := gsdAssistantMessages(runtime.HarnessSessionID, "")
				return err == nil && count > beforeInterruptMessages
			})
			idleAfterInterrupt := waitFor(15*time.Second, func() bool {
				activity, err := manager.HarnessActivity(ctx, "live")
				return err == nil && activity == harness.ActivityIdle
			})
			activity, activityErr := manager.HarnessActivity(ctx, "live")
			report("single Escape interrupted GSD turn: %v (idle=%v activity=%q activityError=%v elapsed=%s)", interrupted && idleAfterInterrupt, idleAfterInterrupt, activity, activityErr, time.Since(interruptStarted).Round(time.Millisecond))
			if !interrupted || !idleAfterInterrupt {
				t.Error("one Escape did not return GSD to idle after the controlled command")
			}
		}
	}
	if tool == session.Codex {
		skillBeforeRestore, err := codexAssistantTokenCount(runtime.HarnessSessionID, "CODEXSKILLTAU")
		if err != nil {
			t.Fatalf("read restored Codex skill baseline: %v", err)
		}
		if err := manager.Write("live", []byte("Use the taushell-live-probe skill and follow its instructions.")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		skillAfterRestore := waitFor(120*time.Second, func() bool {
			count, err := codexAssistantTokenCount(runtime.HarnessSessionID, "CODEXSKILLTAU")
			return err == nil && count > skillBeforeRestore
		})
		report("TauShell custom skill works after restore: %v", skillAfterRestore)
		if !skillAfterRestore {
			t.Error("Codex did not follow TauShell's skill after restoring the conversation")
		}

		checkSlashCommand := func(command string, expectedOutput ...string) {
			t.Helper()
			mark := output.mark()
			beforeSignals, _, beforeErr := codexRolloutSignals(runtime.HarnessSessionID)
			if beforeErr != nil {
				t.Errorf("read Codex %s baseline: %v", command, beforeErr)
				return
			}
			if err := manager.Write("live", []byte(command)); err != nil {
				t.Errorf("send Codex %s: %v", command, err)
				return
			}
			time.Sleep(500 * time.Millisecond)
			if err := manager.Write("live", []byte("\r")); err != nil {
				t.Errorf("submit Codex %s: %v", command, err)
				return
			}
			time.Sleep(2 * time.Second)
			unsupported := output.hasSince(mark, "unknown command", "unknown slash command", "not a valid command")
			afterSignals, _, afterErr := codexRolloutSignals(runtime.HarnessSessionID)
			forwardedToModel := afterErr == nil &&
				(codexSignalCount(afterSignals, "user_message") > codexSignalCount(beforeSignals, "user_message") ||
					codexSignalCount(afterSignals, "task_started") > codexSignalCount(beforeSignals, "task_started"))
			expectedSeen := len(expectedOutput) == 0 || output.hasSince(mark, expectedOutput...)
			report("%s command diagnostics: unsupported=%v forwardedToModel=%v expectedOutput=%v readError=%v", command, unsupported, forwardedToModel, expectedSeen, afterErr != nil)
			if unsupported || forwardedToModel || !expectedSeen {
				t.Errorf("Codex rejected the declared %s command", command)
			}
		}

		checkSlashCommand("/status", "context", "rate limit", "thread")

		modelBefore, err := manager.HarnessModel(ctx, "live")
		if err != nil || modelBefore == "" {
			t.Errorf("read Codex model before selection: model=%q err=%v", modelBefore, err)
		} else {
			pickerMark := output.mark()
			if err := manager.Write("live", []byte("/model")); err != nil {
				t.Fatalf("open Codex model picker: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			if err := manager.Write("live", []byte("\r")); err != nil {
				t.Fatalf("submit Codex model picker command: %v", err)
			}
			pickerOpened := waitFor(10*time.Second, func() bool {
				return output.hasSince(pickerMark, modelBefore)
			})
			report("bare /model opened picker containing current model: %v", pickerOpened)
			if !pickerOpened {
				t.Error("Codex /model did not display the model picker")
			}
			if err := manager.Write("live", []byte("\x1b")); err != nil {
				t.Errorf("close Codex model picker: %v", err)
			}
			time.Sleep(300 * time.Millisecond)

			modelChoice, err := codexAlternateModel(ctx, modelBefore)
			if err != nil {
				t.Fatalf("find an alternate Codex model: %v", err)
			}
			if err := manager.Write("live", []byte("/model "+modelChoice)); err != nil {
				t.Fatalf("select Codex model %s: %v", modelChoice, err)
			}
			time.Sleep(500 * time.Millisecond)
			if err := manager.Write("live", []byte("\r")); err != nil {
				t.Fatalf("submit Codex model selection: %v", err)
			}
			modelTokenBefore, err := codexAssistantTokenCount(runtime.HarnessSessionID, "MODELSELECTTAU")
			if err != nil {
				t.Fatalf("read Codex model-selection baseline: %v", err)
			}
			if err := manager.Write("live", []byte("Reply with exactly MODELSELECTTAU and nothing else.")); err != nil {
				t.Fatal(err)
			}
			time.Sleep(500 * time.Millisecond)
			if err := manager.Write("live", []byte("\r")); err != nil {
				t.Fatal(err)
			}
			modelAnswered := waitFor(120*time.Second, func() bool {
				count, err := codexAssistantTokenCount(runtime.HarnessSessionID, "MODELSELECTTAU")
				return err == nil && count > modelTokenBefore
			})
			modelAfter, modelErr := manager.HarnessModel(ctx, "live")
			report("Codex /model changed model: %v", modelAnswered && modelErr == nil && modelAfter == modelChoice)
			if !modelAnswered || modelErr != nil || modelAfter != modelChoice {
				t.Errorf("Codex model selection did not switch models: before=%q selected=%q after=%q err=%v", modelBefore, modelChoice, modelAfter, modelErr)
			}
		}

		beforeCompact, _, err := codexRolloutSignals(runtime.HarnessSessionID)
		if err != nil {
			t.Fatalf("read Codex compaction baseline: %v", err)
		}
		compactMark := output.mark()
		if err := manager.Write("live", []byte("/compact")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		compacted := waitFor(90*time.Second, func() bool {
			signals, _, err := codexRolloutSignals(runtime.HarnessSessionID)
			if err != nil {
				return false
			}
			return codexSignalCount(signals, "compact") > codexSignalCount(beforeCompact, "compact") ||
				output.hasSince(compactMark, "context compacted", "conversation compacted", "compaction complete")
		})
		report("Codex /compact completed: %v", compacted)
		if !compacted {
			signals, _, _ := codexRolloutSignals(runtime.HarnessSessionID)
			report("Codex compaction event types: %v", signals)
			t.Error("Codex /compact did not produce an observable compaction result")
		}

		interruptSignalsBefore, _, err := codexRolloutSignals(runtime.HarnessSessionID)
		if err != nil {
			t.Fatalf("read Codex interrupt baseline: %v", err)
		}
		if err := manager.Write("live", []byte("Use your terminal tool to run `powershell -NoProfile -Command \"Start-Sleep -Seconds 45\"`. Do not modify any files. After it exits, reply with exactly SLEPTTAU.")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := manager.Write("live", []byte("\r")); err != nil {
			t.Fatal(err)
		}
		slowCallStarted := waitFor(30*time.Second, func() bool {
			signals, started, err := codexRolloutSignals(runtime.HarnessSessionID)
			started = started || codexSignalCount(signals, "custom_tool_call") > codexSignalCount(interruptSignalsBefore, "custom_tool_call")
			return err == nil && started
		})
		report("Codex started the controlled slow command: %v", slowCallStarted)
		if !slowCallStarted {
			signals, _, signalErr := codexRolloutSignals(runtime.HarnessSessionID)
			activity, activityErr := manager.HarnessActivity(ctx, "live")
			report("Codex interrupt probe diagnostics: signals=%v signalError=%v activity=%q activityError=%v responseProduced=%v", signals, signalErr != nil, activity, activityErr != nil, output.hasAny("slepttau"))
			t.Error("Codex did not start the controlled sleep command for interrupt verification")
		} else {
			interruptStarted := time.Now()
			if err := manager.Write("live", []byte("\x1b")); err != nil {
				t.Fatalf("send Codex interrupt key: %v", err)
			}
			interrupted := waitFor(15*time.Second, func() bool {
				signals, _, err := codexRolloutSignals(runtime.HarnessSessionID)
				if err != nil {
					return false
				}
				activity, activityErr := manager.HarnessActivity(ctx, "live")
				return codexSignalCount(signals, "abort") > codexSignalCount(interruptSignalsBefore, "abort") ||
					(activityErr == nil && activity == harness.ActivityIdle)
			})
			signals, _, signalErr := codexRolloutSignals(runtime.HarnessSessionID)
			aborted := signalErr == nil && codexSignalCount(signals, "abort") > codexSignalCount(interruptSignalsBefore, "abort")
			report("single Escape interrupted Codex turn: %v (turn aborted=%v, elapsed=%s)", interrupted && aborted, aborted, time.Since(interruptStarted).Round(time.Millisecond))
			if !interrupted || !aborted {
				t.Error("one Escape did not produce a Codex turn-aborted event")
			}
		}

	}
	if err := manager.Stop(ctx, "live"); err != nil {
		t.Errorf("stop after restore failed: %v", err)
	}
}
