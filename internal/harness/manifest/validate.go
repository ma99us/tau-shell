package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reader names a manifest may select. Each is implemented in Go next to the
// private log format it understands; the sets here keep validation in one place.
var (
	ResumeCheckReaders = names("claude-transcript", "copilot-session-state", "gsd-session-file", "codex-rollout", "opencode-db")
	DiscoveryReaders   = names("codex-rollout", "opencode-db")
	ModelReaders       = names("claude-jsonl", "copilot-events", "gsd-jsonl", "codex-rollout", "opencode-db")
	TitleReaders       = names("claude-ai-title", "copilot-workspace")
	UsageReaders       = names("claude-jsonl", "copilot-events", "gsd-jsonl", "codex-rollout", "opencode-db")
	PhoneAnswerReaders = names("claude-permission", "copilot-permission")
)

func names(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	colorPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	envPattern     = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)\|([^}]*)\}`)
	placeholder    = regexp.MustCompile(`\{[^}]*\}`)
	inlineFlagsRE  = regexp.MustCompile(`\(\?[a-zA-Z-]+[:)]|\(\?P`)
	cueStates      = names("input", "working", "idle")
	cueSources     = names("title", "last-rows", "last-text-rows")
	brandMarks     = names("claude", "copilot", "monogram")
	versionPattern = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)
)

// Decode parses and validates one manifest. Unknown fields are errors. The
// result is Builtin when builtin is true, which allows install package IDs.
func Decode(data []byte, builtin bool) (*Manifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var item Manifest
	if err := decoder.Decode(&item); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the file is empty")
		}
		return nil, fmt.Errorf("cannot parse: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("the file must hold exactly one manifest")
	}
	item.Builtin = builtin
	if err := item.Validate(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	item.Hash = hex.EncodeToString(sum[:])
	return &item, nil
}

// Validate checks a manifest for completeness and safety. Errors name the
// offending field.
func (item *Manifest) Validate() error {
	fail := func(field, format string, args ...any) error {
		return fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...))
	}
	if item.Schema != SchemaVersion {
		return fail("schema", "must be %d (got %d)", SchemaVersion, item.Schema)
	}
	if !idPattern.MatchString(item.ID) {
		return fail("id", "%q must be a lowercase slug of at most 63 characters", item.ID)
	}
	if strings.TrimSpace(item.Name) == "" || len(item.Name) > 60 {
		return fail("name", "is required and at most 60 characters")
	}
	if strings.TrimSpace(item.Short) == "" || len(item.Short) > 20 {
		return fail("short", "is required and at most 20 characters")
	}
	if err := checkExecutable(item.Executable); err != nil {
		return fail("executable", "%v", err)
	}
	if len(item.Probes.VersionArgs) == 0 {
		return fail("probes.version_args", "is required")
	}
	for _, field := range []struct{ name, value string }{
		{"probes.minimum", item.Probes.Minimum}, {"probes.tested_from", item.Probes.TestedFrom}, {"probes.tested_before", item.Probes.TestedBefore},
	} {
		if field.value != "" && !versionPattern.MatchString(field.value) {
			return fail(field.name, "%q must look like 1.2.3", field.value)
		}
	}
	if (item.Probes.TestedFrom == "") != (item.Probes.TestedBefore == "") {
		return fail("probes.tested_from", "and tested_before must be set together")
	}
	if len(item.Capabilities) > 0 && len(item.Probes.HelpArgs) == 0 {
		return fail("probes.help_args", "is required when capabilities are declared")
	}
	for index, capability := range item.Capabilities {
		if strings.TrimSpace(capability.Name) == "" || strings.TrimSpace(capability.Option) == "" {
			return fail(fmt.Sprintf("capabilities[%d]", index), "needs a name and an option")
		}
	}
	for index, arg := range item.Launch.DefaultArgs {
		if arg == "" {
			return fail(fmt.Sprintf("launch.default_args[%d]", index), "must not be empty")
		}
	}
	for index, arg := range item.Launch.ReservedArgs {
		if strings.TrimSpace(arg) == "" || strings.Contains(arg, "=") {
			return fail(fmt.Sprintf("launch.reserved_args[%d]", index), "must be an option name without a value")
		}
	}
	if err := item.validateSession(fail); err != nil {
		return err
	}
	for field, check := range map[string]struct {
		value string
		set   map[string]bool
	}{
		"resume_check":      {item.ResumeCheck, ResumeCheckReaders},
		"model":             {item.Model, ModelReaders},
		"title":             {item.Title, TitleReaders},
		"usage.reader":      {item.Usage.Reader, UsageReaders},
		"phone_answer":      {item.PhoneAnswer, PhoneAnswerReaders},
		"session.discovery": {item.Session.Discovery, DiscoveryReaders},
	} {
		if check.value != "" && !check.set[check.value] {
			return fail(field, "unknown reader %q", check.value)
		}
	}
	if item.Usage.RecordsOwnCost && item.Usage.Reader == "" {
		return fail("usage.records_own_cost", "needs usage.reader")
	}
	for index, step := range item.GracefulExit {
		if step.Input == "" {
			return fail(fmt.Sprintf("graceful_exit[%d].input", index), "must not be empty")
		}
		if step.PauseMS < 0 || step.PauseMS > 5000 {
			return fail(fmt.Sprintf("graceful_exit[%d].pause_ms", index), "must be 0 to 5000")
		}
	}
	if item.Widgets != nil {
		for index, command := range item.Widgets.Commands {
			if strings.TrimSpace(command.Label) == "" || strings.TrimSpace(command.Text) == "" {
				return fail(fmt.Sprintf("widgets.commands[%d]", index), "needs a label and text")
			}
		}
	}
	for index, cue := range item.Cues {
		if err := validateCue(cue); err != nil {
			return fail(fmt.Sprintf("cues[%d]", index), "%v", err)
		}
	}
	if item.ConfigFile != nil {
		if len(item.ConfigFile.Paths) == 0 {
			return fail("config_file.paths", "needs at least one path")
		}
		for index, path := range item.ConfigFile.Paths {
			if err := checkPathTemplate(path, ""); err != nil {
				return fail(fmt.Sprintf("config_file.paths[%d]", index), "%v", err)
			}
		}
	}
	for index, path := range item.Skills.PersonalDirs {
		if err := checkPathTemplate(path, ""); err != nil {
			return fail(fmt.Sprintf("skills.personal_dirs[%d]", index), "%v", err)
		}
	}
	if option := item.Skills.PluginDirOption; option != "" && !strings.HasPrefix(option, "--") {
		return fail("skills.plugin_dir_option", "must start with --")
	}
	return item.validateInstallAndBrand(fail)
}

func (item *Manifest) validateSession(fail func(string, string, ...any) error) error {
	session := item.Session
	switch session.ID {
	case "tau":
		if session.Discovery != "" {
			return fail("session.discovery", "applies only when session.id is discover")
		}
	case "discover":
		if session.Discovery == "" {
			return fail("session.discovery", "is required when session.id is discover")
		}
	default:
		return fail("session.id", "must be tau or discover (got %q)", session.ID)
	}
	if len(session.ResumeArgs) == 0 {
		return fail("session.resume_args", "is required: TauShell never claims a resume it cannot request")
	}
	switch session.ResumePos {
	case "", "after", "before":
	default:
		return fail("session.resume_position", "must be after or before")
	}
	if session.File != "" {
		if err := checkPathTemplate(session.File, "x"); err != nil {
			return fail("session.file", "%v", err)
		}
	}
	for _, field := range []struct {
		name string
		args []string
	}{{"session.start_args", session.StartArgs}, {"session.resume_args", session.ResumeArgs}} {
		for index, arg := range field.args {
			if arg == "" {
				return fail(fmt.Sprintf("%s[%d]", field.name, index), "must not be empty")
			}
			for _, found := range placeholder.FindAllString(arg, -1) {
				switch found {
				case "{id}":
					if session.ID == "discover" && field.name == "session.start_args" {
						return fail(fmt.Sprintf("%s[%d]", field.name, index), "{id} is unknown before launch when the harness picks its own ID")
					}
				case "{session_file}":
					if session.File == "" {
						return fail(fmt.Sprintf("%s[%d]", field.name, index), "{session_file} needs session.file")
					}
				default:
					return fail(fmt.Sprintf("%s[%d]", field.name, index), "unknown placeholder %s", found)
				}
			}
		}
	}
	return nil
}

func (item *Manifest) validateInstallAndBrand(fail func(string, string, ...any) error) error {
	parsed, err := url.Parse(item.Install.VendorURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fail("install.vendor_url", "must be an https URL")
	}
	if !item.Builtin && (item.Install.WingetID != "" || item.Install.NpmPackage != "") {
		return fail("install", "winget_id and npm_package are only for built-in harnesses; use vendor_url")
	}
	if !brandMarks[item.Brand.Mark] {
		return fail("brand.mark", "must be claude, copilot or monogram")
	}
	if !item.Builtin && item.Brand.Mark != "monogram" {
		return fail("brand.mark", "user harnesses must use monogram")
	}
	if len([]rune(item.Brand.Text)) > 2 {
		return fail("brand.text", "must be at most 2 characters")
	}
	if item.Brand.Color != "" && !colorPattern.MatchString(item.Brand.Color) {
		return fail("brand.color", "must be a #rrggbb value")
	}
	return nil
}

// checkExecutable accepts a bare command name resolved on PATH: no folders,
// no drive letters, no scripts.
func checkExecutable(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("is required")
	}
	if strings.ContainsAny(name, `/\:" `) || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") {
		return fmt.Errorf("%q must be a bare command name found on PATH, not a path", name)
	}
	if extension := strings.ToLower(filepath.Ext(name)); extension != "" && extension != ".exe" {
		return fmt.Errorf("%q must be a program name: scripts and shortcuts (%s) are not allowed", name, extension)
	}
	return nil
}

