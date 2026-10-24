package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Harness string

const (
	ClaudeCode Harness = "claude-code"
	CopilotCLI Harness = "copilot-cli"
)

var (
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type Config struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	Tool        Harness           `yaml:"tool"`
	WorkingDir  string            `yaml:"working_dir"`
	LaunchArgs  []string          `yaml:"launch_args,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	PowerShell  struct {
		UTF8 *bool `yaml:"utf8"`
	} `yaml:"powershell"`
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

	var config Config
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode session config %q: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return Config{}, fmt.Errorf("decode trailing session YAML in %q: %w", path, err)
		}
		return Config{}, fmt.Errorf("session config %q must contain one YAML document", path)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate session config %q: %w", path, err)
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
