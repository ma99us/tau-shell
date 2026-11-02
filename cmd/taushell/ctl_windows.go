package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ma99us/tau-shell/internal/agentstate"
	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/control"
)

// ctl exit codes; 0 is success and 1 is any other error.
const (
	exitCtlUsage    = 2
	exitCtlNotFound = 3
	exitCtlDisabled = 4
	exitCtlRefused  = 5
	exitCtlTimeout  = 6
)

const (
	ctlTimeout = 5 * time.Second
	ctlUsage   = "usage: taushell ctl list [--json] [--instance <pid>]\n       taushell ctl wait <session> [--for idle|input|working] [--timeout 10m] [--instance <pid>]\n       taushell ctl send [--insert-only] [--stdin] [--instance <pid>] <session> <text...>"
)

type ctlError struct {
	code int
	msg  string
}

func (e *ctlError) Error() string { return e.msg }

type ctlBackend struct {
	instances func() ([]control.Instance, error)
	call      func(pipe string, request control.Request) (control.Response, error)
	enabled   func() (bool, error)
	input     io.Reader
}

func (backend ctlBackend) stdin() io.Reader {
	if backend.input != nil {
		return backend.input
	}
	return os.Stdin
}

func defaultCtlBackend() ctlBackend {
	return ctlBackend{
		instances: func() ([]control.Instance, error) { return control.Instances(ctlTimeout) },
		call: func(pipe string, request control.Request) (control.Response, error) {
			// A wait request legitimately blocks up to MaxWait on the server.
			return control.Call(pipe, request, ctlTimeout+control.MaxWait)
		},
		enabled: func() (bool, error) {
			configDirectory, _, err := appPaths()
			if err != nil {
				return false, err
			}
			settings, err := appconfig.Load(filepath.Join(filepath.Dir(configDirectory), appconfig.FileName))
			return settings.Control.ChannelEnabled(), err
		},
	}
}

func runCtl(args []string, stdout io.Writer) error {
	return runCtlWith(args, stdout, defaultCtlBackend())
}

func runCtlWith(args []string, stdout io.Writer, backend ctlBackend) error {
	if len(args) == 0 {
		return &ctlError{exitCtlUsage, ctlUsage}
	}
	switch args[0] {
	case "list":
		return runCtlList(args[1:], stdout, backend)
	case "wait":
		return runCtlWait(args[1:], stdout, backend)
	case "send":
		return runCtlSend(args[1:], stdout, backend)
	}
	return &ctlError{exitCtlUsage, ctlUsage}
}

// ctlFailure maps a refused or missing-session response to its exit code.
func ctlFailure(action string, response control.Response) error {
	if response.Error != nil {
		switch response.Error.Code {
		case control.CodeRefused:
			return &ctlError{exitCtlRefused, action + ": " + response.Error.Message}
		case control.CodeNotFound:
			return &ctlError{exitCtlNotFound, action + ": " + response.Error.Message}
		}
	}
	return fmt.Errorf("%s: %w", action, response.Error)
}

