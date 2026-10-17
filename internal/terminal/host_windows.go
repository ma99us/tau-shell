package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/term"

	"github.com/ma99us/tau-shell/internal/conpty"
)

const (
	utf8CodePage  = 65001
	resizePeriod  = 200 * time.Millisecond
	interruptWait = 3 * time.Second
)

type ShutdownChoice int

const (
	ShutdownContinue ShutdownChoice = iota
	ShutdownConfirm
)

type Options struct {
	Executable string
	Directory  string
	Args       []string
	OnStart    func(processID int) error
	OnFinish   func(status string, detail string) error
}

type inputResult struct {
	data []byte
	err  error
}

type consoleCodePages struct {
	getInput  func() (uint32, error)
	setInput  func(uint32) error
	getOutput func() (uint32, error)
	setOutput func(uint32) error
}

// WithUTF8Console sets UTF-8 code pages for attached console streams and restores them afterward.
func WithUTF8Console(action func() error) error {
	inputTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	outputTerminal := term.IsTerminal(int(os.Stdout.Fd())) || term.IsTerminal(int(os.Stderr.Fd()))
	return withConsoleCodePages(inputTerminal, outputTerminal, consoleCodePages{
		getInput:  windows.GetConsoleCP,
		setInput:  windows.SetConsoleCP,
		getOutput: windows.GetConsoleOutputCP,
		setOutput: windows.SetConsoleOutputCP,
	}, action)
}

func withConsoleCodePages(inputTerminal, outputTerminal bool, codePages consoleCodePages, action func() error) (returnErr error) {
	if !inputTerminal && !outputTerminal {
		return action()
	}

	oldInputCodePage, err := codePages.getInput()
	if err != nil {
		return fmt.Errorf("read console input code page: %w", err)
	}
	oldOutputCodePage, err := codePages.getOutput()
	if err != nil {
		return fmt.Errorf("read console output code page: %w", err)
	}
	if inputTerminal {
		if err := codePages.setInput(utf8CodePage); err != nil {
			return fmt.Errorf("set console input code page to UTF-8: %w", err)
		}
	}
	if outputTerminal {
		if err := codePages.setOutput(utf8CodePage); err != nil {
			if inputTerminal {
				if restoreErr := codePages.setInput(oldInputCodePage); restoreErr != nil {
					return errors.Join(
						fmt.Errorf("set console output code page to UTF-8: %w", err),
						fmt.Errorf("restore console input code page: %w", restoreErr),
					)
				}
			}
			return fmt.Errorf("set console output code page to UTF-8: %w", err)
		}
	}
	defer func() {
		if outputTerminal {
			if err := codePages.setOutput(oldOutputCodePage); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("restore console output code page: %w", err))
			}
		}
		if inputTerminal {
			if err := codePages.setInput(oldInputCodePage); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("restore console input code page: %w", err))
			}
		}
	}()
	return action()
}

func Run(options Options) error {
	return WithUTF8Console(func() error {
		return runInConsole(options)
	})
}

