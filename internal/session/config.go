package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/ma99us/tau-shell/internal/fsutil"
	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/presets"
	"github.com/ma99us/tau-shell/internal/vault"
)

type Harness string

const (
	ClaudeCode Harness = "claude-code"
	CopilotCLI Harness = "copilot-cli"
	GSD        Harness = "gsd"
	Codex      Harness = "codex"
	OpenCode   Harness = "opencode"
)

// SupportedHarnesses lists every harness ID in display order: the built-in
// harnesses, then any the user declared with a manifest.
func SupportedHarnesses() []Harness {
	items := manifest.All()
	ids := make([]Harness, len(items))
	for index, item := range items {
		ids[index] = Harness(item.ID)
	}
	return ids
}

func (harness Harness) Supported() bool {
	_, ok := manifest.Get(string(harness))
	return ok
}

var (
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	colorPattern       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	ErrConfigChanged   = errors.New("session config changed outside TauShell; reload and retry")
)

const (
	maxNameLength  = 80
	MaxNotesLength = 500
	MaxTags        = 8
	MaxTagLength   = 120
	MaxLinkLength  = 500
)

type Config struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Color string `yaml:"color,omitempty"`
	// Notes is free text shown on the Home tile and the phone. http(s) URLs in
	// it become links. Tags are short labels without whitespace; a URL is a
	// valid tag. Neither is ever sent to the harness.
	Notes string   `yaml:"notes,omitempty"`
	Tags  []string `yaml:"tags,omitempty"`
	// Link is one http(s) URL, typically the Jira issue or pull request the
	// session works on. It is shown on the Home tile and the phone.
	Link string `yaml:"link,omitempty"`
	// AutoResume resumes this session when TauShell starts, if it has a saved
	// harness conversation. The Home tile checkbox edits it.
	AutoResume bool `yaml:"auto_resume,omitempty"`
	// Skills set to false keeps TauShell's skill library out of this session.
	Skills      *bool             `yaml:"skills,omitempty"`
	Tool        Harness           `yaml:"tool"`
	WorkingDir  string            `yaml:"working_dir"`
	LaunchArgs  []string          `yaml:"launch_args,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	// Secrets maps an environment variable name to the name of a vault secret.
	// The value is looked up at launch and goes only into the process environment.
	Secrets    map[string]string `yaml:"secrets,omitempty"`
	PowerShell struct {
		UTF8 *bool `yaml:"utf8"`
	} `yaml:"powershell,omitempty"`
	// Runtime is where the harness runs: "powershell" (default) or "wsl".
	Runtime string `yaml:"runtime,omitempty"`
	WSL     struct {
		Distro string `yaml:"distro,omitempty"`
	} `yaml:"wsl,omitempty"`
	// Presets are session-level prompt presets merged over the global
	// prompts.yaml entries by name.
	Presets []presets.Preset `yaml:"presets,omitempty"`
}

// LaunchEnvironment returns the session's literal environment plus its vault
// secrets. A missing secret is an error naming the secret, never a value.
func (config Config) LaunchEnvironment(store vault.Store) (map[string]string, error) {
	if len(config.Secrets) == 0 {
		return config.Environment, nil
	}
	environment := make(map[string]string, len(config.Environment)+len(config.Secrets))
	for name, value := range config.Environment {
		environment[name] = value
	}
	variables := make([]string, 0, len(config.Secrets))
	for variable := range config.Secrets {
		variables = append(variables, variable)
	}
	sort.Strings(variables)
	for _, variable := range variables {
		name := config.Secrets[variable]
		if store == nil {
			return nil, fmt.Errorf("session %q uses secret %q for %s, but the secret vault is unavailable", config.ID, name, variable)
		}
		value, err := store.Get(name)
		if errors.Is(err, vault.ErrNotFound) {
			return nil, fmt.Errorf("session %q needs secret %q for %s, which is not in the vault; add it on Home under Secrets", config.ID, name, variable)
		}
		if err != nil {
			return nil, fmt.Errorf("session %q: read secret %q: %w", config.ID, name, err)
		}
		environment[variable] = value
	}
	return environment, nil
}

func (config Config) SkillsEnabled() bool {
	return config.Skills == nil || *config.Skills
}

func (config Config) UTF8Enabled() bool {
	return config.PowerShell.UTF8 == nil || *config.PowerShell.UTF8
}

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open session config %q: %w", path, err)
	}
	defer file.Close()

	config, err := parseConfig(file, fmt.Sprintf("%q", path))
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

// ParseConfig strictly decodes and validates one session YAML document, such
// as text edited in the Create dialog.
func ParseConfig(text string) (Config, error) {
	return parseConfig(strings.NewReader(text), "text")
}

// MarshalConfig renders config in the same form SaveConfig writes.
func MarshalConfig(config Config) (string, error) {
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode session config: %w", err)
	}
	return string(data), nil
}

func parseConfig(reader io.Reader, source string) (Config, error) {
	var config Config
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode session config %s: %w", source, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return Config{}, fmt.Errorf("decode trailing session YAML in %s: %w", source, err)
		}
		return Config{}, fmt.Errorf("session config %s must contain one YAML document", source)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate session config %s: %w", source, err)
	}
	return config, nil
}

func (config Config) Validate() error {
	if !idPattern.MatchString(config.ID) {
		return fmt.Errorf("id %q must be a lowercase slug of at most 63 characters", config.ID)
	}
	if strings.TrimSpace(config.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if config.Color != "" && !colorPattern.MatchString(config.Color) {
		return fmt.Errorf("color %q must be a #rrggbb hex value", config.Color)
	}
	if err := ValidateNotes(config.Notes); err != nil {
		return err
	}
	if err := ValidateTags(config.Tags); err != nil {
		return err
	}
	if err := ValidateLink(config.Link); err != nil {
		return err
	}
	if !config.UTF8Enabled() {
		return fmt.Errorf("powershell.utf8 cannot be disabled in the Windows 10 Phase 1 runtime")
	}
	if !config.Tool.Supported() {
		supportedHarnesses := SupportedHarnesses()
		names := make([]string, len(supportedHarnesses))
		for index, supported := range supportedHarnesses {
			names[index] = string(supported)
		}
		return fmt.Errorf("unsupported tool %q (supported: %s)", config.Tool, strings.Join(names, ", "))
	}
	if config.WorkingDir == "" || !(filepath.IsAbs(config.WorkingDir) || (config.IsWSL() && strings.HasPrefix(config.WorkingDir, "/"))) {
		return fmt.Errorf("working_dir must be an absolute path")
	}
	if err := config.validateRuntime(); err != nil {
		return err
	}
	if config.IsWSL() && strings.HasPrefix(config.WorkingDir, "/") {
		// A Linux path cannot be checked without starting the distro; the
		// launch fails visibly if it does not exist.
		return config.validateRest()
	}
	info, err := os.Stat(config.WorkingDir)
	if err != nil {
		return fmt.Errorf("working_dir %q: %w", config.WorkingDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working_dir %q is not a directory", config.WorkingDir)
	}
	return config.validateRest()
}

// IsWSL reports whether the harness runs inside a WSL distro.
func (config Config) IsWSL() bool { return config.Runtime == "wsl" }

var wslDistroPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateWSLDistro accepts a distro name safe to pass as one wsl.exe argument.
func ValidateWSLDistro(distro string) error {
	if !wslDistroPattern.MatchString(distro) {
		return fmt.Errorf("wsl.distro %q must be 1 to 64 letters, digits, dots, dashes or underscores", distro)
	}
	return nil
}

func (config Config) validateRuntime() error {
	switch config.Runtime {
	case "", "powershell":
		if config.WSL.Distro != "" {
			return errors.New("wsl.distro is only valid when runtime is wsl")
		}
	case "wsl":
		return ValidateWSLDistro(config.WSL.Distro)
	default:
		return fmt.Errorf("unsupported runtime %q (supported: powershell, wsl)", config.Runtime)
	}
	return nil
}

func (config Config) validateRest() error {
	for name := range config.Environment {
		if !environmentPattern.MatchString(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
	}
	for variable, secret := range config.Secrets {
		if !environmentPattern.MatchString(variable) {
			return fmt.Errorf("invalid environment variable name %q in secrets", variable)
		}
		if _, duplicate := config.Environment[variable]; duplicate {
			return fmt.Errorf("%s is set in both environment and secrets", variable)
		}
		if err := vault.ValidateName(secret); err != nil {
			return fmt.Errorf("secrets.%s: %w", variable, err)
		}
	}
	if err := presets.Validate(config.Presets); err != nil {
		return fmt.Errorf("presets: %w", err)
	}
	if err := ValidateLaunchArgs(config.Tool, config.LaunchArgs); err != nil {
		return err
	}
	return nil
}

// ValidateNotes accepts free text with newlines but no other control characters.
func ValidateNotes(notes string) error {
	if len([]rune(notes)) > MaxNotesLength {
		return fmt.Errorf("notes must be at most %d characters", MaxNotesLength)
	}
	if strings.IndexFunc(notes, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0 {
		return errors.New("notes must not contain control characters")
	}
	return nil
}

// NormalizeTags trims, drops empty entries and removes case-insensitive duplicates.
func NormalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	var result []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if _, duplicate := seen[key]; tag == "" || duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result
}

// ValidateTags requires short tags without whitespace or control characters,
// so a URL is a valid tag.
func ValidateTags(tags []string) error {
	if len(tags) > MaxTags {
		return fmt.Errorf("at most %d tags are allowed", MaxTags)
	}
	for _, tag := range tags {
		if tag == "" || len([]rune(tag)) > MaxTagLength {
			return fmt.Errorf("tags must be 1 to %d characters", MaxTagLength)
		}
		if strings.IndexFunc(tag, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return fmt.Errorf("tag %q must not contain spaces or control characters", tag)
		}
	}
	return nil
}

// ValidateLink accepts an empty value or one http(s) URL without whitespace.
func ValidateLink(link string) error {
	if link == "" {
		return nil
	}
	if len([]rune(link)) > MaxLinkLength {
		return fmt.Errorf("link must be at most %d characters", MaxLinkLength)
	}
	if strings.IndexFunc(link, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return errors.New("link must not contain spaces or control characters")
	}
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("link must be an http or https URL")
	}
	return nil
}

var (
	pullPath      = regexp.MustCompile(`/(?:pull|pull-requests)/(\d+)(?:/|$)`)
	issuePath     = regexp.MustCompile(`/issues/(\d+)(?:/|$)`)
	mergePath     = regexp.MustCompile(`/merge_requests/(\d+)(?:/|$)`)
	jiraPath      = regexp.MustCompile(`/browse/([A-Za-z][A-Za-z0-9_]*-\d+)(?:/|$)`)
	jiraBoardPath = regexp.MustCompile(`(?:^|[?&])selectedIssue=([A-Za-z][A-Za-z0-9_]*-\d+)`)
)

// LinkLabel is a short name for a link: "PR #12", "Issue #7", "MR !3", a Jira
// key such as "VTA-123", or the host for any other address.
func LinkLabel(link string) string {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Host == "" {
		return ""
	}
	switch {
	case mergePath.MatchString(parsed.Path):
		return "MR !" + mergePath.FindStringSubmatch(parsed.Path)[1]
	case pullPath.MatchString(parsed.Path):
		return "PR #" + pullPath.FindStringSubmatch(parsed.Path)[1]
	case jiraPath.MatchString(parsed.Path):
		return strings.ToUpper(jiraPath.FindStringSubmatch(parsed.Path)[1])
	case jiraBoardPath.MatchString(parsed.RawQuery):
		return strings.ToUpper(jiraBoardPath.FindStringSubmatch(parsed.RawQuery)[1])
	case issuePath.MatchString(parsed.Path):
		return "Issue #" + issuePath.FindStringSubmatch(parsed.Path)[1]
	}
	return parsed.Hostname()
}

var linkPattern = regexp.MustCompile("https?://[^\\s<>\"'`]+")