func validateCue(cue Cue) error {
	if !cueStates[cue.State] {
		return fmt.Errorf("state must be input, working or idle (got %q)", cue.State)
	}
	if !cueSources[cue.Source] {
		return fmt.Errorf("source must be title, last-rows or last-text-rows (got %q)", cue.Source)
	}
	switch {
	case cue.Source == "title" && cue.Rows != 0:
		return errors.New("rows does not apply to the title source")
	case cue.Source != "title" && (cue.Rows < 1 || cue.Rows > 24):
		return errors.New("rows must be 1 to 24")
	}
	return CheckPattern(cue.Pattern)
}

// CheckPattern accepts only regular expressions that mean the same in Go
// (RE2) and in JavaScript: no inline flags, named groups or Go-only escapes.
func CheckPattern(pattern string) error {
	if pattern == "" {
		return errors.New("pattern is required")
	}
	if len(pattern) > 500 {
		return errors.New("pattern is longer than 500 characters")
	}
	if inlineFlagsRE.MatchString(pattern) {
		return errors.New("pattern must not use inline flags or named groups; use ignore_case")
	}
	for _, goOnly := range []string{`\A`, `\z`, `\Z`, `\x{`, `[[:`, `\Q`, `\E`} {
		if strings.Contains(pattern, goOnly) {
			return fmt.Errorf("pattern uses %s, which JavaScript does not understand", goOnly)
		}
	}
	if _, err := CompilePattern(pattern, false); err != nil {
		return fmt.Errorf("pattern does not compile: %w", err)
	}
	return nil
}

