package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/agentstate"
)

func sessionsWith(running bool) (Sessions, *agentstate.Registry, *[]string) {
	states := agentstate.NewRegistry()
	delivered := &[]string{}
	return Sessions{
		List: func(context.Context) ([]SessionInfo, error) {
			return []SessionInfo{{ID: "s1", Name: "one", RunningHere: running}}, nil
		},
		States: states,
		Deliver: func(id, text string, submit bool) error {
			*delivered = append(*delivered, id)
			return nil
		},
		Poll: 5 * time.Millisecond,
	}, states, delivered
}

func sendArgs(t *testing.T, text string, insertOnly bool) json.RawMessage {
	raw, err := json.Marshal(SendArgs{Session: "s1", Text: text, InsertOnly: insertOnly})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSendRefusalMatrix(t *testing.T) {
	cases := []struct {
		name       string
		running    bool
		report     func(*agentstate.Registry)
		insertOnly bool
		want       string // "" means delivered, otherwise a refusal fragment
	}{
		{"not running", false, nil, false, "not running"},
		{"never reported", true, nil, false, "not confirmed"},
		{"heuristic idle", true, func(r *agentstate.Registry) { r.Report("s1", agentstate.Idle, agentstate.Heuristic, false) }, false, "not confirmed"},
		{"waiting for input", true, func(r *agentstate.Registry) { r.Report("s1", agentstate.Input, agentstate.Cue, false) }, false, "waiting for an answer"},
		{"waiting even insert-only", true, func(r *agentstate.Registry) { r.Report("s1", agentstate.Input, agentstate.Cue, false) }, true, "waiting for an answer"},
		{"working", true, func(r *agentstate.Registry) { r.Report("s1", agentstate.Working, agentstate.Cue, false) }, false, "never interrupted"},
		{"working insert-only", true, func(r *agentstate.Registry) { r.Report("s1", agentstate.Working, agentstate.Cue, false) }, true, ""},
		{"idle just started", true, func(r *agentstate.Registry) { r.Report("s1", agentstate.Idle, agentstate.Cue, false) }, false, "not confirmed idle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions, states, delivered := sessionsWith(tc.running)
			if tc.report != nil {
				tc.report(states)
			}
			response := sessions.HandleSend(sendArgs(t, "hello", tc.insertOnly))
			if tc.want == "" {
				if !response.OK || len(*delivered) != 1 {
					t.Fatalf("expected delivery: %+v", response)
				}
				return
			}
			if response.OK || response.Error.Code != CodeRefused || !strings.Contains(response.Error.Message, tc.want) || len(*delivered) != 0 {
				t.Fatalf("expected refusal %q: %+v delivered=%v", tc.want, response, *delivered)
			}
		})
	}
}

func TestSendDeliversWhenIdleHeldAndRejectsBadInput(t *testing.T) {
	sessions, states, delivered := sessionsWith(true)
	states.Report("s1", agentstate.Idle, agentstate.Cue, false)
	time.Sleep(agentstate.IdleFor + 100*time.Millisecond)
	states.Report("s1", agentstate.Idle, agentstate.Cue, false)
	if response := sessions.HandleSend(sendArgs(t, "go", false)); !response.OK || len(*delivered) != 1 {
		t.Fatalf("confirmed idle must deliver: %+v", response)
	}
	states.Report("s1", agentstate.Idle, agentstate.Cue, true)
	if response := sessions.HandleSend(sendArgs(t, "go", false)); response.OK || !strings.Contains(response.Error.Message, "unsent text") {
		t.Fatalf("a draft must refuse a submit: %+v", response)
	}
	if response := sessions.HandleSend(sendArgs(t, strings.Repeat("x", MaxSendBytes+1), true)); response.OK || response.Error.Code != CodeBadRequest {
		t.Fatalf("oversized text: %+v", response)
	}
	if response := sessions.HandleSend(json.RawMessage(`{"session":"nope","text":"x"}`)); response.OK || response.Error.Code != CodeNotFound {
		t.Fatalf("unknown session: %+v", response)
	}
}

