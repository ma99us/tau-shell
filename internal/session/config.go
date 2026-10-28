package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/ma99us/tau-shell/internal/presets"
)

type Harness string

const (
	ClaudeCode Harness = "claude-code"
	CopilotCLI Harness = "copilot-cli"
)

var (
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	colorPattern       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

const maxNameLength = 80

type Config struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	Color       string            `yaml:"color,omitempty"`
	// AutoResume resumes this session when TauShell starts, if it has a saved
	// harness conversation. The Home tile checkbox edits it.
	AutoResume  bool              `yaml:"auto_resume,omitempty"`
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
	if config.Tool != ClaudeCode && config.Tool != CopilotCLI {
		return fmt.Errorf("unsupported tool %q (supported: %s, %s)", config.Tool, ClaudeCode, CopilotCLI)
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
// "auto_resume") of an
// existing session YAML in place, preserving the user's other keys and
// comments. An empty color or auto_resume removes the key. The ID and filename never change.
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
	return updated, replaceFile(path, encoded.Bytes())
}

func setMappingString(mapping *yaml.Node, key, value string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		if value == "" {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
		mapping.Content[index+1] = stringNode(key, value)
		return
	}
	if value == "" {
		return
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, stringNode(key, value))
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

// replaceFile writes data beside path and renames it over path so a crash
// cannot leave a truncated session definition.
func replaceFile(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary session config: %w", err)
	}
	writeErr := func() error {
		if _, err := temp.Write(data); err != nil {
			return err
		}
		return temp.Sync()
	}()
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		_ = os.Remove(temp.Name())
		return fmt.Errorf("write temporary session config: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
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

func DeleteConfig(directory, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	if err := os.Remove(filepath.Join(directory, id+".yaml")); err != nil {
		return fmt.Errorf("delete session config %q: %w", id, err)
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
