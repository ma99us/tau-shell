package appconfig

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/ma99us/tau-shell/internal/fsutil"
)

const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"

	MinFontScale        = 0.8
	MaxFontScale        = 1.6
	DefaultTerminalFont = 14
	MinTerminalFont     = 9
	MaxTerminalFont     = 28
)

// Appearance controls the theme and text sizes. Nil values mean the defaults
// (dark theme, 100% UI text, 14 px terminal text).
type Appearance struct {
	Theme            string   `yaml:"theme,omitempty"`
	FontScale        *float64 `yaml:"font_scale,omitempty"`
	TerminalFontSize *int     `yaml:"terminal_font_size,omitempty"`
}

// ResolvedAppearance is Appearance with every default applied.
type ResolvedAppearance struct {
	Theme            string  `json:"theme"`
	FontScale        float64 `json:"fontScale"`
	TerminalFontSize int     `json:"terminalFontSize"`
}

func validTheme(theme string) bool {
	return theme == ThemeDark || theme == ThemeLight || theme == ThemeSystem
}

func validFontScale(scale float64) bool {
	return !math.IsNaN(scale) && scale >= MinFontScale && scale <= MaxFontScale
}

func validTerminalFont(size int) bool { return size >= MinTerminalFont && size <= MaxTerminalFont }

func (a Appearance) validate() error {
	if a.Theme != "" && !validTheme(a.Theme) {
		return fmt.Errorf("appearance.theme %q must be dark, light or system", a.Theme)
	}
	if a.FontScale != nil && !validFontScale(*a.FontScale) {
		return fmt.Errorf("appearance.font_scale %v must be %.1f-%.1f", *a.FontScale, MinFontScale, MaxFontScale)
	}
	if a.TerminalFontSize != nil && !validTerminalFont(*a.TerminalFontSize) {
		return fmt.Errorf("appearance.terminal_font_size %d must be %d-%d", *a.TerminalFontSize, MinTerminalFont, MaxTerminalFont)
	}
	return nil
}

// Resolve applies the defaults.
func (a Appearance) Resolve() ResolvedAppearance {
	resolved := ResolvedAppearance{Theme: ThemeDark, FontScale: 1, TerminalFontSize: DefaultTerminalFont}
	if a.Theme != "" {
		resolved.Theme = a.Theme
	}
	if a.FontScale != nil {
		resolved.FontScale = *a.FontScale
	}
	if a.TerminalFontSize != nil {
		resolved.TerminalFontSize = *a.TerminalFontSize
	}
	return resolved
}

// SetAppearance stores the appearance settings by editing the file in place,
// so comments and other settings survive. An invalid file is left untouched.
func SetAppearance(path string, value ResolvedAppearance) error {
	value.FontScale = math.Round(value.FontScale*100) / 100
	if err := (Appearance{Theme: value.Theme, FontScale: &value.FontScale, TerminalFontSize: &value.TerminalFontSize}).validate(); err != nil {
		return err
	}
	if value.Theme == "" {
		return errors.New("appearance.theme is required")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := EnsureExample(path); err != nil {
			return fmt.Errorf("create settings: %w", err)
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if _, err := Load(path); err != nil {
		return fmt.Errorf("not changing an invalid settings file: %w", err)
	}
	text := string(data)
	text = setSectionKeyText(text, "appearance", "theme", value.Theme)
	text = setSectionKeyText(text, "appearance", "font_scale", strconv.FormatFloat(value.FontScale, 'f', -1, 64))
	text = setSectionKeyText(text, "appearance", "terminal_font_size", strconv.Itoa(value.TerminalFontSize))
	if err := fsutil.ReplaceFile(path, []byte(text)); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	config, err := Load(path)
	if err != nil || config.Appearance.Resolve() != value {
		return fmt.Errorf("settings did not keep appearance %+v (%v)", value, err)
	}
	return nil
}
