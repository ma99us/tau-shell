// Package appconfig loads the hand-edited, app-wide TauShell settings file.
package appconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/ma99us/tau-shell/internal/skills"
	"gopkg.in/yaml.v3"
)

const FileName = "config.yaml"

// Notifications controls local attention alerts. A nil flag means enabled.
type Notifications struct {
	Toast        *bool `yaml:"toast,omitempty"`
	TaskbarFlash *bool `yaml:"taskbar_flash,omitempty"`
}

// Control configures the local control channel. Both flags default to off.
type Control struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	Agents  *bool `yaml:"agents,omitempty"`
}

func (c Control) ChannelEnabled() bool { return c.Enabled != nil && *c.Enabled }

// AgentsEnabled reports whether programs inside sessions may use the channel.
func (c Control) AgentsEnabled() bool { return c.ChannelEnabled() && c.Agents != nil && *c.Agents }

// Mobile configures the phone companion server. It is off by default and never
// listens on all interfaces: Interface must name one local IP address.
type Mobile struct {
	Enabled   *bool  `yaml:"enabled,omitempty"`
	Interface string `yaml:"interface,omitempty"`
	Port      int    `yaml:"port,omitempty"`
	// Screen lets paired phones read the visible terminal text of running
	// sessions. On by default; set false to keep terminal text on the PC.
	Screen *bool `yaml:"screen,omitempty"`
	// Presets are quick replies offered on every paired phone, added to the ones
	// the user keeps on the phone.
	Presets []string `yaml:"presets,omitempty"`
}

func (m Mobile) ServerEnabled() bool { return m.Enabled != nil && *m.Enabled }

// ShareScreen reports whether paired phones may read terminal text.
func (m Mobile) ShareScreen() bool { return m.Screen == nil || *m.Screen }

// DefaultMobilePort is used when mobile.port is not set.
const DefaultMobilePort = 47821

// ListenPort returns the configured port or the default.
func (m Mobile) ListenPort() int {
	if m.Port == 0 {
		return DefaultMobilePort
	}
	return m.Port
}

func (m Mobile) validate() error {
	if m.Port != 0 && (m.Port < 1024 || m.Port > 65535) {
		return fmt.Errorf("mobile.port must be between 1024 and 65535, got %d", m.Port)
	}
	if m.Interface != "" {
		ip := net.ParseIP(m.Interface)
		if ip == nil || ip.IsUnspecified() {
			return fmt.Errorf("mobile.interface %q must be one local IP address, not all interfaces", m.Interface)
		}
	}
	return nil
}

type Config struct {
	Version       int           `yaml:"version"`
	Control       Control       `yaml:"control"`
	Mobile        Mobile        `yaml:"mobile"`
	Notifications Notifications `yaml:"notifications"`
	Usage         Usage         `yaml:"usage"`
	Quota         Quota         `yaml:"quota"`
	Skills        Skills        `yaml:"skills"`
}

// Quota controls the Home screen's Copilot premium-request chip, which reads
// the quota through the GitHub CLI. A nil flag means enabled.
type Quota struct {
	Copilot *bool `yaml:"copilot,omitempty"`
}

func (q Quota) CopilotEnabled() bool { return q.Copilot == nil || *q.Copilot }

// Skills controls the skill library TauShell loads into harness sessions.
// Repo is a pointer so an explicit empty string can turn fetching off.
type Skills struct {
	Enabled      *bool    `yaml:"enabled,omitempty"`
	Repo         *string  `yaml:"repo,omitempty"`
	Ref          string   `yaml:"ref,omitempty"`
	AutoUpdate   bool     `yaml:"auto_update,omitempty"`
	LocalSources []string `yaml:"local_sources,omitempty"`
}

// Resolve applies defaults and returns the library configuration.
func (s Skills) Resolve() skills.Config {
	config := skills.DefaultConfig()
	if s.Enabled != nil {
		config.Enabled = *s.Enabled
	}
	if s.Repo != nil {
		config.Repo = *s.Repo
	}
	if s.Ref != "" {
		config.Ref = s.Ref
	}
	config.AutoUpdate = s.AutoUpdate
	config.LocalSources = s.LocalSources
	return config
}

func (s Skills) validate() error {
	if s.Repo != nil && *s.Repo != "" {
		if _, _, err := skills.ParseRepo(*s.Repo); err != nil {
			return err
		}
	}
	if s.Ref != "" && !skills.ValidRef(s.Ref) {
		return fmt.Errorf("skills.ref %q is not a safe ref name", s.Ref)
	}
	for _, dir := range s.LocalSources {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("skills.local_sources entry %q must be an absolute path", dir)
		}
	}
	return nil
}