// LinkURLs returns the http(s) URLs in text in order, without trailing
// punctuation. The frontend uses the same rule to build links.
func LinkURLs(text string) []string {
	var urls []string
	for _, match := range linkPattern.FindAllString(text, -1) {
		match = strings.TrimRight(match, ".,;:!?)]}")
		if parsed, err := url.Parse(match); err == nil && parsed.Host != "" {
			urls = append(urls, match)
		}
	}
	return urls
}

// HasLink reports whether target is one of the URLs in the session's notes or tags.
func (config Config) HasLink(target string) bool {
	if config.Link != "" && config.Link == target {
		return true
	}
	for _, source := range append([]string{config.Notes}, config.Tags...) {
		for _, candidate := range LinkURLs(source) {
			if candidate == target {
				return true
			}
		}
	}
	return false
}

// LockConfig serializes a session's configuration read and runtime reservation
// with updates and staged deletion across TauShell processes on Windows.
func LockConfig(directory, id string) (func(), error) {
	if !idPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid session id %q", id)
	}
	return lockConfigPath(filepath.Join(directory, id+".yaml"))
}

// ValidateLaunchArgs rejects user-configured launch arguments that would
// override a TauShell-managed session/resume identity argument for tool.
// Exported so callers (such as a launch-command preview) can reuse the same
// check without a full, filesystem-backed Config.Validate.
func ValidateLaunchArgs(tool Harness, launchArgs []string) error {
	item, known := manifest.Get(string(tool))
	if !known {
		return nil
	}
	reserved := make(map[string]struct{}, len(item.Launch.ReservedArgs))
	for _, name := range item.Launch.ReservedArgs {
		reserved[strings.ToLower(name)] = struct{}{}
	}
	for _, arg := range launchArgs {
		name, _, _ := strings.Cut(arg, "=")
		if _, isReserved := reserved[strings.ToLower(name)]; isReserved {
			return fmt.Errorf("launch_args cannot override TauShell-managed %s arguments", name)
		}
	}
	return nil
}

