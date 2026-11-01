package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/ma99us/tau-shell/internal/fsutil"
	"github.com/ma99us/tau-shell/internal/presets"
)

type Harness string

const (
	ClaudeCode Harness = "claude-code"
	CopilotCLI Harness = "copilot-cli"
	GSD        Harness = "gsd"
	Codex      Harness = "codex"
	OpenCode   Harness = "opencode"
)

// SupportedHarnesses lists every harness ID in display order.
var SupportedHarnesses = []Harness{ClaudeCode, CopilotCLI, GSD, Codex, OpenCode}

func (harness Harness) Supported() bool {
	for _, candidate := range SupportedHarnesses {
		if candidate == harness {
			return true
		}
	}
	return false
}

var (
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	colorPattern       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	ErrConfigChanged   = errors.New("session config changed outside TauShell; reload and retry")
)

const maxNameLength = 80

type Config struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Color string `yaml:"color,omitempty"`
	// AutoResume resumes this session when TauShell starts, if it has a saved
	// harness conversation. The Home tile checkbox edits it.
	AutoResume bool `yaml:"auto_resume,omitempty"`
	// Skills set to false keeps TauShell's skill library out of this session.
	Skills      *bool             `yaml:"skills,omitempty"`
	Tool        Harness           `yaml:"tool"`
	WorkingDir  string            `yaml:"working_dir"`
	LaunchArgs  []string          `yaml:"launch_args,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	PowerShell  struct {
		UTF8 *bool `yaml:"utf8"`
	} `yaml:"powershell,omitempty"`
	// Presets are session-level prompt presets merged over the global
	// prompts.yaml entries by name.
	Presets []presets.Preset `yaml:"presets,omitempty"`
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
	if !config.UTF8Enabled() {
		return fmt.Errorf("powershell.utf8 cannot be disabled in the Windows 10 Phase 1 runtime")
	}
	if !config.Tool.Supported() {
		names := make([]string, len(SupportedHarnesses))
		for index, supported := range SupportedHarnesses {
			names[index] = string(supported)
		}
		return fmt.Errorf("unsupported tool %q (supported: %s)", config.Tool, strings.Join(names, ", "))
	}
	if config.WorkingDir == "" || !filepath.IsAbs(config.WorkingDir) {
		return fmt.Errorf("working_dir must be an absolute path")
	}
	info, err := os.Stat(config.WorkingDir)
	if err != nil {
		return fmt.Errorf("working_dir %q: %w", config.WorkingDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working_dir %q is not a directory", config.WorkingDir)
	}
	for name := range config.Environment {
		if !environmentPattern.MatchString(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
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
	reservedArgs := map[Harness]map[string]struct{}{
		ClaudeCode: {
			"--session-id": {},
			"--resume":     {},
		},
		CopilotCLI: {
			"--session-id": {},
			"--resume":     {},
		},
		GSD: {
			"--session":     {},
			"--session-dir": {},
			"--continue":    {},
			"-c":            {},
			"--no-session":  {},
		},
		Codex: {
			"resume": {},
			"fork":   {},
		},
		OpenCode: {
			"--session":  {},
			"-s":         {},
			"--continue": {},
			"-c":         {},
			"--fork":     {},
		},
	}
	for _, arg := range launchArgs {
		name, _, _ := strings.Cut(arg, "=")
		if _, reserved := reservedArgs[tool][strings.ToLower(name)]; reserved {
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

// UpdateConfigField rewrites one top-level field ("name", "color" or
// "auto_resume") of an existing session YAML, preserving other keys and
// comments. TauShell writers are serialized across processes; an external edit
// observed before replacement returns ErrConfigChanged. An empty color or
// auto_resume removes the key. The ID and filename never change.
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
	case "auto_resume":
		if value != "" && value != "true" {
			return Config{}, fmt.Errorf("auto_resume must be true or empty")
		}
	default:
		return Config{}, fmt.Errorf("unsupported session field %q", key)
	}
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
	setMappingString(document.Content[0], key, value)
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
