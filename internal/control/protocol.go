// Package control is the local control channel between the TauShell CLI and the
// running desktop app. Requests are single JSON lines over a per-user named pipe
// (see pipe_windows.go). Messages never carry prompt text or terminal content in
// this slice, and nothing here is written to logs beyond operation names.
package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ProtocolVersion is bumped for incompatible wire changes.
const ProtocolVersion = 1

// maxLineBytes bounds one request or response line.
const maxLineBytes = 1 << 20

// Operation names.
const (
	OpHello = "hello"
	OpList  = "list"
)

// Error codes returned in Response.Error.Code.
const (
	CodeDisabled           = "disabled"
	CodeBadRequest         = "bad_request"
	CodeUnknownOp          = "unknown_op"
	CodeInternal           = "internal"
	CodeUnsupportedVersion = "unsupported_version"
)

// Request is one command from a client.
type Request struct {
	Version int             `json:"v"`
	Op      string          `json:"op"`
	Args    json.RawMessage `json:"args,omitempty"`
}

// Response answers exactly one Request.
type Response struct {
	OK    bool            `json:"ok"`
	Error *Error          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Error is a machine-readable failure.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Hello identifies a running instance.
type Hello struct {
	Version   int    `json:"version"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	App       string `json:"app"`
}

// SessionInfo is the read-only view of a session returned by list. Activity
// (working, idle, waiting for input) is not reported yet: it is derived in the
// UI today, and a reliable shared state is planned work.
type SessionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Tool        string `json:"tool"`
	Status      string `json:"status"`
	RunningHere bool   `json:"runningHere"`
	WorkingDir  string `json:"workingDir"`
	LastError   string `json:"lastError,omitempty"`
}

// Caller describes the connected process. It is a hint for the audit trail,
// never an authorization decision: re-parenting and PID reuse can fool it.
type Caller struct {
	PID   uint32
	Image string
}

// Handler serves one request. It must not block indefinitely.
type Handler interface {
	Handle(caller Caller, request Request) Response
}

// OK builds a success response carrying data.
func OK(data any) Response {
	if data == nil {
		return Response{OK: true}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return Failure(CodeInternal, "encode response: "+err.Error())
	}
	return Response{OK: true, Data: raw}
}

// Failure builds an error response.
func Failure(code, message string) Response {
	return Response{Error: &Error{Code: code, Message: message}}
}

func writeLine(writer io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) >= maxLineBytes {
		return fmt.Errorf("message exceeds %d bytes", maxLineBytes)
	}
	_, err = writer.Write(append(raw, '\n'))
	return err
}

// readLine reads one bounded line into value.
func readLine(reader *bufio.Reader, value any) error {
	var line []byte
	for {
		chunk, isPrefix, err := reader.ReadLine()
		if err != nil {
			return err
		}
		line = append(line, chunk...)
		if len(line) > maxLineBytes {
			return errors.New("message too large")
		}
		if !isPrefix {
			break
		}
	}
	return json.Unmarshal(line, value)
}
