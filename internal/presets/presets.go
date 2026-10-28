// Package presets loads user-authored prompt presets from a hand-edited YAML
// file and expands their variables. Presets are plain text TauShell types into
// a harness; they are not validated harness commands.
package presets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const FileName = "prompts.yaml"

const exampleFile = `# TauShell prompt presets. Edit freely; TauShell never overwrites this file.
# Variables: {project} {dir} {session} {branch} {date}
version: 1
presets:
  - name: Review uncommitted changes
    text: "Review the uncommitted changes in {project} on branch {branch} and list any bugs."
  - name: Explain this project
    text: "Give me a short overview of {project}: purpose, layout, and how to build and test it."
    # tools: [claude-code, copilot-cli]   # optional: limit to these harnesses
    # submit: false                       # optional: insert only, do not press Enter
`

// Preset is one saved prompt. Submit defaults to true when omitted.
type Preset struct {
	Name   string   `yaml:"name"`
	Text   string   `yaml:"text"`
	Tools  []string `yaml:"tools,omitempty"`
	Submit *bool    `yaml:"submit,omitempty"`
	// Skill hides the preset unless that skill is available to the session.
	Skill string `yaml:"skill,omitempty"`
}

// Resolved is a preset ready to send.
type Resolved struct {
	Name   string `json:"name"`
	Text   string `json:"text"`
	Submit bool   `json:"submit"`
	// Unknown lists variables that were left as written.
	Unknown []string `json:"unknown,omitempty"`
}

type file struct {
	Version int      `yaml:"version"`
	Presets []Preset `yaml:"presets"`
}

// Context supplies variable values.
type Context struct {
	Tool       string
	Session    string
	WorkingDir string
	// Skills is the set of skill names available to the session's harness.
	Skills map[string]bool
}

// Load reads the global presets. A missing file is an empty list; invalid
// content is an error.
func Load(path string) ([]Preset, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read prompt presets: %w", err)
	}
	var parsed file
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if parsed.Version != 0 && parsed.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported version %d", filepath.Base(path), parsed.Version)
	}
	if err := Validate(parsed.Presets); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return parsed.Presets, nil
}

// Validate checks preset entries; it is also used for session YAML presets.
func Validate(list []Preset) error {
	seen := map[string]bool{}
	for index, preset := range list {
		name := strings.TrimSpace(preset.Name)
		if name == "" || strings.TrimSpace(preset.Text) == "" {
			return fmt.Errorf("preset %d needs a name and text", index+1)
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("duplicate preset name %q", name)
		}
		seen[strings.ToLower(name)] = true
	}
	return nil
}

// Merge returns global presets followed by session presets; a session preset
// replaces a global one with the same name.
func Merge(global, session []Preset) []Preset {
	merged := make([]Preset, 0, len(global)+len(session))
	for _, preset := range global {
		replaced := false
		for _, override := range session {
			if strings.EqualFold(override.Name, preset.Name) {
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, preset)
		}
	}
	return append(merged, session...)
}

var variablePattern = regexp.MustCompile(`\{([a-z]+)\}`)

// Resolve filters presets for the tool and expands variables.
func Resolve(ctx context.Context, list []Preset, values Context) []Resolved {
	var branch *string
	lookup := func(name string) (string, bool) {
		switch name {
		case "project":
			return filepath.Base(values.WorkingDir), true
		case "dir":
			return values.WorkingDir, true
		case "session":
			return values.Session, true
		case "date":
			return time.Now().Format("2006-01-02"), true
		case "branch":
			if branch == nil {
				value := gitBranch(ctx, values.WorkingDir)
				branch = &value
			}
			return *branch, true
		}
		return "", false
	}
	resolved := []Resolved{}
	for _, preset := range list {
		if len(preset.Tools) > 0 && !contains(preset.Tools, values.Tool) {
			continue
		}
		if preset.Skill != "" && !values.Skills[preset.Skill] {
			continue
		}
		item := Resolved{Name: preset.Name, Submit: preset.Submit == nil || *preset.Submit}
		item.Text = variablePattern.ReplaceAllStringFunc(preset.Text, func(match string) string {
			name := match[1 : len(match)-1]
			if value, ok := lookup(name); ok {
				return value
			}
			item.Unknown = append(item.Unknown, name)
			return match
		})
		resolved = append(resolved, item)
	}
	return resolved
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func gitBranch(ctx context.Context, dir string) string {
	return GitBranch(ctx, dir)
}

// GitBranch returns the current branch of the repository at dir, or "" when
// dir is not a git working tree.
func GitBranch(ctx context.Context, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	hideWindow(command)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// EnsureExample creates a commented example file only when none exists.
func EnsureExample(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(exampleFile)
	return errors.Join(writeErr, file.Close())
}
