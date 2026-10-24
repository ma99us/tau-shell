package conpty

import (
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	maxDimension = 32767
	exitWait     = 5 * time.Second
)

type Process struct {
	api       pseudoConsoleAPI
	console   windows.Handle
	process   windows.Handle
	processID uint32
	job       windows.Handle
	input     *os.File
	output    *os.File
}

func Start(width, height int, executable, directory string, args ...string) (*Process, error) {
	return start(inboxAPI, width, height, executable, directory, args...)
}

// StartWithLibrary hosts the process through a bundled conpty.dll at the given
// absolute path instead of the inbox kernel32 ConPTY.
func StartWithLibrary(library string, width, height int, executable, directory string, args ...string) (*Process, error) {
	api, err := loadBundledAPI(library)
	if err != nil {
		return nil, err
	}
	return start(api, width, height, executable, directory, args...)
}

func start(api pseudoConsoleAPI, width, height int, executable, directory string, args ...string) (*Process, error) {
	if width < 1 || height < 1 || width > maxDimension || height > maxDimension {
		return nil, fmt.Errorf("invalid terminal size %dx%d", width, height)
	}

	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	closePipes := func() {
		for _, handle := range []windows.Handle{inputRead, inputWrite, outputRead, outputWrite} {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
	}
	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		closePipes()
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}

	var pseudoConsole windows.Handle
	if err := api.create(windows.Coord{X: int16(width), Y: int16(height)}, inputRead, outputWrite, 0, &pseudoConsole); err != nil {
		closePipes()
		return nil, fmt.Errorf("create pseudoconsole: %w", err)
	}
	_ = windows.CloseHandle(inputRead)
	inputRead = 0
	_ = windows.CloseHandle(outputWrite)
	outputWrite = 0

	closeConsole := true
	defer func() {
		if closeConsole {
			api.close(pseudoConsole)
		}
	}()

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		closePipes()
		return nil, fmt.Errorf("create process attribute list: %w", err)
	}
	defer attributes.Delete()
	if err := setPseudoConsoleAttribute(attributes.List(), pseudoConsole); err != nil {
		closePipes()
		return nil, fmt.Errorf("set pseudoconsole process attribute: %w", err)
	}

	startup := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:    uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags: windows.STARTF_USESTDHANDLES,
		},
		ProcThreadAttributeList: attributes.List(),
	}

	commandLineUTF16, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{executable}, args...)))
	if err != nil {
		closePipes()
		return nil, fmt.Errorf("encode process command line: %w", err)
	}
	directoryUTF16, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		closePipes()
		return nil, fmt.Errorf("encode working directory: %w", err)
	}

	var processInfo windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(
		nil,
		commandLineUTF16,
		nil,
		nil,
		false,
		flags,
		nil,
		directoryUTF16,
		&startup.StartupInfo,
		&processInfo,
	); err != nil {
		closePipes()
		return nil, fmt.Errorf("start %s: %w", executable, err)
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		_ = windows.TerminateProcess(processInfo.Process, 1)
		_ = windows.CloseHandle(processInfo.Process)
		_ = windows.CloseHandle(processInfo.Thread)
		closePipes()
		return nil, fmt.Errorf("create session job object: %w", err)
	}
	jobInfo := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&jobInfo)),
		uint32(unsafe.Sizeof(jobInfo)),
	); err != nil {
		_ = windows.CloseHandle(job)
		_ = windows.TerminateProcess(processInfo.Process, 1)
		_ = windows.CloseHandle(processInfo.Process)
		_ = windows.CloseHandle(processInfo.Thread)
		closePipes()
		return nil, fmt.Errorf("configure session job object: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, processInfo.Process); err != nil {
		_ = windows.CloseHandle(job)
		_ = windows.TerminateProcess(processInfo.Process, 1)
		_ = windows.CloseHandle(processInfo.Process)
		_ = windows.CloseHandle(processInfo.Thread)
		closePipes()
		return nil, fmt.Errorf("assign PowerShell to session job: %w", err)
	}
	if _, err := windows.ResumeThread(processInfo.Thread); err != nil {
		_ = windows.CloseHandle(processInfo.Thread)
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		_ = windows.CloseHandle(processInfo.Process)
		closePipes()
		return nil, fmt.Errorf("resume PowerShell process: %w", err)
	}
	_ = windows.CloseHandle(processInfo.Thread)

	inputFile := os.NewFile(uintptr(inputWrite), "conpty-input")
	inputWrite = 0
	outputFile := os.NewFile(uintptr(outputRead), "conpty-output")
	outputRead = 0
	closePipes()

	process := &Process{
		api:       api,
		console:   pseudoConsole,
		process:   processInfo.Process,
		processID: processInfo.ProcessId,
		job:       job,
		input:     inputFile,
		output:    outputFile,
	}
	closeConsole = false
	return process, nil
}