func TestMobileSendSubmitsDuringConfirmedWorkButStillProtectsPrompts(t *testing.T) {
	sessions, states, delivered := sessionsWith(true)
	states.Report("s1", agentstate.Working, agentstate.Cue, false)
	if response := sessions.HandleSend(sendArgs(t, "desktop send", false)); response.OK {
		t.Fatal("local control send should continue refusing during work")
	}
	if response := sessions.HandleMobileSend(sendArgs(t, "phone send", false)); !response.OK || len(*delivered) != 1 {
		t.Fatalf("mobile send should submit during confirmed work: %+v delivered=%v", response, *delivered)
	}

	states.Report("s1", agentstate.Idle, agentstate.Cue, true)
	time.Sleep(agentstate.IdleFor + 10*time.Millisecond)
	states.Report("s1", agentstate.Idle, agentstate.Cue, true)
	if response := sessions.HandleMobileSend(sendArgs(t, "phone with existing draft", false)); !response.OK || len(*delivered) != 2 {
		t.Fatalf("mobile send should submit alongside an existing draft: %+v delivered=%v", response, *delivered)
	}

	states.Report("s1", agentstate.Input, agentstate.Cue, false)
	if response := sessions.HandleMobileSend(sendArgs(t, "permission text", false)); response.OK ||
		response.Error.Code != CodeRefused || !strings.Contains(response.Error.Message, "waiting for an answer") {
		t.Fatalf("mobile send must not type into a permission prompt: %+v", response)
	}
	if len(*delivered) != 2 {
		t.Fatalf("permission prompt text was delivered: %v", *delivered)
	}
}

func TestWaitMatchesOnlyConfirmedState(t *testing.T) {
	sessions, states, _ := sessionsWith(true)
	states.Report("s1", agentstate.Working, agentstate.Cue, false)
	raw, _ := json.Marshal(WaitArgs{Session: "s1", For: "idle", TimeoutMS: 50})
	var result WaitResult
	response := sessions.HandleWait(raw)
	if err := json.Unmarshal(response.Data, &result); err != nil || !response.OK || result.Matched {
		t.Fatalf("working must not match idle: %+v %+v", response, result)
	}
	raw, _ = json.Marshal(WaitArgs{Session: "s1", For: "working", TimeoutMS: 50})
	response = sessions.HandleWait(raw)
	if err := json.Unmarshal(response.Data, &result); err != nil || !result.Matched {
		t.Fatalf("working must match: %+v", response)
	}
	if response := sessions.HandleWait(json.RawMessage(`{"session":"s1","for":"done"}`)); response.OK {
		t.Fatal("unknown state name must be rejected")
	}
}

func TestAnswerOnlyConfirmedClaudePrompts(t *testing.T) {
	states := agentstate.NewRegistry()
	var answered []string
	tool := "claude-code"
	sessions := Sessions{
		List: func(context.Context) ([]SessionInfo, error) {
			return []SessionInfo{{ID: "s1", Tool: tool, RunningHere: true}}, nil
		},
		States: states,
		Answer: func(id, answer string) error { answered = append(answered, answer); return nil },
	}
	ask := func(answer string) Response {
		raw, _ := json.Marshal(AnswerArgs{Session: "s1", Answer: answer})
		return sessions.HandleAnswer(raw)
	}
	if r := ask("yes"); r.OK || r.Error.Code != CodeRefused {
		t.Fatalf("no state yet must be refused: %+v", r)
	}
	states.Report("s1", agentstate.Working, agentstate.Cue, false)
	if r := ask("yes"); r.OK {
		t.Fatal("a working session must refuse an answer")
	}
	states.Report("s1", agentstate.Input, agentstate.Heuristic, false)
	if r := ask("yes"); r.OK {
		t.Fatal("a heuristic input state must refuse an answer")
	}
	states.Report("s1", agentstate.Input, agentstate.Cue, false)
	if r := ask("maybe"); r.OK || r.Error.Code != CodeBadRequest {
		t.Fatalf("only yes or no: %+v", r)
	}
	tool = "codex"
	if r := ask("yes"); r.OK {
		t.Fatal("an unverified harness must refuse an answer")
	}
	tool = "copilot-cli"
	if r := ask("no"); !r.OK {
		t.Fatalf("a confirmed Copilot prompt takes an answer: %+v", r)
	}
	tool = "claude-code"
	if r := ask("no"); !r.OK || len(answered) != 2 || answered[1] != "no" {
		t.Fatalf("a confirmed Claude prompt takes an answer: %+v %v", r, answered)
	}
}

