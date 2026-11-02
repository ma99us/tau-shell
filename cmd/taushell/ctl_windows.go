package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/control"
)

// ctl exit codes; 0 is success and 1 is any other error.
const (
	exitCtlUsage    = 2
	exitCtlNotFound = 3
	exitCtlDisabled = 4
)

const (
	ctlTimeout = 5 * time.Second
	ctlUsage   = "usage: taushell ctl list [--json] [--instance <pid>]"
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
}

func defaultCtlBackend() ctlBackend {
	return ctlBackend{
		instances: func() ([]control.Instance, error) { return control.Instances(ctlTimeout) },
		call: func(pipe string, request control.Request) (control.Response, error) {
			return control.Call(pipe, request, ctlTimeout)
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
	if len(args) == 0 || args[0] != "list" {
		return &ctlError{exitCtlUsage, ctlUsage}
	}
	flags := flag.NewFlagSet("ctl list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	instance := flags.Int("instance", 0, "target instance process ID")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
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
	fmt.Fprintln(table, "ID\tNAME\tTOOL\tSTATUS\tDIRECTORY")
	for _, item := range sessions {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Tool, item.Status, item.WorkingDir)
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
