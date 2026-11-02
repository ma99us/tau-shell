package control

import (
	"encoding/json"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type echoHandler struct{}

func (echoHandler) Handle(_ Caller, request Request) Response {
	switch request.Op {
	case OpHello:
		return OK(Hello{Version: ProtocolVersion, PID: os.Getpid(), StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	case "panic":
		panic("boom")
	}
	return Failure(CodeUnknownOp, "unknown operation")
}

func TestPipeRoundTripAuditAndFirstInstance(t *testing.T) {
	audit := NewAuditLog(2, nil)
	server, err := Listen(echoHandler{}, audit)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if _, err := Listen(echoHandler{}, nil); err == nil {
		t.Fatal("a second server on the same pipe name must fail")
	}

	response, err := Call(server.Name(), Request{Op: OpHello}, 3*time.Second)
	if err != nil || !response.OK {
		t.Fatalf("hello: %+v %v", response, err)
	}
	var hello Hello
	if err := json.Unmarshal(response.Data, &hello); err != nil || hello.Version != ProtocolVersion {
		t.Fatalf("hello data: %+v %v", hello, err)
	}

	for _, op := range []string{"nope", "panic", "nope"} {
		response, err := Call(server.Name(), Request{Op: op}, 3*time.Second)
		if err != nil || response.OK || response.Error == nil {
			t.Fatalf("%s: %+v %v", op, response, err)
		}
	}

	entries := audit.Entries()
	if len(entries) != 2 || entries[0].Outcome != CodeInternal || entries[1].Outcome != CodeUnknownOp {
		t.Fatalf("audit must keep the newest 2 entries: %+v", entries)
	}
	if entries[1].PID == 0 {
		t.Fatalf("audit should record the caller pid: %+v", entries[1])
	}
}

func TestInstancesFindsServerAndNewestFirst(t *testing.T) {
	server, err := Listen(echoHandler{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	instances, err := Instances(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, instance := range instances {
		if instance.PipeName == server.Name() {
			found = true
		}
	}
	if !found {
		t.Fatalf("server not discovered: %+v", instances)
	}
}

func TestPipeDACLAllowsOnlyCurrentUser(t *testing.T) {
	server, err := Listen(echoHandler{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	// A second, idle instance has the same DACL; querying the listening handle
	// would block behind its pending ConnectNamedPipe.
	handle, err := server.createInstance(false)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("dacl: %v", err)
	}
	if dacl.AceCount != 1 {
		t.Fatalf("expected a single ACE, got %d", dacl.AceCount)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !sid.Equals(server.sid) {
		t.Fatalf("ACE is for %s, want %s", sid, server.sid)
	}
}

func TestMessageIsBounded(t *testing.T) {
	if err := writeLine(discard{}, map[string]string{"x": string(make([]byte, maxLineBytes))}); err == nil {
		t.Fatal("oversized message must be rejected")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