var unicodeEscape = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

// CompilePattern compiles a cue pattern in Go. \uXXXX escapes, which
// JavaScript understands natively, are rewritten to Go's \x{XXXX} form.
func CompilePattern(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	translated := unicodeEscape.ReplaceAllString(pattern, `\x{$1}`)
	if ignoreCase {
		translated = "(?i)" + translated
	}
	return regexp.Compile(translated)
}

// ExpandPath resolves a path template: ~ for the home folder, {id}, and
// {env:NAME|fallback}. Nothing else expands.
func ExpandPath(template, id string) (string, error) {
	return expandPath(template, id, true)
}

func checkPathTemplate(template, id string) error {
	_, err := expandPath(template, id, false)
	return err
}

func expandPath(template, id string, strict bool) (string, error) {
	home, _ := os.UserHomeDir()
	resolve := func(text string) string {
		if text == "~" || strings.HasPrefix(text, "~/") || strings.HasPrefix(text, `~\`) {
			return home + text[1:]
		}
		return text
	}
	expanded := envPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if value := os.Getenv(parts[1]); value != "" {
			return value
		}
		return resolve(parts[2])
	})
	expanded = resolve(expanded)
	for _, found := range placeholder.FindAllString(expanded, -1) {
		if found != "{id}" {
			return "", fmt.Errorf("unknown placeholder %s in %q", found, template)
		}
		if id == "" {
			return "", fmt.Errorf("{id} is not available in %q", template)
		}
	}
	expanded = strings.ReplaceAll(expanded, "{id}", id)
	if strict && strings.TrimSpace(expanded) == "" {
		return "", fmt.Errorf("%q expands to nothing", template)
	}
	return filepath.FromSlash(expanded), nil
}

// ExpandArg fills {id} and {session_file} in one argument.
func ExpandArg(arg, id, sessionFile string) string {
	arg = strings.ReplaceAll(arg, "{id}", id)
	return strings.ReplaceAll(arg, "{session_file}", sessionFile)
}
