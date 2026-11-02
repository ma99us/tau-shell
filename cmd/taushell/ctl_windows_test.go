package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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

func TestControlHandlerListsWithoutPromptData(t *testing.T) {
	handler := controlHandler{list: nil}
	if response := handler.Handle(control.Caller{}, control.Request{Op: "send"}); response.Error == nil || response.Error.Code != control.CodeUnknownOp {
		t.Fatalf("write operations are not available yet: %+v", response)
	}
}
