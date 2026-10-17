package terminal

import (
	"bytes"
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestVirtualTerminalOutputModePreservesExistingFlags(t *testing.T) {
	initial := uint32(windows.ENABLE_WRAP_AT_EOL_OUTPUT)
	want := initial | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	want |= windows.ENABLE_PROCESSED_OUTPUT
	if got := virtualTerminalOutputMode(initial); got != want {
		t.Fatalf("virtual terminal output mode = 0x%x, want 0x%x", got, want)
	}
	if got := virtualTerminalOutputMode(want); got != want {
		t.Fatalf("enabling virtual terminal output twice changed mode to 0x%x", got)
	}
}

func TestWithConsoleCodePagesRunsActionWithoutTerminals(t *testing.T) {
	wantErr := errors.New("action failed")
	called := false
	err := withConsoleCodePages(false, false, consoleCodePages{}, func() error {
		called = true
		return wantErr
	})
	if !called {
		t.Fatal("action was not run")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("withConsoleCodePages error = %v, want %v", err, wantErr)
	}
}

func TestWithConsoleCodePagesRestoresOriginalValues(t *testing.T) {
	inputCodePage, outputCodePage := uint32(437), uint32(850)
	codePages := consoleCodePages{
		getInput: func() (uint32, error) { return inputCodePage, nil },
		setInput: func(value uint32) error {
			inputCodePage = value
			return nil
		},
		getOutput: func() (uint32, error) { return outputCodePage, nil },
		setOutput: func(value uint32) error {
			outputCodePage = value
			return nil
		},
	}
	wantErr := errors.New("action failed")
	err := withConsoleCodePages(true, true, codePages, func() error {
		if inputCodePage != utf8CodePage || outputCodePage != utf8CodePage {
			t.Fatalf("code pages during action = (%d, %d), want UTF-8", inputCodePage, outputCodePage)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("withConsoleCodePages error = %v, want %v", err, wantErr)
	}
	if inputCodePage != 437 || outputCodePage != 850 {
		t.Fatalf("restored code pages = (%d, %d), want (437, 850)", inputCodePage, outputCodePage)
	}
}

func TestForwardInputPreservesUTF8BytesAsOneWrite(t *testing.T) {
	want := []byte("line-λ-🙂")
	var writes [][]byte
	shutdownCalls := 0
	stop, err := forwardInput(want, func(data []byte) error {
		writes = append(writes, append([]byte(nil), data...))
		return nil
	}, func() (bool, error) {
		shutdownCalls++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stop {
		t.Fatal("unexpected shutdown request")
	}
	if shutdownCalls != 0 {
		t.Fatalf("shutdown handler called %d times, want 0", shutdownCalls)
	}
	if len(writes) != 1 || !bytes.Equal(writes[0], want) {
		t.Fatalf("forwarded writes = %q, want one unmodified UTF-8 write %q", writes, want)
	}
}

func TestForwardInputKeepsChunksAroundShutdownControl(t *testing.T) {
	var forwarded bytes.Buffer
	shutdownCalls := 0
	stop, err := forwardInput([]byte("before\x11after"), func(data []byte) error {
		_, err := forwarded.Write(data)
		return err
	}, func() (bool, error) {
		shutdownCalls++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stop {
		t.Fatal("declined shutdown unexpectedly stopped input forwarding")
	}
	if shutdownCalls != 1 {
		t.Fatalf("shutdown handler called %d times, want 1", shutdownCalls)
	}
	if got, want := forwarded.String(), "beforeafter"; got != want {
		t.Fatalf("forwarded input = %q, want %q", got, want)
	}
}

func TestForwardInputStopsAfterConfirmedShutdown(t *testing.T) {
	var forwarded bytes.Buffer
	stop, err := forwardInput([]byte("before\x11after"), func(data []byte) error {
		_, err := forwarded.Write(data)
		return err
	}, func() (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stop {
		t.Fatal("confirmed shutdown did not stop input forwarding")
	}
	if got, want := forwarded.String(), "before"; got != want {
		t.Fatalf("forwarded input = %q, want %q", got, want)
	}
}

func TestForwardInputReturnsWriteFailure(t *testing.T) {
	wantErr := errors.New("write failed")
	_, err := forwardInput([]byte("input"), func([]byte) error {
		return wantErr
	}, func() (bool, error) {
		t.Fatal("shutdown handler must not run")
		return false, nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("forwardInput error = %v, want %v", err, wantErr)
	}
}
