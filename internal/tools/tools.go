// Package tools describes TauShell's host prerequisites and supported
// harnesses, detects installed versions and adapter capabilities, and drives
// user-confirmed installation.
package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ID string

const (
	PowerShell ID = "powershell"
	ClaudeCode ID = "claude-code"
	CopilotCLI ID = "copilot-cli"
)

// Capability is a command-line option an adapter relies on. It is probed by
// looking for the option in the tool's help output.
type Capability struct {
	Name   string `json:"name"`
	Option string `json:"option"`
	// Optional capabilities only gate a UI feature; a tool without them is
	// still usable.
	Optional bool `json:"optional,omitempty"`
}

// Command is a harness slash command offered as a quick action. TauShell only
// types it into the terminal; it cannot confirm the harness applied it.
type Command struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// Widgets declares the quality-of-life actions an adapter supports. The lists
// were checked against the harness versions in the tested range.
type Widgets struct {
	Commands []Command `json:"commands"`
	// Models are suggestions for the live "/model <id>" action; users can
	// also enter a custom model ID.
	Models []string `json:"models"`
	// InterruptKeys are sent in order, with a short gap, to interrupt a working
	// harness. Copilot CLI needs Esc twice; a single Esc was verified not to stop it.
	InterruptKeys []string `json:"interruptKeys"`
	// CompactText is the command behind the Compact quick action.
	CompactText string `json:"compactText"`
	// ClearText is the command behind the Clear quick action; it discards the
	// current conversation context, so the UI asks for confirmation first.
	ClearText string `json:"clearText"`
}

// InstallSource identifies a maintained winget package and the vendor's
// official setup instructions used when winget is unavailable.
type InstallSource struct {
	WingetID  string `json:"wingetId,omitempty"`
	Source    string `json:"source,omitempty"`
	VendorURL string `json:"vendorUrl"`
}

// Spec is the metadata contract for one detectable tool. Minimum is optional;
// when empty, required capabilities decide whether the tool is usable.
// TestedFrom is inclusive and TestedBefore is exclusive.
type Spec struct {
	ID           ID
	Name         string
	Executable   string
	Required     bool
	VersionArgs  []string
	HelpArgs     []string
	Minimum      string
	TestedFrom   string
	TestedBefore string
	Capabilities []Capability
	Widgets      *Widgets
	Install      InstallSource
}

var specs = []Spec{
	{
		ID:           PowerShell,
		Name:         "PowerShell 7",
		Executable:   "pwsh.exe",
		Required:     true,
		VersionArgs:  []string{"--version"},
		HelpArgs:     []string{"-?"},
		Minimum:      "7.0.0",
		TestedFrom:   "7.6.0",
		TestedBefore: "7.7.0",
		Capabilities: []Capability{
			{Name: "Skip the startup banner", Option: "-NoLogo"},
			{Name: "Skip user profiles", Option: "-NoProfile"},
			{Name: "Run a bootstrap command", Option: "-Command"},
		},
		Install: InstallSource{
			WingetID:  "Microsoft.PowerShell",
			Source:    "winget",
			VendorURL: "https://learn.microsoft.com/powershell/scripting/install/installing-powershell-on-windows",
		},
	},
	{
		ID:           ClaudeCode,
		Name:         "Claude Code",
		Executable:   "claude",
		VersionArgs:  []string{"--version"},
		HelpArgs:     []string{"--help"},
		TestedFrom:   "2.1.133",
		TestedBefore: "2.2.0",
		Capabilities: []Capability{
			{Name: "Start with a TauShell session ID", Option: "--session-id"},
			{Name: "Resume a conversation", Option: "--resume"},
			{Name: "Select a model", Option: "--model", Optional: true},
		},
		Widgets: &Widgets{
			Commands: []Command{
				{Label: "Compact conversation", Text: "/compact"},
				{Label: "Clear conversation", Text: "/clear"},
				{Label: "Show context usage", Text: "/context"},
				{Label: "Show cost", Text: "/cost"},
				{Label: "Help", Text: "/help"},
			},
			Models:        []string{"default", "sonnet", "opus", "haiku"},
			InterruptKeys: []string{"\x1b"},
			CompactText:   "/compact",
			ClearText:     "/clear",
		},
		Install: InstallSource{
			WingetID:  "Anthropic.ClaudeCode",
			Source:    "winget",
			VendorURL: "https://code.claude.com/docs/en/setup",
		},
	},
	{
		ID:           CopilotCLI,
		Name:         "GitHub Copilot CLI",
		Executable:   "copilot",
		VersionArgs:  []string{"--version"},
		HelpArgs:     []string{"--help"},
		TestedFrom:   "1.0.100",
		TestedBefore: "1.1.0",
		Capabilities: []Capability{
			{Name: "Start with a TauShell session ID", Option: "--session-id"},
			{Name: "Resume a conversation", Option: "--resume"},
			{Name: "Select a model", Option: "--model", Optional: true},
		},
		Widgets: &Widgets{
			Commands: []Command{
				{Label: "Compact conversation", Text: "/compact"},
				{Label: "Clear conversation", Text: "/clear"},
				{Label: "Show context usage", Text: "/context"},
				{Label: "Show usage", Text: "/usage"},
				{Label: "Help", Text: "/help"},
			},
			Models:        []string{"auto", "claude-sonnet-5.5", "claude-opus-5.5", "claude-haiku-5.5"},
			InterruptKeys: []string{"\x1b", "\x1b"},
			CompactText:   "/compact",
			ClearText:     "/clear",
		},
		Install: InstallSource{
			WingetID:  "GitHub.Copilot",
			Source:    "winget",
			VendorURL: "https://docs.github.com/copilot/how-tos/set-up/install-copilot-cli",
		},
	},
}

