package control

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ma99us/tau-shell/internal/agentstate"
)

// Sessions serves the session-addressed operations. It holds no terminal
// content: sent text passes through Deliver once, and screen text is fetched
// from the window per request and passed straight on; neither is stored or logged.
type Sessions struct {
	// List returns sessions without Activity; Sessions fills that in.
	List   func(context.Context) ([]SessionInfo, error)
	States *agentstate.Registry
	// Deliver hands text to the UI, which types it through the terminal.
	Deliver func(sessionID, text string, submit bool) error
	// Launch asks the window to start or restore a stopped session.
	Launch func(sessionID, mode string) error
	// Screen returns the last rows of a running session's terminal text from
	// the window. Nil or an error means it is not available.
	Screen func(sessionID string, rows int) ([]string, error)
	// Answer asks the window to press the key for a yes or no on a waiting prompt;
	// the window re-checks the prompt before pressing anything.
	Answer func(sessionID, answer string) error
	// Poll is how often Wait re-reads the state; tests shorten it.
	Poll time.Duration
}

func (sessions Sessions) find(id string) (SessionInfo, *Error) {
	items, err := sessions.List(context.Background())
	if err != nil {
		return SessionInfo{}, &Error{Code: CodeInternal, Message: "list sessions failed"}
	}
	for _, item := range items {
		if item.ID == id {
			item.Activity = sessions.activity(item)
			return item, nil
		}
	}
	return SessionInfo{}, &Error{Code: CodeNotFound, Message: "no session with that id"}
}

func (sessions Sessions) activity(item SessionInfo) agentstate.View {
	if !item.RunningHere {
		return agentstate.View{State: agentstate.Stopped}
	}
	return sessions.States.Get(item.ID)
}

// HandleList returns all sessions with their activity.
func (sessions Sessions) HandleList() Response {
	items, err := sessions.List(context.Background())
	if err != nil {
		return Failure(CodeInternal, "list sessions failed")
	}
	for index := range items {
		items[index].Activity = sessions.activity(items[index])
	}
	return OK(items)
}