func SaveConfig(directory string, config Config) (string, error) {
	if err := config.Validate(); err != nil {
		return "", fmt.Errorf("validate session config: %w", err)
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encode session config: %w", err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create session config directory: %w", err)
	}
	path := filepath.Join(directory, config.ID+".yaml")
	unlock, err := lockConfigPath(path)
	if err != nil {
		return "", fmt.Errorf("lock session config %q: %w", path, err)
	}
	defer unlock()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create session config %q: %w", path, err)
	}
	writeErr := func() error {
		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("write session config %q: %w", path, err)
		}
		if err := file.Sync(); err != nil {
			return fmt.Errorf("sync session config %q: %w", path, err)
		}
		return nil
	}()
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		removeErr := os.Remove(path)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove incomplete session config %q: %w", path, removeErr))
		}
		return "", err
	}
	return path, nil
}

// UpdateConfigField rewrites one top-level field ("name", "color", "notes",
// "link" or "auto_resume") of an existing session YAML, preserving other keys and
// comments. TauShell writers are serialized across processes; an external edit
// observed before replacement returns ErrConfigChanged. An empty color, notes,
// link or auto_resume removes the key. The ID and filename never change.
func UpdateConfigField(directory, id, key, value string) (Config, error) {
	if !idPattern.MatchString(id) {
		return Config{}, fmt.Errorf("invalid session id %q", id)
	}
	switch key {
	case "name":
		value = strings.TrimSpace(value)
		if value == "" {
			return Config{}, errors.New("name must not be empty")
		}
		if len([]rune(value)) > maxNameLength {
			return Config{}, fmt.Errorf("name must be at most %d characters", maxNameLength)
		}
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return Config{}, errors.New("name must not contain control characters")
		}
	case "color":
	case "notes":
		value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
		if err := ValidateNotes(value); err != nil {
			return Config{}, err
		}
	case "link":
		value = strings.TrimSpace(value)
		if err := ValidateLink(value); err != nil {
			return Config{}, err
		}
	case "auto_resume":
		if value != "" && value != "true" {
			return Config{}, fmt.Errorf("auto_resume must be true or empty")
		}
	default:
		return Config{}, fmt.Errorf("unsupported session field %q", key)
	}
	return updateConfigDocument(directory, id, func(mapping *yaml.Node) { setMappingString(mapping, key, value) })
}