func runInConsole(options Options) (returnErr error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("interactive terminal required; run TauShell from a Windows console")
	}

	outputHandle := windows.Handle(os.Stdout.Fd())
	var oldOutputMode uint32
	if err := windows.GetConsoleMode(outputHandle, &oldOutputMode); err != nil {
		return fmt.Errorf("read console output mode: %w", err)
	}
	if err := windows.SetConsoleMode(outputHandle, virtualTerminalOutputMode(oldOutputMode)); err != nil {
		return fmt.Errorf("enable virtual-terminal output: %w", err)
	}
	defer func() {
		if err := windows.SetConsoleMode(outputHandle, oldOutputMode); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("restore console output mode: %w", err))
		}
	}()

	oldMode, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("enable raw terminal input: %w", err)
	}
	defer func() {
		if err := term.Restore(int(os.Stdin.Fd()), oldMode); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("restore terminal input mode: %w", err))
		}
	}()

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return fmt.Errorf("read terminal dimensions: %w", err)
	}
	process, err := conpty.Start(width, height, options.Executable, options.Directory, options.Args...)
	if err != nil {
		return err
	}
	defer func() {
		if err := process.Close(); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()
	if options.OnStart != nil {
		if err := options.OnStart(process.PID()); err != nil {
			_ = process.Terminate()
			return fmt.Errorf("persist launched session: %w", err)
		}
	}
	shutdownForced := false
	if options.OnFinish != nil {
		defer func() {
			if err := process.Close(); err != nil {
				returnErr = errors.Join(returnErr, err)
			}
			status := "stopped"
			detail := ""
			if returnErr != nil {
				status = "failed"
				detail = returnErr.Error()
			} else if shutdownForced {
				detail = "Graceful interrupt timed out; session process tree was force-terminated."
			}
			if err := options.OnFinish(status, detail); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("persist session outcome: %w", err))
			}
		}()
	}

	fmt.Fprintln(os.Stderr, "\r\nTauShell session active. Ctrl+C is sent to PowerShell. Ctrl+Q requests shutdown.")
	outputDone := make(chan struct{})
	outputErrors := make(chan error, 1)
	go func() {
		_, err := io.Copy(os.Stdout, process.Output())
		outputErrors <- err
		close(outputDone)
	}()
	inputReady := make(chan inputResult, 1)
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buffer)
			if n > 0 {
				data := append([]byte(nil), buffer[:n]...)
				select {
				case inputReady <- inputResult{data: data}:
				case <-outputDone:
					return
				}
			}
			if err != nil {
				inputReady <- inputResult{err: err}
				return
			}
		}
	}()

	resizeTicker := time.NewTicker(resizePeriod)
	defer resizeTicker.Stop()
	lastWidth, lastHeight := width, height
	for {
		exited, exitCode, err := process.Wait(0)
		if err != nil {
			return err
		}
		if exited {
			process.CloseConsole()
			<-outputDone
			if err := <-outputErrors; err != nil && !errors.Is(err, os.ErrClosed) {
				return fmt.Errorf("relay ConPTY output: %w", err)
			}
			if exitCode != 0 {
				return fmt.Errorf("PowerShell exited with code %d", exitCode)
			}
			return nil
		}

		select {
		case input := <-inputReady:
			if input.err != nil {
				forced, err := gracefulShutdown(process, interruptWait)
				shutdownForced = forced
				if err != nil {
					return err
				}
				if !errors.Is(input.err, io.EOF) {
					return fmt.Errorf("read terminal input: %w", input.err)
				}
				return nil
			}
			shutdown, err := forwardInput(input.data, process.Write, func() (bool, error) {
				choice, err := confirmShutdown(inputReady)
				if err != nil {
					return false, err
				}
				if choice != ShutdownConfirm {
					return false, nil
				}
				shutdownForced, err = gracefulShutdown(process, interruptWait)
				return true, err
			})
			if err != nil {
				return err
			}
			if shutdown {
				return nil
			}
		case <-resizeTicker.C:
			width, height, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				return fmt.Errorf("read terminal dimensions: %w", err)
			}
			if width != lastWidth || height != lastHeight {
				if err := process.Resize(width, height); err != nil {
					return err
				}
				lastWidth, lastHeight = width, height
			}
		case <-outputDone:
			if err := <-outputErrors; err != nil && !errors.Is(err, os.ErrClosed) {
				return fmt.Errorf("relay ConPTY output: %w", err)
			}
			return fmt.Errorf("ConPTY output closed while the session process was still running")
		}
	}
}

func virtualTerminalOutputMode(mode uint32) uint32 {
	return mode | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
}

func forwardInput(data []byte, write func([]byte) error, requestShutdown func() (bool, error)) (bool, error) {
	start := 0
	for index, key := range data {
		if key != 0x11 {
			continue
		}
		if index > start {
			if err := write(data[start:index]); err != nil {
				return false, err
			}
		}
		stop, err := requestShutdown()
		if err != nil {
			return false, err
		}
		if stop {
			return true, nil
		}
		start = index + 1
	}
	if start < len(data) {
		if err := write(data[start:]); err != nil {
			return false, err
		}
	}
	return false, nil
}

func confirmShutdown(inputReady <-chan inputResult) (ShutdownChoice, error) {
	fmt.Fprint(os.Stderr, "\r\nA session is running. Exit and interrupt it? [y/N] ")
	for {
		input := <-inputReady
		if input.err != nil {
			return ShutdownContinue, fmt.Errorf("read shutdown confirmation: %w", input.err)
		}
		for _, key := range input.data {
			switch key {
			case 'y', 'Y':
				fmt.Fprintln(os.Stderr, "yes")
				return ShutdownConfirm, nil
			case 'n', 'N', 0x1b:
				fmt.Fprintln(os.Stderr, "no")
				return ShutdownContinue, nil
			}
		}
	}
}

func gracefulShutdown(process *conpty.Process, timeout time.Duration) (bool, error) {
	if err := process.Interrupt(); err != nil {
		return false, err
	}
	exited, _, err := process.Wait(timeout)
	if err != nil {
		return false, err
	}
	if exited {
		return false, nil
	}
	fmt.Fprintln(os.Stderr, "\r\nGraceful interrupt timed out; force-terminating the session and child processes.")
	if err := process.Terminate(); err != nil {
		return false, err
	}
	exited, _, err = process.Wait(5 * time.Second)
	if err != nil {
		return false, err
	}
	if !exited {
		return false, fmt.Errorf("session process did not exit after force termination")
	}
	return true, nil
}