// HandleWait blocks up to MaxWait until the session is in the wanted state.
// Idle, input and working only count when confirmed.
func (sessions Sessions) HandleWait(raw json.RawMessage) Response {
	var args WaitArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Session == "" {
		return Failure(CodeBadRequest, "wait needs a session")
	}
	want := agentstate.State(args.For)
	if want != agentstate.Idle && want != agentstate.Input && want != agentstate.Working {
		return Failure(CodeBadRequest, "wait --for must be idle, input or working")
	}
	timeout := time.Duration(args.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > MaxWait {
		timeout = MaxWait
	}
	poll := sessions.Poll
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for {
		item, failure := sessions.find(args.Session)
		if failure != nil {
			return Response{Error: failure}
		}
		if !item.RunningHere {
			return Failure(CodeRefused, "the session is not running")
		}
		matched := item.Activity.State == want && item.Activity.Confirmed
		if matched || !time.Now().Before(deadline) {
			return OK(WaitResult{Matched: matched, Activity: item.Activity})
		}
		time.Sleep(poll)
	}
}

// HandleSend refuses unless the session is confirmed ready, so a working turn
// is never interrupted and a permission prompt never receives text.
func (sessions Sessions) HandleSend(raw json.RawMessage) Response {
	var args SendArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Session == "" || args.Text == "" {
		return Failure(CodeBadRequest, "send needs a session and text")
	}
	if len(args.Text) > MaxSendBytes || !utf8.ValidString(args.Text) {
		return Failure(CodeBadRequest, "text is too large or not valid UTF-8")
	}
	item, failure := sessions.find(args.Session)
	if failure != nil {
		return Response{Error: failure}
	}
	if reason := Refusal(item, args.InsertOnly); reason != "" {
		return Failure(CodeRefused, reason)
	}
	if err := sessions.Deliver(item.ID, args.Text, !args.InsertOnly); err != nil {
		return Failure(CodeInternal, "the window could not accept the text")
	}
	return OK(nil)
}

// HandleAnswer answers a harness permission prompt with a fixed yes or no. It is
// the opposite gate of send: only a confirmed cue-based input state, only for Claude
// Code (the one harness whose keys are verified), and never free text.
func (sessions Sessions) HandleAnswer(raw json.RawMessage) Response {
	var args AnswerArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Session == "" || (args.Answer != "yes" && args.Answer != "no") {
		return Failure(CodeBadRequest, "answer needs a session and yes or no")
	}
	item, failure := sessions.find(args.Session)
	if failure != nil {
		return Response{Error: failure}
	}
	if reason := AnswerRefusal(item); reason != "" {
		return Failure(CodeRefused, reason)
	}
	if sessions.Answer == nil {
		return Failure(CodeDisabled, "answering is not available")
	}
	if err := sessions.Answer(item.ID, args.Answer); err != nil {
		return Failure(CodeInternal, "the window could not accept the answer")
	}
	return OK(nil)
}

// AnswerRefusal returns why a prompt must not be answered remotely, or "".
func AnswerRefusal(item SessionInfo) string {
	switch {
	case !item.RunningHere:
		return "the session is not running"
	case item.Tool != "claude-code":
		return "answering from the phone is only verified for Claude Code"
	case item.Activity.State != agentstate.Input || !item.Activity.Confirmed || item.Activity.Source != agentstate.Cue:
		return "the harness is not confirmed to be waiting for an answer"
	}
	return ""
}

// HandleStart hands a stopped session to the window, which launches it the same
// way the Start, Resume and Start fresh buttons do.
func (sessions Sessions) HandleStart(raw json.RawMessage) Response {
	var args StartArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Session == "" {
		return Failure(CodeBadRequest, "start needs a session")
	}
	if args.Mode != "start" && args.Mode != "restore" && args.Mode != "fresh" {
		return Failure(CodeBadRequest, "start mode must be start, restore or fresh")
	}
	item, failure := sessions.find(args.Session)
	if failure != nil {
		return Response{Error: failure}
	}
	if item.RunningHere {
		return Failure(CodeRefused, "the session is already running")
	}
	if err := sessions.Launch(item.ID, args.Mode); err != nil {
		return Failure(CodeInternal, "the window could not accept the request")
	}
	return OK(nil)
}

// HandleScreen returns the visible terminal text of a running session. Only the
// phone server routes to it, and only when the user turned screen sharing on.
func (sessions Sessions) HandleScreen(raw json.RawMessage) Response {
	var args ScreenArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Session == "" {
		return Failure(CodeBadRequest, "screen needs a session")
	}
	if sessions.Screen == nil {
		return Failure(CodeDisabled, "screen sharing is off on the PC")
	}
	if args.Rows <= 0 {
		args.Rows = DefaultScreenRows
	}
	if args.Rows > MaxScreenRows {
		args.Rows = MaxScreenRows
	}
	item, failure := sessions.find(args.Session)
	if failure != nil {
		return Response{Error: failure}
	}
	if !item.RunningHere {
		return Failure(CodeRefused, "the session is not running")
	}
	rows, err := sessions.Screen(item.ID, args.Rows)
	if err != nil {
		return Failure(CodeRefused, err.Error())
	}
	return OK(ScreenResult{Rows: ClipScreen(rows, args.Rows)})
}

// ClipScreen keeps the last n rows, drops trailing blank rows and caps the total size.
func ClipScreen(rows []string, n int) []string {
	for len(rows) > 0 && strings.TrimSpace(rows[len(rows)-1]) == "" {
		rows = rows[:len(rows)-1]
	}
	if len(rows) > n {
		rows = rows[len(rows)-n:]
	}
	total := 0
	out := make([]string, 0, len(rows))
	for index := len(rows) - 1; index >= 0; index-- {
		row := strings.ToValidUTF8(rows[index], "?")
		total += len(row) + 1
		if total > MaxScreenBytes {
			break
		}
		out = append([]string{row}, out...)
	}
	return out
}

// Refusal returns why text must not be typed into the session, or "".
// A submitted send needs a confirmed idle harness with an empty input line;
// an insert-only send also works while the harness is busy, but never while
// it waits for an answer.
func Refusal(item SessionInfo, insertOnly bool) string {
	activity := item.Activity
	switch {
	case !item.RunningHere:
		return "the session is not running"
	case activity.State == agentstate.Input && activity.Confirmed:
		return "the harness is waiting for an answer; answer it in the terminal first"
	case activity.State == agentstate.Unknown || activity.State == agentstate.Stopped || activity.Source != agentstate.Cue:
		return "the session state is not confirmed (is the TauShell window open?)"
	case insertOnly:
		return ""
	case activity.State == agentstate.Working:
		return "the harness is working; a running turn is never interrupted"
	case activity.State != agentstate.Idle || !activity.Confirmed:
		return "the session is not confirmed idle yet"
	case activity.HasDraft:
		return "there is unsent text in the input line"
	}
	return ""
}