// Specs returns the declared tools in display order.
func Specs() []Spec {
	return append([]Spec(nil), specs...)
}

func Lookup(id ID) (Spec, bool) {
	for _, spec := range specs {
		if spec.ID == id {
			return spec, true
		}
	}
	return Spec{}, false
}

type Version struct {
	Major, Minor, Patch int
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ParseVersion extracts the first major.minor.patch triple from tool output.
func ParseVersion(text string) (Version, bool) {
	match := versionPattern.FindStringSubmatch(text)
	if match == nil {
		return Version{}, false
	}
	var parts [3]int
	for index := range parts {
		value, err := strconv.Atoi(match[index+1])
		if err != nil {
			return Version{}, false
		}
		parts[index] = value
	}
	return Version{Major: parts[0], Minor: parts[1], Patch: parts[2]}, true
}

func mustParse(text string) Version {
	version, ok := ParseVersion(text)
	if !ok {
		panic(fmt.Sprintf("invalid declared version %q", text))
	}
	return version
}

func (version Version) Compare(other Version) int {
	for _, pair := range [][2]int{{version.Major, other.Major}, {version.Minor, other.Minor}, {version.Patch, other.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func (version Version) String() string {
	return fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
}

type Support string

const (
	SupportTested       Support = "tested"
	SupportUntested     Support = "untested"
	SupportBelowMinimum Support = "below-minimum"
	SupportUnknown      Support = "unknown"
)

// Classify reports where version falls relative to the declared minimum and
// tested range.
func (spec Spec) Classify(version Version) Support {
	if spec.Minimum != "" && version.Compare(mustParse(spec.Minimum)) < 0 {
		return SupportBelowMinimum
	}
	if version.Compare(mustParse(spec.TestedFrom)) >= 0 && version.Compare(mustParse(spec.TestedBefore)) < 0 {
		return SupportTested
	}
	return SupportUntested
}

func (spec Spec) TestedRange() string {
	return fmt.Sprintf(">=%s <%s", spec.TestedFrom, spec.TestedBefore)
}

type CapabilityStatus struct {
	Capability
	Available bool `json:"available"`
}

type Status struct {
	ID             ID                 `json:"id"`
	Name           string             `json:"name"`
	Required       bool               `json:"required"`
	Installed      bool               `json:"installed"`
	ExecutablePath string             `json:"executablePath,omitempty"`
	Version        string             `json:"version,omitempty"`
	VersionOutput  string             `json:"versionOutput,omitempty"`
	Minimum        string             `json:"minimum,omitempty"`
	TestedRange    string             `json:"testedRange"`
	Support        Support            `json:"support"`
	Capabilities   []CapabilityStatus `json:"capabilities"`
	Warnings       []string           `json:"warnings"`
	Error          string             `json:"error,omitempty"`
	Widgets        *Widgets           `json:"widgets,omitempty"`
	Install        InstallSource      `json:"install"`
}

// Usable reports whether the tool was found, meets the minimum, and exposes
// every required capability. An untested version can still be usable.
func (status Status) Usable() bool {
	if !status.Installed || status.Error != "" || status.Support == SupportBelowMinimum {
		return false
	}
	for _, capability := range status.Capabilities {
		if !capability.Available && !capability.Optional {
			return false
		}
	}
	return true
}

// Environment abstracts executable lookup and command execution for tests.
type Environment struct {
	LookPath func(string) (string, error)
	Run      func(ctx context.Context, path string, args ...string) ([]byte, error)
}

func DefaultEnvironment() Environment {
	return Environment{
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
			command := exec.CommandContext(ctx, path, args...)
			hideWindow(command)
			return command.CombinedOutput()
		},
	}
}

const probeTimeout = 10 * time.Second
const maxVersionOutput = 200

// Detect finds the executable, reads and classifies its version, and probes
// required capabilities from its help output.
func Detect(ctx context.Context, environment Environment, spec Spec) Status {
	status := Status{
		ID:          spec.ID,
		Name:        spec.Name,
		Required:    spec.Required,
		Minimum:     spec.Minimum,
		TestedRange: spec.TestedRange(),
		Support:     SupportUnknown,
		Warnings:    []string{},
		Widgets:     spec.Widgets,
		Install:     spec.Install,
	}
	for _, capability := range spec.Capabilities {
		status.Capabilities = append(status.Capabilities, CapabilityStatus{Capability: capability})
	}
	path, err := environment.LookPath(spec.Executable)
	if err != nil {
		status.Error = fmt.Sprintf("%s was not found on PATH", spec.Executable)
		return status
	}
	status.Installed = true
	status.ExecutablePath = path

	output, err := runProbe(ctx, environment, path, spec.VersionArgs)
	versionOutput := strings.TrimSpace(string(output))
	if len(versionOutput) > maxVersionOutput {
		versionOutput = versionOutput[:maxVersionOutput]
	}
	status.VersionOutput = versionOutput
	if err != nil {
		status.Error = fmt.Sprintf("run %s %s: %v", filepath.Base(path), strings.Join(spec.VersionArgs, " "), err)
		return status
	}
	if version, ok := ParseVersion(versionOutput); ok {
		status.Version = version.String()
		status.Support = spec.Classify(version)
		switch status.Support {
		case SupportBelowMinimum:
			status.Warnings = append(status.Warnings, fmt.Sprintf("Version %s is below the minimum supported version %s.", status.Version, spec.Minimum))
		case SupportUntested:
			status.Warnings = append(status.Warnings, fmt.Sprintf("Version %s is outside the tested range (%s). It can still be used while required capabilities are available.", status.Version, status.TestedRange))
		}
	} else {
		status.Warnings = append(status.Warnings, "The installed version could not be determined.")
	}

	if len(spec.Capabilities) == 0 {
		return status
	}
	help, err := runProbe(ctx, environment, path, spec.HelpArgs)
	if err != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("Capability probe failed: run %s %s: %v", filepath.Base(path), strings.Join(spec.HelpArgs, " "), err))
		return status
	}
	for index := range status.Capabilities {
		capability := &status.Capabilities[index]
		capability.Available = HasOption(string(help), capability.Option)
		if !capability.Available && !capability.Optional {
			status.Warnings = append(status.Warnings, fmt.Sprintf("Required option %s (%s) was not found in %s help output.", capability.Option, capability.Name, spec.Name))
		}
	}
	return status
}

func runProbe(ctx context.Context, environment Environment, path string, args []string) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return environment.Run(probeCtx, path, args...)
}

// HasOption reports whether help text lists option as a whole token.
func HasOption(help, option string) bool {
	pattern := regexp.MustCompile(`(?m)(^|[\s,\[(|])` + regexp.QuoteMeta(option) + `($|[\s,=<\[\])|])`)
	return pattern.MatchString(help)
}

// DetectAll detects every declared tool concurrently, preserving Specs order.
func DetectAll(ctx context.Context, environment Environment) []Status {
	all := Specs()
	statuses := make([]Status, len(all))
	var group sync.WaitGroup
	for index, spec := range all {
		group.Add(1)
		go func() {
			defer group.Done()
			statuses[index] = Detect(ctx, environment, spec)
		}()
	}
	group.Wait()
	return statuses
}
