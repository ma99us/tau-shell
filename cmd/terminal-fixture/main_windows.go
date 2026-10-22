//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

const (
	enableLineInput            = 0x0002
	enableEchoInput            = 0x0004
	enableMouseInput           = 0x0010
	enableVirtualTerminalInput = 0x0200
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("TauShell terminal fixture 1.0")
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "terminal fixture:", err)
		os.Exit(1)
	}
}

func run() error {
	inputHandle := windows.Handle(os.Stdin.Fd())
	outputHandle := windows.Handle(os.Stdout.Fd())
	var oldInputMode uint32
	if err := windows.GetConsoleMode(inputHandle, &oldInputMode); err != nil {
		return fmt.Errorf("read console input mode: %w", err)
	}
	inputMode := (oldInputMode &^ (enableLineInput | enableEchoInput)) | windows.ENABLE_PROCESSED_INPUT | enableMouseInput | enableVirtualTerminalInput
	if err := windows.SetConsoleMode(inputHandle, inputMode); err != nil {
		return fmt.Errorf("enable virtual-terminal input: %w", err)
	}
	defer windows.SetConsoleMode(inputHandle, oldInputMode)

	log, err := openEventLog()
	if err != nil {
		return err
	}
	defer log.Close()
	if err := writeEvent(log, "started"); err != nil {
		return err
	}

	if _, err := os.Stdout.Write([]byte("Primary-screen marker: PRESERVE-ME\r\n")); err != nil {
		return fmt.Errorf("write primary-screen marker: %w", err)
	}
	if _, err := os.Stdout.Write([]byte(
		"\x1b[?1049h\x1b[?1000h\x1b[?1006h\x1b[2J\x1b[H" +
			"\x1b[1;36mTauShell WebView2 + ConPTY terminal fixture\x1b[0m\r\n" +
			"Unicode: Ελληνικά | 日本語 | é | emoji: 🙂 🌍 🧪\r\n",
	)); err != nil {
		return fmt.Errorf("write fixture screen: %w", err)
	}
	if _, err := os.Stdout.Write([]byte(
		"ANSI colors: \x1b[31mred\x1b[0m \x1b[32mgreen\x1b[0m \x1b[34mblue\x1b[0m\r\n" +
			"\x1b[4;1HCursor movement and erase: stale text [erase me]\x1b[4;28H\x1b[0K" +
			"\x1b[5;1HTerminal query: sending device-status and cursor-position requests...\r\n" +
			"\x1b[6;1HResize: waiting for terminal dimensions...\r\n" +
			"\x1b[7;1HType tau-proof; click; m re-enables mouse; c/d re-query; q exits.\r\n" +
			"\x1b[8;1HInput: \r\n\x1b[9;1HMouse: \r\n" +
			"\x1b[5n\x1b[6n\x1b[10;1HSplit UTF-8: ",
	)); err != nil {
		return fmt.Errorf("write fixture screen details: %w", err)
	}
	if _, err := os.Stdout.Write([]byte{0xf0, 0x9f}); err != nil {
		return fmt.Errorf("write first split UTF-8 bytes: %w", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stdout.Write([]byte{0x8c, 0x8d, ' ', '[', 'e', 'n', 'd', ']'}); err != nil {
		return fmt.Errorf("write final split UTF-8 bytes: %w", err)
	}
	lastSize := consoleSize(outputHandle)
	if err := reportSize(outputHandle, log); err != nil {
		return err
	}

	input := make(chan []byte, 16)
	readErr := make(chan error, 1)
	go func() {
		buffer := make([]byte, 256)
		for {
			count, err := os.Stdin.Read(buffer)
			if count > 0 {
				if logErr := writeEvent(log, fmt.Sprintf("terminal-input-chunk-bytes=%d", count)); logErr != nil {
					readErr <- logErr
					return
				}
				input <- append([]byte(nil), buffer[:count]...)
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	var pending, line bytes.Buffer
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case chunk := <-input:
			pending.Write(chunk)
			done, err := processInput(&pending, &line, outputHandle, log)
			if err != nil {
				return err
			}
			if done {
				return writeEvent(log, "exit")
			}
		case err := <-readErr:
			if errors.Is(err, os.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read terminal input: %w", err)
		case <-ticker.C:
			size := consoleSize(outputHandle)
			if size != "" && size != lastSize {
				lastSize = size
				if err := reportSize(outputHandle, log); err != nil {
					return err
				}
			}
		}
	}
}

func processInput(pending *bytes.Buffer, line *bytes.Buffer, output windows.Handle, log *os.File) (bool, error) {
	data := pending.Bytes()
	for len(data) > 0 {
		if data[0] == 0x1b {
			if len(data) == 1 {
				break
			}
			if data[1] != '[' {
				if len(data) < 2 {
					break
				}
				data = data[2:]
				continue
			}
			if bytes.HasPrefix(data, []byte("\x1b[M")) {
				if len(data) < 6 {
					break
				}
				report := fmt.Sprintf("%02x-%02x-%02x", data[3], data[4], data[5])
				if err := writeEvent(log, "mouse-x10="+report); err != nil {
					return false, err
				}
				if _, err := fmt.Fprintf(os.Stdout, "\x1b[9;1H\x1b[2KMouse: X10 %s", report); err != nil {
					return false, err
				}
				data = data[6:]
				continue
			}
			end := -1
			for index := 2; index < len(data); index++ {
				if data[index] >= 0x40 && data[index] <= 0x7e {
					end = index
					break
				}
			}
			if end < 0 {
				break
			}
			sequence := string(data[:end+1])
			if err := writeEvent(log, "terminal-control-input="+printable(sequence)); err != nil {
				return false, err
			}
			switch {
			case strings.HasSuffix(sequence, "R") && strings.Contains(sequence, ";"):
				if err := writeEvent(log, "cursor-position-report="+printable(sequence)); err != nil {
					return false, err
				}
			case strings.HasSuffix(sequence, "n"):
				if err := writeEvent(log, "device-status-report="+printable(sequence)); err != nil {
					return false, err
				}
			case strings.HasPrefix(sequence, "\x1b[<") && (strings.HasSuffix(sequence, "M") || strings.HasSuffix(sequence, "m")):
				if err := writeEvent(log, "mouse="+printable(sequence)); err != nil {
					return false, err
				}
				if _, err := fmt.Fprintf(os.Stdout, "\x1b[9;1H\x1b[2KMouse: %s", printable(sequence)); err != nil {
					return false, err
				}
			}
			data = data[end+1:]
			continue
		}

		if data[0] == '\r' || data[0] == '\n' {
			if line.Len() > 0 {
				value := line.String()
				if err := writeEvent(log, "line="+value); err != nil {
					return false, err
				}
				if _, err := fmt.Fprintf(os.Stdout, "\x1b[8;1H\x1b[2KInput: %s", value); err != nil {
					return false, err
				}
				line.Reset()
			}
			data = data[1:]
			continue
		}
		if data[0] == 'q' && line.Len() == 0 {
			data = data[1:]
			pending.Reset()
			_, err := os.Stdout.Write([]byte("\x1b[?1000l\x1b[?1006l\x1b[?1049l\r\nAlternate screen restored: PRESERVE-ME\r\n"))
			return true, err
		}
		if data[0] == 'd' && line.Len() == 0 {
			if err := writeEvent(log, "device-status-request=sent"); err != nil {
				return false, err
			}
			if _, err := os.Stdout.Write([]byte("\x1b[5n")); err != nil {
				return false, err
			}
			data = data[1:]
			continue
		}
		if data[0] == 'm' && line.Len() == 0 {
			if err := writeEvent(log, "mouse-tracking-request=sent"); err != nil {
				return false, err
			}
			if _, err := os.Stdout.Write([]byte("\x1b[?1000h\x1b[?1006h")); err != nil {
				return false, err
			}
			data = data[1:]
			continue
		}
		if data[0] == 'c' && line.Len() == 0 {
			if err := writeEvent(log, "cursor-position-request=sent"); err != nil {
				return false, err
			}
			if _, err := os.Stdout.Write([]byte("\x1b[6n")); err != nil {
				return false, err
			}
			data = data[1:]
			continue
		}
		if data[0] == 0x7f || data[0] == 0x08 {
			current := line.Bytes()
			if len(current) > 0 {
				_, _ = line.Write(current[:len(current)-1])
			}
			data = data[1:]
			continue
		}
		_, size := utf8.DecodeRune(data)
		if size == 0 {
			break
		}
		if size == 1 && data[0] >= utf8.RuneSelf {
			break
		}
		_, _ = line.Write(data[:size])
		data = data[size:]
	}
	pending.Reset()
	_, _ = pending.Write(data)
	return false, nil
}

func reportSize(output windows.Handle, log *os.File) error {
	size := consoleSize(output)
	if size == "" {
		return nil
	}
	if err := writeEvent(log, "resize="+size); err != nil {
		return err
	}
	_, err := fmt.Fprintf(os.Stdout, "\x1b[6;1H\x1b[2KResize: %s", size)
	return err
}

func consoleSize(output windows.Handle) string {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(output, &info); err != nil {
		return ""
	}
	columns := int(info.Window.Right-info.Window.Left) + 1
	rows := int(info.Window.Bottom-info.Window.Top) + 1
	return fmt.Sprintf("%dx%d", columns, rows)
}

func openEventLog() (*os.File, error) {
	path := os.Getenv("TAUSHELL_FIXTURE_LOG")
	if path == "" {
		return nil, errors.New("TAUSHELL_FIXTURE_LOG must point to an event log")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	return file, nil
}

func writeEvent(file *os.File, value string) error {
	if _, err := fmt.Fprintln(file, value); err != nil {
		return fmt.Errorf("write event log: %w", err)
	}
	return file.Sync()
}

func printable(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\x1b", "ESC"), "\r", "M")
}