func TestSendAllowsConfirmedQuestionOnly(t *testing.T) {
	sessions, states, delivered := sessionsWith(true)
	states.Report("s1", agentstate.Input, agentstate.Cue, false)
	if r := sessions.HandleSend(sendArgs(t, "2", false)); r.OK || len(*delivered) != 0 {
		t.Fatalf("a permission or unclassified prompt must refuse text: %+v", r)
	}
	states.Report("s1", agentstate.Input, agentstate.Cue, false)
	states.MarkQuestion("s1")
	if r := sessions.HandleSend(sendArgs(t, "2", false)); !r.OK || len(*delivered) != 1 {
		t.Fatalf("a confirmed question takes a reply: %+v", r)
	}
	states.Report("s1", agentstate.Input, agentstate.Cue, false)
	if r := sessions.HandleSend(sendArgs(t, "2", false)); r.OK {
		t.Fatal("the next report without the flag must refuse again")
	}
}

func TestStopOnlyStopsRunningSessions(t *testing.T) {
	for _, running := range []bool{false, true} {
		sessions, _, _ := sessionsWith(running)
		var stopped []string
		sessions.Stop = func(id string) error { stopped = append(stopped, id); return nil }
		response := sessions.HandleStop(json.RawMessage(`{"session":"s1"}`))
		if !running && (response.OK || response.Error.Code != CodeRefused || len(stopped) != 0) {
			t.Fatalf("a stopped session must be refused: %+v", response)
		}
		if running && (!response.OK || len(stopped) != 1 || stopped[0] != "s1") {
			t.Fatalf("a running session must stop: %+v %v", response, stopped)
		}
	}
	sessions, _, _ := sessionsWith(true)
	if response := sessions.HandleStop(json.RawMessage(`{"session":"s1"}`)); response.OK || response.Error.Code != CodeDisabled {
		t.Fatalf("no stop hook must report disabled: %+v", response)
	}
	if response := sessions.HandleStop(json.RawMessage(`{}`)); response.OK || response.Error.Code != CodeBadRequest {
		t.Fatalf("missing session: %+v", response)
	}
}

func TestAuditEntriesEncodeAsEmptyArray(t *testing.T) {
	raw, err := json.Marshal(NewAuditLog(10, nil).Entries())
	if err != nil || string(raw) != "[]" {
		t.Fatalf("an empty audit log must encode as [] (not null): %s %v", raw, err)
	}
}

func TestScreenReadsOnlyRunningSessions(t *testing.T) {
	for _, running := range []bool{false, true} {
		sessions, _, _ := sessionsWith(running)
		var asked int
		sessions.Screen = func(id string, rows int) ([]string, error) {
			asked = rows
			return []string{"", "one", "two", "", ""}, nil
		}
		response := sessions.HandleScreen(json.RawMessage(`{"session":"s1","rows":5}`))
		if !running && (response.OK || response.Error.Code != CodeRefused) {
			t.Fatalf("a stopped session must be refused: %+v", response)
		}
		if running {
			var result ScreenResult
			if err := json.Unmarshal(response.Data, &result); err != nil || !response.OK || len(result.Rows) != 3 || asked != MaxScreenRows {
				t.Fatalf("a running session must return trimmed rows: %+v", response)
			}
		}
	}
	sessions, _, _ := sessionsWith(true)
	if response := sessions.HandleScreen(json.RawMessage(`{"session":"s1"}`)); response.OK || response.Error.Code != CodeDisabled {
		t.Fatalf("no screen hook must report disabled: %+v", response)
	}
}

func TestStartLaunchesOnlyStoppedSessions(t *testing.T) {
	for _, running := range []bool{false, true} {
		sessions, _, _ := sessionsWith(running)
		var launched []string
		sessions.Launch = func(id, mode string) error { launched = append(launched, id+":"+mode); return nil }
		response := sessions.HandleStart(json.RawMessage(`{"session":"s1","mode":"restore"}`))
		if running && (response.OK || response.Error.Code != CodeRefused || len(launched) != 0) {
			t.Fatalf("a running session must be refused: %+v", response)
		}
		if !running && (!response.OK || len(launched) != 1 || launched[0] != "s1:restore") {
			t.Fatalf("a stopped session must launch: %+v %v", response, launched)
		}
	}
	sessions, _, _ := sessionsWith(false)
	if response := sessions.HandleStart(json.RawMessage(`{"session":"s1","mode":"bogus"}`)); response.OK || response.Error.Code != CodeBadRequest {
		t.Fatalf("unknown mode: %+v", response)
	}
}