// UpdateConfigTags replaces the session's tags, or removes the key when none
// remain. It follows the same locking and comment-preserving rules as
// UpdateConfigField.
func UpdateConfigTags(directory, id string, tags []string) (Config, error) {
	if !idPattern.MatchString(id) {
		return Config{}, fmt.Errorf("invalid session id %q", id)
	}
	tags = NormalizeTags(tags)
	if err := ValidateTags(tags); err != nil {
		return Config{}, err
	}
	return updateConfigDocument(directory, id, func(mapping *yaml.Node) { setMappingSequence(mapping, "tags", tags) })
}

func updateConfigDocument(directory, id string, mutate func(*yaml.Node)) (Config, error) {
	path := filepath.Join(directory, id+".yaml")
	unlock, err := lockConfigPath(path)
	if err != nil {
		return Config{}, fmt.Errorf("lock session config %q: %w", path, err)
	}
	defer unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read session config %q: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return Config{}, fmt.Errorf("decode session config %q: %w", path, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Config{}, fmt.Errorf("session config %q must be a YAML mapping", path)
	}
	mutate(document.Content[0])
	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := errors.Join(encoder.Encode(&document), encoder.Close()); err != nil {
		return Config{}, fmt.Errorf("encode session config %q: %w", path, err)
	}
	updated, err := parseConfig(bytes.NewReader(encoded.Bytes()), fmt.Sprintf("%q", path))
	if err != nil {
		return Config{}, err
	}
	if updated.ID != id {
		return Config{}, fmt.Errorf("session config filename %q does not match declared id %q", id, updated.ID)
	}
	if err := replaceConfigIfUnchanged(path, data, encoded.Bytes()); err != nil {
		return Config{}, err
	}
	return updated, nil
}