func setPseudoConsoleAttribute(attributes *windows.ProcThreadAttributeList, console windows.Handle) error {
	update := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
	if err := update.Find(); err != nil {
		return fmt.Errorf("locate UpdateProcThreadAttribute: %w", err)
	}
	result, _, callErr := update.Call(
		uintptr(unsafe.Pointer(attributes)),
		0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		uintptr(console),
		unsafe.Sizeof(console),
		0,
		0,
	)
	if result == 0 {
		return fmt.Errorf("UpdateProcThreadAttribute failed: %v", callErr)
	}
	return nil
}

func (process *Process) Input() *os.File {
	return process.input
}

func (process *Process) Output() *os.File {
	return process.output
}

func (process *Process) PID() int {
	return int(process.processID)
}

// Backend reports which ConPTY implementation hosts the process.
func (process *Process) Backend() string {
	return process.api.name
}

func (process *Process) CloseConsole() {
	if process.console != 0 {
		process.api.close(process.console)
		process.console = 0
	}
}

func (process *Process) Resize(width, height int) error {
	if width < 1 || height < 1 || width > maxDimension || height > maxDimension {
		return fmt.Errorf("invalid terminal size %dx%d", width, height)
	}
	if err := process.api.resize(process.console, windows.Coord{X: int16(width), Y: int16(height)}); err != nil {
		return fmt.Errorf("resize pseudoconsole to %dx%d: %w", width, height, err)
	}
	return nil
}

func (process *Process) Wait(timeout time.Duration) (bool, uint32, error) {
	event, err := windows.WaitForSingleObject(process.process, uint32(timeout/time.Millisecond))
	if err != nil {
		return false, 0, fmt.Errorf("wait for PowerShell: %w", err)
	}
	switch event {
	case windows.WAIT_OBJECT_0:
		var exitCode uint32
		if err := windows.GetExitCodeProcess(process.process, &exitCode); err != nil {
			return false, 0, fmt.Errorf("get PowerShell exit code: %w", err)
		}
		return true, exitCode, nil
	case uint32(windows.WAIT_TIMEOUT):
		return false, 0, nil
	default:
		return false, 0, fmt.Errorf("unexpected process wait result: 0x%x", event)
	}
}

func (process *Process) Write(data []byte) error {
	for len(data) > 0 {
		n, err := process.input.Write(data)
		if err != nil {
			return fmt.Errorf("write ConPTY input: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("write ConPTY input: no progress")
		}
		data = data[n:]
	}
	return nil
}

func (process *Process) Interrupt() error {
	return process.Write([]byte{0x03})
}

func (process *Process) Terminate() error {
	if err := windows.TerminateJobObject(process.job, 1); err != nil {
		return fmt.Errorf("terminate session process tree: %w", err)
	}
	return nil
}

func (process *Process) Close() error {
	var closeErr error
	process.CloseConsole()
	if process.input != nil {
		if err := process.input.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close ConPTY input: %w", err))
		}
		process.input = nil
	}
	if process.output != nil {
		if err := process.output.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close ConPTY output: %w", err))
		}
		process.output = nil
	}
	if process.job != 0 {
		if err := windows.CloseHandle(process.job); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close session job object: %w", err))
		}
		process.job = 0
	}
	if process.process != 0 {
		if err := windows.CloseHandle(process.process); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close PowerShell process handle: %w", err))
		}
		process.process = 0
	}
	return closeErr
}
