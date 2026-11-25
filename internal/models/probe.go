package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Runner runs an executable and returns its output.
type Runner func(ctx context.Context, path string, args ...string) ([]byte, error)

// Env is what probes need from the machine: a way to run the harness and the
// user's home directory.
type Env struct {
	Run  Runner
	Home string
	// Executable resolves the installed harness path; it returns "" when the harness is not installed.
	Executable func(ctx context.Context, harnessID string) string
}

// Result is what a probe found. Authoritative results replace the manifest's
// static list (the harness knows its real catalog); others add to it.
type Result struct {
	Candidates    []Candidate
	Authoritative bool
}

// Harness ids that have a probe. Every other harness keeps its manifest list.
const (
	harnessClaude   = "claude-code"
	harnessCodex    = "codex"
	harnessCopilot  = "copilot-cli"
	harnessOpenCode = "opencode"
)

var errNoProbe = errors.New("no model probe for this harness")

// HasProbe reports whether a harness has a model probe.
func HasProbe(harnessID string) bool {
	switch harnessID {
	case harnessClaude, harnessCodex, harnessCopilot, harnessOpenCode:
		return true
	}
	return false
}

// Probe asks an installed harness which models it can use. executable is the
// resolved harness path and may be empty for probes that only read config.
func Probe(ctx context.Context, harnessID, executable string, env Env) (Result, error) {
	switch harnessID {
	case harnessCodex:
		return probeCodex(ctx, executable, env)
	case harnessOpenCode:
		return probeOpenCode(ctx, executable, env)
	case harnessClaude:
		return probeClaude(env)
	case harnessCopilot:
		return probeCopilot(env)
	}
	return Result{}, errNoProbe
}

// jsonObject cuts the first JSON object out of command output that may carry
// warnings before or after it.
func jsonObject(output []byte) []byte {
	start, end := bytes.IndexByte(output, '{'), bytes.LastIndexByte(output, '}')
	if start < 0 || end < start {
		return nil
	}
	return output[start : end+1]
}

func probeCodex(ctx context.Context, executable string, env Env) (Result, error) {
	if executable == "" || env.Run == nil {
		return Result{}, errors.New("codex is not installed")
	}
	output, err := env.Run(ctx, executable, "debug", "models")
	if err != nil {
		return Result{}, fmt.Errorf("codex debug models: %w", err)
	}
	return parseCodexModels(output)
}

func parseCodexModels(output []byte) (Result, error) {
	var catalog struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	if err := json.Unmarshal(jsonObject(output), &catalog); err != nil {
		return Result{}, fmt.Errorf("codex model list is not readable: %w", err)
	}
	result := Result{Authoritative: true}
	for _, model := range catalog.Models {
		if model.Slug == "" || model.Visibility != "list" {
			continue
		}
		result.Candidates = append(result.Candidates, Candidate{ID: model.Slug, Label: model.DisplayName})
	}
	if len(result.Candidates) == 0 {
		return Result{}, errors.New("codex reported no models")
	}
	return result, nil
}

var openCodeHeader = regexp.MustCompile(`^[A-Za-z0-9._-]+/\S+$`)

func probeOpenCode(ctx context.Context, executable string, env Env) (Result, error) {
	if executable == "" || env.Run == nil {
		return Result{}, errors.New("opencode is not installed")
	}
	output, err := env.Run(ctx, executable, "models", "--verbose")
	if err != nil {
		return Result{}, fmt.Errorf("opencode models: %w", err)
	}
	return parseOpenCodeModels(output)
}

// parseOpenCodeModels reads "opencode models --verbose": a "provider/model"
// line followed by that model's JSON record. OpenCode's /models picker filters
// by display name, so the name is what gets sent.
func parseOpenCodeModels(output []byte) (Result, error) {
	type record struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
		Name       string `json:"name"`
		Status     string `json:"status"`
	}
	var records []record
	var block strings.Builder
	flush := func() {
		if block.Len() == 0 {
			return
		}
		var item record
		if json.Unmarshal([]byte(block.String()), &item) == nil && item.ID != "" && item.Name != "" && item.Status != "deprecated" {
			records = append(records, item)
		}
		block.Reset()
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if openCodeHeader.MatchString(line) {
			flush()
			continue
		}
		block.WriteString(line)
		block.WriteByte('\n')
	}
	flush()
	result := Result{Authoritative: true}
	for _, item := range records {
		result.Candidates = append(result.Candidates, Candidate{
			ID:       item.Name,
			Label:    item.Name + " (" + item.ProviderID + ")",
			MatchKey: item.ID,
		})
	}
	if len(result.Candidates) == 0 {
		return Result{}, errors.New("opencode reported no models")
	}
	return result, nil
}

func probeClaude(env Env) (Result, error) {
	if env.Home == "" {
		return Result{}, errors.New("home directory unknown")
	}
	data, err := os.ReadFile(filepath.Join(env.Home, ".claude", "settings.json"))
	if err != nil {
		return Result{}, nil
	}
	return parseClaudeSettings(data)
}

// parseClaudeSettings reads the models Claude Code is pinned to. The family
// aliases (opus, sonnet, haiku) resolve to the pinned model when one is set.
func parseClaudeSettings(data []byte) (Result, error) {
	var settings struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Result{}, fmt.Errorf("claude settings are not readable: %w", err)
	}
	var result Result
	for alias, variable := range map[string]string{"opus": "ANTHROPIC_DEFAULT_OPUS_MODEL", "sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL", "haiku": "ANTHROPIC_DEFAULT_HAIKU_MODEL"} {
		if pinned := strings.TrimSpace(settings.Env[variable]); pinned != "" {
			result.Candidates = append(result.Candidates, Candidate{ID: alias, Label: alias + " (" + pinned + ")", MatchKey: pinned})
		}
	}
	sort.Slice(result.Candidates, func(left, right int) bool { return result.Candidates[left].ID < result.Candidates[right].ID })
	if model := strings.TrimSpace(settings.Model); strings.HasPrefix(strings.ToLower(model), "claude-") {
		result.Candidates = append(result.Candidates, Candidate{ID: model})
	}
	return result, nil
}

func probeCopilot(env Env) (Result, error) {
	if env.Home == "" {
		return Result{}, errors.New("home directory unknown")
	}
	data, err := os.ReadFile(filepath.Join(env.Home, ".copilot", "config.json"))
	if err != nil {
		return Result{}, nil
	}
	return parseCopilotConfig(data)
}

// parseCopilotConfig reads the models recently used in Copilot CLI. It has no
// command that lists its catalog, so these add to the manifest list.
func parseCopilotConfig(data []byte) (Result, error) {
	var config struct {
		Recent []string `json:"recentModelIds"`
	}
	// Copilot writes "//" comment lines above the JSON.
	var body []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			body = append(body, line)
		}
	}
	if err := json.Unmarshal([]byte(strings.Join(body, "\n")), &config); err != nil {
		return Result{}, fmt.Errorf("copilot config is not readable: %w", err)
	}
	var result Result
	for _, id := range config.Recent {
		if id = strings.TrimSpace(id); id != "" {
			result.Candidates = append(result.Candidates, Candidate{ID: id})
		}
	}
	return result, nil
}

// Combine builds the candidate list for a harness: an authoritative probe
// result replaces the manifest list; any other result is added to it.
func Combine(manifest []string, probe Result) []Candidate {
	var fromManifest []Candidate
	for _, id := range manifest {
		fromManifest = append(fromManifest, Candidate{ID: id})
	}
	if probe.Authoritative && len(probe.Candidates) > 0 {
		return probe.Candidates
	}
	return append(fromManifest, probe.Candidates...)
}