func replaceConfigIfUnchanged(path string, original, replacement []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("recheck session config %q before replace: %w", path, err)
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("%w: %s", ErrConfigChanged, path)
	}
	return replaceFile(path, replacement)
}

func setMappingString(mapping *yaml.Node, key, value string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		if value == "" {
			preserveRemovedMappingComments(mapping, index)
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
		previous := mapping.Content[index+1]
		replacement := stringNode(key, value)
		replacement.HeadComment = previous.HeadComment
		replacement.LineComment = previous.LineComment
		replacement.FootComment = previous.FootComment
		mapping.Content[index+1] = replacement
		return
	}
	if value == "" {
		return
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, stringNode(key, value))
}

func setMappingSequence(mapping *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		setMappingString(mapping, key, "")
		return
	}
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, value := range values {
		sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle})
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			previous := mapping.Content[index+1]
			sequence.HeadComment, sequence.LineComment, sequence.FootComment = previous.HeadComment, previous.LineComment, previous.FootComment
			mapping.Content[index+1] = sequence
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, sequence)
}

func preserveRemovedMappingComments(mapping *yaml.Node, keyIndex int) {
	key, value := mapping.Content[keyIndex], mapping.Content[keyIndex+1]
	var comments []string
	for _, comment := range []string{key.HeadComment, key.LineComment, key.FootComment, value.HeadComment, value.LineComment, value.FootComment} {
		if comment != "" {
			comments = append(comments, comment)
		}
	}
	if len(comments) == 0 {
		return
	}
	comment := strings.Join(comments, "\n")
	if keyIndex+2 < len(mapping.Content) {
		next := mapping.Content[keyIndex+2]
		next.HeadComment = joinYAMLComment(comment, next.HeadComment)
		return
	}
	if keyIndex > 0 {
		previous := mapping.Content[keyIndex-1]
		previous.FootComment = joinYAMLComment(previous.FootComment, comment)
		return
	}
	mapping.FootComment = joinYAMLComment(mapping.FootComment, comment)
}

func joinYAMLComment(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "\n" + second
}

func stringNode(key, value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if key == "color" {
		node.Style = yaml.DoubleQuotedStyle
	}
	if key == "notes" && strings.Contains(value, "\n") {
		node.Style = yaml.LiteralStyle
	}
	if key == "auto_resume" {
		node.Tag = "!!bool"
	}
	return node
}

// replaceFile delegates to the shared temporary-file replacement helper.
func replaceFile(path string, data []byte) error {
	if err := fsutil.ReplaceFile(path, data); err != nil {
		return fmt.Errorf("replace session config %q: %w", path, err)
	}
	return nil
}

func FindConfig(directory, id string) (Config, error) {
	if !idPattern.MatchString(id) {
		return Config{}, fmt.Errorf("invalid session id %q", id)
	}
	config, err := LoadConfig(filepath.Join(directory, id+".yaml"))
	if err != nil {
		return Config{}, err
	}
	if config.ID != id {
		return Config{}, fmt.Errorf("session config filename %q does not match declared id %q", id, config.ID)
	}
	return config, nil
}

type ConfigDeletion struct {
	original  string
	tombstone string
	unlock    func()
	once      sync.Once
}

func (deletion *ConfigDeletion) release() {
	deletion.once.Do(func() {
		if deletion.unlock != nil {
			deletion.unlock()
		}
	})
}