// PriceOverride is a hand-set Claude price in USD per million tokens. Omitted
// cache prices default to the standard multiples of the input price.
type PriceOverride struct {
	Input        float64 `yaml:"input"`
	CacheWrite5m float64 `yaml:"cache_write_5m,omitempty"`
	CacheWrite1h float64 `yaml:"cache_write_1h,omitempty"`
	CacheRead    float64 `yaml:"cache_read,omitempty"`
	Output       float64 `yaml:"output"`
}

// Usage controls token and cost tracking.
type Usage struct {
	MonthStartDay int                      `yaml:"month_start_day,omitempty"`
	PriceUpdates  *bool                    `yaml:"price_updates,omitempty"`
	ClaudePrices  map[string]PriceOverride `yaml:"claude_prices,omitempty"`
}

// StartDay is the day of the month the monthly totals reset; default the 1st.
func (u Usage) StartDay() int {
	if u.MonthStartDay == 0 {
		return 1
	}
	return u.MonthStartDay
}

// PriceUpdatesEnabled reports whether Claude prices are refreshed from the web.
func (u Usage) PriceUpdatesEnabled() bool { return u.PriceUpdates == nil || *u.PriceUpdates }

func (u Usage) validate() error {
	if u.MonthStartDay != 0 && (u.MonthStartDay < 1 || u.MonthStartDay > 28) {
		return fmt.Errorf("usage.month_start_day must be between 1 and 28, got %d", u.MonthStartDay)
	}
	for model, price := range u.ClaudePrices {
		if price.Input <= 0 || price.Output <= 0 || price.CacheWrite5m < 0 || price.CacheWrite1h < 0 || price.CacheRead < 0 {
			return fmt.Errorf("usage.claude_prices.%s needs positive input and output prices", model)
		}
	}
	return nil
}

func (n Notifications) ToastEnabled() bool        { return n.Toast == nil || *n.Toast }
func (n Notifications) TaskbarFlashEnabled() bool { return n.TaskbarFlash == nil || *n.TaskbarFlash }

// Load reads the settings file. A missing file means defaults; invalid
// content is an error so typos are never silently ignored.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read settings: %w", err)
	}
	var parsed Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if parsed.Version != 0 && parsed.Version != 1 {
		return Config{}, fmt.Errorf("%s: unsupported version %d", filepath.Base(path), parsed.Version)
	}
	if err := parsed.Usage.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if err := parsed.Skills.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if err := parsed.Mobile.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return parsed, nil
}

const exampleFile = `# TauShell settings. Edit and save; changes apply to the next alert.
version: 1

control:
  # Let "taushell ctl" (run as you, on this PC) read and drive running sessions
  # through a private pipe. Off by default; applies after TauShell restarts.
  # enabled: false
  # Also let programs running inside sessions use it (reserved; not active yet).
  # agents: false

mobile:
  # Let a paired Android phone list and drive sessions over TLS on your LAN.
  # Off by default; applies after TauShell restarts. Never listens on all interfaces.
  # enabled: false
  # The one local IP address to listen on (required when enabled), for example 192.168.1.20.
  # interface: ""
  # TCP port, 1024-65535.
  # port: 47821

notifications:
  # Windows toast when a session needs your answer or finishes while TauShell is in the background.
  toast: true
  # Flash the TauShell taskbar button for the same events.
  taskbar_flash: true

usage:
  # Day of the month (1-28) the monthly token and cost totals reset.
  # month_start_day: 1
  # Refresh Claude prices from Anthropic's pricing page every 24 hours.
  # price_updates: true
  # Override Claude prices (USD per million tokens). Cache prices are optional.
  # claude_prices:
  #   opus-5.5: { input: 4, output: 20, cache_write_5m: 5, cache_write_1h: 8, cache_read: 0.2 }

quota:
  # Show your Copilot premium-request usage and month-end projection on Home.
  # Runs "gh api /copilot_internal/user" with your own GitHub CLI sign-in; set
  # false to stop all quota calls. The chip is hidden if gh is missing or signed out.
  # copilot: true

skills:
  # Load TauShell's skill library (from the ai-cookbook) into Claude Code and Copilot CLI sessions.
  # enabled: true
  # GitHub repository checked for newer skills (public, https only); "" turns all fetching off.
  # repo: https://github.com/mgerdov-igt/ai-cookbook
  # ref: main
  # Apply new commits automatically instead of waiting for the Update skills button.
  # auto_update: false
  # Developer overrides: absolute paths to ai-cookbook checkouts, re-read at every launch.
  # local_sources:
  #   - C:\path\to\ai-cookbook
`

// EnsureExample creates the commented example file only when none exists.
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
