//go:build windows

package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const codexTimeout = 15 * time.Second

// CodexAppServerRunner starts a hidden local Codex app-server and asks only
// account/rateLimits/read. Authentication, when available, stays entirely in
// Codex's own profile; no token or profile path is read by TauShell.
func CodexAppServerRunner(ctx context.Context) ([]byte, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, errors.New("Codex CLI not found")
	}
	ctx, cancel := context.WithTimeout(ctx, codexTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, "app-server", "--stdio")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = command.Wait()
	}()

	write := func(id *int, method string, params any) error {
		request := map[string]any{"method": method, "params": params}
		if id != nil {
			request["id"] = *id
		}
		data, err := json.Marshal(request)
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(data, '\n'))
		return err
	}
	initializeID, readID := 1, 2
	if err := write(&initializeID, "initialize", map[string]any{"clientInfo": map[string]any{"name": "tau-shell", "title": "TauShell", "version": "1"}, "capabilities": map[string]any{}}); err != nil {
		return nil, err
	}

	lines := make(chan []byte, 8)
	errs := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			select {
			case lines <- append([]byte(nil), scanner.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
		errs <- scanner.Err()
	}()
	initialized := false
	for {
		select {
		case line := <-lines:
			var response struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(line, &response) != nil {
				continue
			}
			if len(response.Error) > 0 && string(response.Error) != "null" {
				return nil, errors.New("Codex app-server rejected rate-limit read")
			}
			switch response.ID {
			case 1:
				if initialized {
					continue
				}
				initialized = true
				if err := write(nil, "initialized", map[string]any{}); err != nil {
					return nil, err
				}
				if err := write(&readID, "account/rateLimits/read", map[string]any{}); err != nil {
					return nil, err
				}
			case 2:
				if len(response.Result) == 0 || string(response.Result) == "null" {
					return nil, errors.New("Codex app-server returned no rate limits")
				}
				return response.Result, nil
			}
		case err := <-errs:
			if err != nil {
				return nil, fmt.Errorf("read Codex app-server: %w", err)
			}
			return nil, errors.New("Codex app-server closed before rate-limit response")
		case <-ctx.Done():
			return nil, errors.New("Codex app-server rate-limit read timed out")
		}
	}
}