// StageConfigDeletion moves a session definition out of the active YAML set
// while retaining it for rollback or startup recovery.
func StageConfigDeletion(directory, id string) (*ConfigDeletion, error) {
	if !idPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid session id %q", id)
	}
	original := filepath.Join(directory, id+".yaml")
	unlock, err := lockConfigPath(original)
	if err != nil {
		return nil, fmt.Errorf("lock session config %q: %w", original, err)
	}
	tombstone := original + ".deleting"
	if _, err := os.Lstat(tombstone); err == nil {
		unlock()
		return nil, fmt.Errorf("session %q already has a pending deletion", id)
	} else if !errors.Is(err, os.ErrNotExist) {
		unlock()
		return nil, fmt.Errorf("inspect pending session deletion %q: %w", id, err)
	}
	if err := os.Rename(original, tombstone); err != nil {
		unlock()
		return nil, fmt.Errorf("stage session config deletion %q: %w", id, err)
	}
	return &ConfigDeletion{original: original, tombstone: tombstone, unlock: unlock}, nil
}

func (deletion *ConfigDeletion) Restore() error {
	defer deletion.release()
	if _, err := os.Lstat(deletion.tombstone); errors.Is(err, os.ErrNotExist) {
		if _, originalErr := os.Lstat(deletion.original); originalErr == nil {
			return nil
		} else if errors.Is(originalErr, os.ErrNotExist) {
			return fmt.Errorf("cannot restore session config: both %q and %q are missing", deletion.original, deletion.tombstone)
		} else {
			return fmt.Errorf("inspect original session config %q: %w", deletion.original, originalErr)
		}
	} else if err != nil {
		return fmt.Errorf("inspect staged session config: %w", err)
	}
	if _, err := os.Lstat(deletion.original); err == nil {
		return fmt.Errorf("cannot restore staged session config %q because the original path exists", deletion.original)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect original session config %q: %w", deletion.original, err)
	}
	if err := os.Rename(deletion.tombstone, deletion.original); err != nil {
		return fmt.Errorf("restore staged session config %q: %w", deletion.original, err)
	}
	return nil
}

func (deletion *ConfigDeletion) Finalize() error {
	defer deletion.release()
	if _, err := os.Lstat(deletion.tombstone); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect staged session config %q: %w", deletion.tombstone, err)
	}
	if err := os.Remove(deletion.tombstone); err != nil {
		return fmt.Errorf("remove staged session config %q: %w", deletion.original, err)
	}
	return nil
}

// RecoverPendingConfigDeletions resolves staged YAML files against the
// database journal: committed deletions are finalized; uncommitted stages are
// restored. It never overwrites an active YAML file.
func RecoverPendingConfigDeletions(directory string, store *Store) error {
	ctx := context.Background()
	pendingIDs, err := store.PendingDeletions(ctx)
	if err != nil {
		return err
	}
	pending := make(map[string]bool, len(pendingIDs))
	for _, id := range pendingIDs {
		if !idPattern.MatchString(id) {
			return fmt.Errorf("invalid session ID %q in deletion journal", id)
		}
		pending[id] = true
	}
	entries, err := os.ReadDir(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read session directory for deletion recovery: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml.deleting") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".yaml.deleting")
		if !idPattern.MatchString(id) {
			return fmt.Errorf("invalid staged session deletion filename %q", entry.Name())
		}
		if pending[id] {
			continue
		}
		original := filepath.Join(directory, id+".yaml")
		unlock, err := lockConfigPath(original)
		if err != nil {
			return fmt.Errorf("lock session config %q: %w", original, err)
		}
		deletion := &ConfigDeletion{
			original:  original,
			tombstone: filepath.Join(directory, entry.Name()),
			unlock:    unlock,
		}
		if err := deletion.Restore(); err != nil {
			return err
		}
	}
	for _, id := range pendingIDs {
		original := filepath.Join(directory, id+".yaml")
		unlock, err := lockConfigPath(original)
		if err != nil {
			return fmt.Errorf("lock session config %q: %w", original, err)
		}
		deletion := &ConfigDeletion{
			original:  original,
			tombstone: filepath.Join(directory, id+".yaml.deleting"),
			unlock:    unlock,
		}
		if err := deletion.Finalize(); err != nil {
			return err
		}
		if err := store.CompleteDeletion(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func ListConfigs(directory string) ([]Config, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return []Config{}, nil
		}
		return nil, fmt.Errorf("read session config directory %q: %w", directory, err)
	}
	configs := make([]Config, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		config, err := LoadConfig(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		if filepath.Base(entry.Name()) != config.ID+".yaml" {
			return nil, fmt.Errorf("session config filename %q must be %q", entry.Name(), config.ID+".yaml")
		}
		configs = append(configs, config)
	}
	return configs, nil
}
