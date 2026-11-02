package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/agentstate"
	"github.com/ma99us/tau-shell/internal/control"
)

func fakeBackend(instances []control.Instance, enabled bool, data any) ctlBackend {
	return ctlBackend{
		instances: func() ([]control.Instance, error) { return instances, nil },
		call: func(pipe string, _ control.Request) (control.Response, error) {
			raw, _ := json.Marshal(data)
			return control.Response{OK: true, Data: raw}, nil
		},
		enabled: func() (bool, error) { return enabled, nil },
	}
}

func codeOf(err error) int {
	var coded *ctlError
	if errors.As(err, &coded) {
		return coded.code
	}
	return 1
}

func TestCtlListJSONAndTable(t *testing.T) {
	sessions := []control.SessionInfo{{ID: "a", Name: "Alpha", Tool: "claude-code", Status: "running", WorkingDir: `C:\p`}}
	backend := fakeBackend([]control.Instance{{PipeName: "p1", Hello: control.Hello{PID: 1}}}, true, sessions)

	var out bytes.Buffer
	if err := runCtlWith([]string{"list", "--json"}, &out, backend); err != nil {
		t.Fatal(err)
	}
	var decoded []control.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || len(decoded) != 1 || decoded[0].ID != "a" {
		t.Fatalf("json output %q: %v", out.String(), err)
	}

	out.Reset()
	if err := runCtlWith([]string{"list"}, &out, backend); err != nil || !strings.Contains(out.String(), "Alpha") || !strings.Contains(out.String(), "ID") {
		t.Fatalf("table output %q: %v", out.String(), err)
	}
}

func TestCtlExitCodes(t *testing.T) {
	var out bytes.Buffer
	if err := runCtlWith([]string{"list"}, &out, fakeBackend(nil, false, nil)); codeOf(err) != exitCtlDisabled {
		t.Fatalf("disabled channel: %v", err)
	}
	if err := runCtlWith([]string{"list"}, &out, fakeBackend(nil, true, nil)); codeOf(err) != exitCtlNotFound {
		t.Fatalf("not running: %v", err)
	}
	if err := runCtlWith([]string{"bogus"}, &out, fakeBackend(nil, true, nil)); codeOf(err) != exitCtlUsage {
		t.Fatalf("usage: %v", err)
	}
	backend := fakeBackend([]control.Instance{{PipeName: "p1", Hello: control.Hello{PID: 1}}}, true, []int{})
	if err := runCtlWith([]string{"list", "--instance", "9"}, &out, backend); codeOf(err) != exitCtlNotFound {
		t.Fatalf("unknown instance: %v", err)
	}
}

func TestControlHandlerRejectsUnknownOps(t *testing.T) {
	handler := controlHandler{}
	if response := handler.Handle(control.Caller{}, control.Request{Op: "start"}); response.Error == nil || response.Error.Code != control.CodeUnknownOp {
		t.Fatalf("start is not available yet: %+v", response)
	}
}

func TestCtlSendAndWaitExitCodes(t *testing.T) {
	instances := []control.Instance{{PipeName: "p1", Hello: control.Hello{PID: 1}}}
	var seen control.Request
	respond := func(response control.Response) ctlBackend {
		backend := fakeBackend(instances, true, nil)
		backend.call = func(_ string, request control.Request) (control.Response, error) {
			seen = request
			return response, nil
		}
		return backend
	}
	var out bytes.Buffer

	refused := respond(control.Failure(control.CodeRefused, "the harness is working"))
	if err := runCtlWith([]string{"send", "s1", "hello", "world"}, &out, refused); codeOf(err) != exitCtlRefused {
		t.Fatalf("refusal: %v", err)
	}
	var sent control.SendArgs
	_ = json.Unmarshal(seen.Args, &sent)
	if sent.Session != "s1" || sent.Text != "hello world" || sent.InsertOnly {
		t.Fatalf("send args: %+v", sent)
	}

	if err := runCtlWith([]string{"send", "--insert-only", "--stdin", "s1"}, &out, func() ctlBackend {
		backend := respond(control.OK(nil))
		backend.input = strings.NewReader("from stdin\r\n")
		return backend
	}()); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(seen.Args, &sent)
	if sent.Text != "from stdin" || !sent.InsertOnly {
		t.Fatalf("stdin send args: %+v", sent)
	}
	if err := runCtlWith([]string{"send", "s1"}, &out, respond(control.OK(nil))); codeOf(err) != exitCtlUsage {
		t.Fatalf("missing text: %v", err)
	}
	if err := runCtlWith([]string{"send", "nope", "x"}, &out, respond(control.Failure(control.CodeNotFound, "no session"))); codeOf(err) != exitCtlNotFound {
		t.Fatalf("not found: %v", err)
	}

	notYet := fakeBackend(instances, true, control.WaitResult{Matched: false})
	if err := runCtlWith([]string{"wait", "s1", "--for", "idle", "--timeout", "30ms"}, &out, notYet); codeOf(err) != exitCtlTimeout {
		t.Fatalf("wait timeout: %v", err)
	}
	out.Reset()
	reached := fakeBackend(instances, true, control.WaitResult{Matched: true, Activity: agentstate.View{State: agentstate.Idle, Confirmed: true}})
	if err := runCtlWith([]string{"wait", "s1", "--timeout", "1s"}, &out, reached); err != nil || strings.TrimSpace(out.String()) != "idle" {
		t.Fatalf("wait matched: %q %v", out.String(), err)
	}
}
