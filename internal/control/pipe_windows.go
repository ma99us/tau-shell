package control

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pipePrefix = `\\.\pipe\`
	pipeStem   = "taushell-ctl-"
	// connectionTimeout bounds one request/response exchange so a stalled
	// client cannot hold a pipe instance.
	connectionTimeout = 10 * time.Second
)

// userTag is a short, stable tag of the current user's SID. It keeps other
// users' instances out of enumeration; the DACL is what actually denies them.
func userTag() (tag string, sid *windows.SID, err error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", nil, fmt.Errorf("read current user: %w", err)
	}
	sum := sha256.Sum256([]byte(user.User.Sid.String()))
	return hex.EncodeToString(sum[:4]), user.User.Sid, nil
}

// PipeName is the pipe address of the instance with the given process ID.
func PipeName(pid int) (string, error) {
	tag, _, err := userTag()
	if err != nil {
		return "", err
	}
	return pipePrefix + pipeStem + tag + "-" + strconv.Itoa(pid), nil
}

// Server accepts control connections on a current-user-only named pipe.
type Server struct {
	name    string
	sid     *windows.SID
	handler Handler
	audit   *AuditLog

	mu      sync.Mutex
	closed  bool
	current windows.Handle
	done    chan struct{}
}

// Listen creates the pipe and starts serving. It fails if another process
// already owns the pipe name: a pre-created pipe must never be served from.
func Listen(handler Handler, audit *AuditLog) (*Server, error) {
	name, err := PipeName(os.Getpid())
	if err != nil {
		return nil, err
	}
	_, sid, err := userTag()
	if err != nil {
		return nil, err
	}
	server := &Server{name: name, sid: sid, handler: handler, audit: audit, done: make(chan struct{})}
	handle, err := server.createInstance(true)
	if err != nil {
		return nil, fmt.Errorf("create control pipe: %w", err)
	}
	server.current = handle
	go server.acceptLoop()
	return server, nil
}

// Name returns the pipe address.
func (server *Server) Name() string { return server.name }

func (server *Server) createInstance(first bool) (windows.Handle, error) {
	// Protected DACL: a single ACE granting access to the current user only.
	sddl := "D:P(A;;GA;;;" + server.sid.String() + ")"
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return 0, err
	}
	attributes := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	name, err := windows.UTF16PtrFromString(server.name)
	if err != nil {
		return 0, err
	}
	open := uint32(windows.PIPE_ACCESS_DUPLEX)
	if first {
		open |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	return windows.CreateNamedPipe(name, open,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		windows.PIPE_UNLIMITED_INSTANCES, 64<<10, 64<<10, 0, attributes)
}

func (server *Server) acceptLoop() {
	defer close(server.done)
	for {
		server.mu.Lock()
		handle, closed := server.current, server.closed
		server.mu.Unlock()
		if closed {
			return
		}
		err := windows.ConnectNamedPipe(handle, nil)
		if err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			// Close cancels the pending connect; any other failure also ends serving.
			_ = windows.CloseHandle(handle)
			return
		}
		next, createErr := server.createInstance(false)
		server.mu.Lock()
		if server.closed || createErr != nil {
			server.closed = true
			server.mu.Unlock()
			go server.serve(handle)
			if next != 0 && createErr == nil {
				_ = windows.CloseHandle(next)
			}
			return
		}
		server.current = next
		server.mu.Unlock()
		go server.serve(handle)
	}
}

// Close stops accepting and releases the pipe. In-flight exchanges finish or time out.
func (server *Server) Close() {
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		<-server.done
		return
	}
	server.closed = true
	handle := server.current
	server.mu.Unlock()
	// Unblocks the pending ConnectNamedPipe.
	_ = windows.CancelIoEx(handle, nil)
	<-server.done
}

func (server *Server) serve(handle windows.Handle) {
	file := os.NewFile(uintptr(handle), "control-pipe")
	timer := time.AfterFunc(connectionTimeout, func() { _ = file.Close() })
	defer func() {
		timer.Stop()
		_ = windows.FlushFileBuffers(handle)
		_ = windows.DisconnectNamedPipe(handle)
		_ = file.Close()
	}()

	var clientPID uint32
	_ = windows.GetNamedPipeClientProcessId(handle, &clientPID)
	caller := Caller{PID: clientPID, Image: imageName(clientPID)}

	var request Request
	if err := readLine(bufio.NewReader(file), &request); err != nil {
		server.record(caller, request, Failure(CodeBadRequest, "unreadable request"))
		_ = writeLine(file, Failure(CodeBadRequest, "unreadable request"))
		return
	}
	response := server.dispatch(caller, request)
	server.record(caller, request, response)
	_ = writeLine(file, response)
}

func (server *Server) dispatch(caller Caller, request Request) (response Response) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = Failure(CodeInternal, "internal error")
		}
	}()
	if request.Version != ProtocolVersion {
		return Failure(CodeUnsupportedVersion, fmt.Sprintf("protocol version %d is not supported (server speaks %d)", request.Version, ProtocolVersion))
	}
	return server.handler.Handle(caller, request)
}

func (server *Server) record(caller Caller, request Request, response Response) {
	if server.audit == nil {
		return
	}
	outcome := "ok"
	if response.Error != nil {
		outcome = response.Error.Code
	}
	op := request.Op
	if len(op) > 32 {
		op = op[:32]
	}
	server.audit.Add(AuditEntry{Time: time.Now(), PID: caller.PID, Image: caller.Image, Op: op, Outcome: outcome})
}

// imageName returns only the executable's base name, a hint for humans.
func imageName(pid uint32) string {
	if pid == 0 {
		return ""
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buffer[:size]))
}

// Instance is a running TauShell that answered hello.
type Instance struct {
	PipeName string
	Hello    Hello
}

// ErrNoInstance means no TauShell of this user answered.
var ErrNoInstance = errors.New("no running TauShell instance answered")

// Call sends one request to the pipe and returns its response.
func Call(pipeName string, request Request, timeout time.Duration) (Response, error) {
	request.Version = ProtocolVersion
	deadline := time.Now().Add(timeout)
	name, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return Response{}, err
	}
	var handle windows.Handle
	for {
		// Anonymous impersonation: the server learns nothing about the client token.
		handle, err = windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
			windows.OPEN_EXISTING, windows.SECURITY_SQOS_PRESENT|windows.SECURITY_ANONYMOUS, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) || time.Now().After(deadline) {
			return Response{}, err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if want := pidFromPipeName(pipeName); want != 0 {
		var serverPID uint32
		if err := windows.GetNamedPipeServerProcessId(handle, &serverPID); err != nil || int(serverPID) != want {
			_ = windows.CloseHandle(handle)
			return Response{}, fmt.Errorf("pipe %s is not served by process %d", filepath.Base(pipeName), want)
		}
	}
	file := os.NewFile(uintptr(handle), "control-client")
	timer := time.AfterFunc(time.Until(deadline), func() { _ = file.Close() })
	defer func() {
		timer.Stop()
		_ = file.Close()
	}()
	if err := writeLine(file, request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := readLine(bufio.NewReader(file), &response); err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	return response, nil
}

func pidFromPipeName(name string) int {
	index := strings.LastIndex(name, "-")
	if index < 0 {
		return 0
	}
	pid, err := strconv.Atoi(name[index+1:])
	if err != nil {
		return 0
	}
	return pid
}

// Instances lists this user's running instances, newest first. Pipes that
// belong to other users or do not answer are skipped.
func Instances(timeout time.Duration) ([]Instance, error) {
	tag, _, err := userTag()
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(pipePrefix + pipeStem + tag + "-*")
	if err != nil {
		return nil, fmt.Errorf("enumerate control pipes: %w", err)
	}
	var instances []Instance
	for _, match := range matches {
		response, err := Call(match, Request{Op: OpHello}, timeout)
		if err != nil || !response.OK {
			continue
		}
		var hello Hello
		if err := decode(response, &hello); err != nil || hello.PID != pidFromPipeName(match) {
			continue
		}
		instances = append(instances, Instance{PipeName: match, Hello: hello})
	}
	// Newest first; RFC 3339 timestamps sort lexically.
	for i := 1; i < len(instances); i++ {
		for j := i; j > 0 && instances[j].Hello.StartedAt > instances[j-1].Hello.StartedAt; j-- {
			instances[j], instances[j-1] = instances[j-1], instances[j]
		}
	}
	return instances, nil
}