func runCtlWait(args []string, stdout io.Writer, backend ctlBackend) error {
	flags := flag.NewFlagSet("ctl wait", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	state := flags.String("for", "idle", "idle, input or working")
	timeout := flags.Duration("timeout", 10*time.Minute, "give up after this long")
	instance := flags.Int("instance", 0, "target instance process ID")
	if err := flags.Parse(reorderFlags(args)); err != nil || flags.NArg() != 1 {
		return &ctlError{exitCtlUsage, ctlUsage}
	}
	target, err := pickInstance(backend, *instance)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(*timeout)
	for {
		slice := time.Until(deadline)
		if slice > control.MaxWait {
			slice = control.MaxWait
		}
		if slice <= 0 {
			return &ctlError{exitCtlTimeout, "timed out waiting for " + *state}
		}
		arguments, _ := json.Marshal(control.WaitArgs{Session: flags.Arg(0), For: *state, TimeoutMS: int(slice / time.Millisecond)})
		response, err := backend.call(target.PipeName, control.Request{Op: control.OpWait, Args: arguments})
		if err != nil {
			return fmt.Errorf("wait: %w", err)
		}
		if !response.OK {
			return ctlFailure("wait", response)
		}
		var result control.WaitResult
		if err := json.Unmarshal(response.Data, &result); err != nil {
			return fmt.Errorf("decode wait result: %w", err)
		}
		if result.Matched {
			_, err := fmt.Fprintln(stdout, result.Activity.State)
			return err
		}
	}
}

func runCtlSend(args []string, stdout io.Writer, backend ctlBackend) error {
	flags := flag.NewFlagSet("ctl send", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	insertOnly := flags.Bool("insert-only", false, "type the text but do not press Enter")
	fromStdin := flags.Bool("stdin", false, "read the text from standard input (keeps it off the command line)")
	instance := flags.Int("instance", 0, "target instance process ID")
	if err := flags.Parse(args); err != nil || flags.NArg() < 1 {
		return &ctlError{exitCtlUsage, ctlUsage}
	}
	var text string
	if *fromStdin {
		if flags.NArg() != 1 {
			return &ctlError{exitCtlUsage, ctlUsage}
		}
		raw, err := io.ReadAll(io.LimitReader(backend.stdin(), control.MaxSendBytes+1))
		if err != nil {
			return fmt.Errorf("read text: %w", err)
		}
		text = strings.TrimRight(string(raw), "\r\n")
	} else {
		text = strings.Join(flags.Args()[1:], " ")
	}
	if text == "" {
		return &ctlError{exitCtlUsage, "send needs text: pass it after the session id or use --stdin"}
	}
	target, err := pickInstance(backend, *instance)
	if err != nil {
		return err
	}
	arguments, _ := json.Marshal(control.SendArgs{Session: flags.Arg(0), Text: text, InsertOnly: *insertOnly})
	response, err := backend.call(target.PipeName, control.Request{Op: control.OpSend, Args: arguments})
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	if !response.OK {
		return ctlFailure("send", response)
	}
	// The window re-checks the live state before typing; delivery is not a confirmation the harness acted on it.
	_, err = fmt.Fprintln(stdout, "handed to the TauShell window")
	return err
}

// reorderFlags lets flags follow the positional session id, as in
// "ctl wait <id> --for idle".
func reorderFlags(args []string) []string {
	var flagArgs, positional []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && index+1 < len(args) {
				index++
				flagArgs = append(flagArgs, args[index])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flagArgs, positional...)
}

func runCtlList(args []string, stdout io.Writer, backend ctlBackend) error {
	flags := flag.NewFlagSet("ctl list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	instance := flags.Int("instance", 0, "target instance process ID")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return &ctlError{exitCtlUsage, ctlUsage}
	}

	target, err := pickInstance(backend, *instance)
	if err != nil {
		return err
	}
	response, err := backend.call(target.PipeName, control.Request{Op: control.OpList})
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	if !response.OK {
		return fmt.Errorf("list sessions: %w", response.Error)
	}
	if *asJSON {
		_, err := fmt.Fprintln(stdout, string(response.Data))
		return err
	}
	var sessions []control.SessionInfo
	if err := json.Unmarshal(response.Data, &sessions); err != nil {
		return fmt.Errorf("decode session list: %w", err)
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tTOOL\tSTATUS\tSTATE\tDIRECTORY")
	for _, item := range sessions {
		state := string(item.Activity.State)
		if item.Activity.State != agentstate.Stopped && item.Activity.State != agentstate.Unknown && !item.Activity.Confirmed {
			state += "?"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Tool, item.Status, state, item.WorkingDir)
	}
	return table.Flush()
}

func pickInstance(backend ctlBackend, wantPID int) (control.Instance, error) {
	instances, err := backend.instances()
	if err != nil {
		return control.Instance{}, err
	}
	if wantPID != 0 {
		for _, candidate := range instances {
			if candidate.Hello.PID == wantPID {
				return candidate, nil
			}
		}
		return control.Instance{}, &ctlError{exitCtlNotFound, "no TauShell instance with process ID " + strconv.Itoa(wantPID) + " answered"}
	}
	if len(instances) > 0 {
		return instances[0], nil
	}
	enabled, err := backend.enabled()
	if err != nil {
		return control.Instance{}, fmt.Errorf("read settings: %w", err)
	}
	if !enabled {
		return control.Instance{}, &ctlError{exitCtlDisabled, "the control channel is off; set control.enabled: true in ~/.tau_shell/config.yaml and restart TauShell"}
	}
	return control.Instance{}, &ctlError{exitCtlNotFound, "no running TauShell answered; start TauShell first"}
}
