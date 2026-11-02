package appconfig

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ma99us/tau-shell/internal/fsutil"
)

var (
	enabledLine = regexp.MustCompile(`^(\s+)enabled:\s*\S*(\s*#.*)?$`)
	versionLine = regexp.MustCompile(`^version:`)
)

// SetControlEnabled stores control.enabled in the settings file by editing it
// in place, so the user's comments and other settings survive. A missing file
// is created from the commented example. An invalid file is left untouched.
func SetControlEnabled(path string, enabled bool) error {
	return setSectionEnabled(path, "control", enabled, func(c Config) bool { return c.Control.ChannelEnabled() })
}

// SetMobileEnabled stores mobile.enabled the same way.
func SetMobileEnabled(path string, enabled bool) error {
	return setSectionEnabled(path, "mobile", enabled, func(c Config) bool { return c.Mobile.ServerEnabled() })
}

func setSectionEnabled(path, section string, enabled bool, read func(Config) bool) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := EnsureExample(path); err != nil {
			return fmt.Errorf("create settings: %w", err)
		}
		if data, err = os.ReadFile(path); err != nil {
			return fmt.Errorf("read settings: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if _, err := Load(path); err != nil {
		return fmt.Errorf("not changing an invalid settings file: %w", err)
	}
	updated := setSectionEnabledText(string(data), section, enabled)
	if err := fsutil.ReplaceFile(path, []byte(updated)); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if config, err := Load(path); err != nil || read(config) != enabled {
		return fmt.Errorf("settings did not keep %s.enabled=%t (%v)", section, enabled, err)
	}
	return nil
}

func setSectionEnabledText(text, section string, enabled bool) string {
	sectionLine := regexp.MustCompile(`^` + regexp.QuoteMeta(section) + `:\s*(#.*)?$`)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	value := fmt.Sprintf("%t", enabled)
	lines := strings.Split(text, newline)
	for index, line := range lines {
		if !sectionLine.MatchString(line) {
			continue
		}
		for next := index + 1; next < len(lines); next++ {
			current := lines[next]
			if current != "" && current[0] != ' ' && current[0] != '\t' && current[0] != '#' {
				break
			}
			if match := enabledLine.FindStringSubmatch(current); match != nil {
				lines[next] = match[1] + "enabled: " + value + match[2]
				return strings.Join(lines, newline)
			}
		}
		lines = append(lines[:index+1], append([]string{"  enabled: " + value}, lines[index+1:]...)...)
		return strings.Join(lines, newline)
	}
	block := []string{section + ":", "  enabled: " + value, ""}
	for index, line := range lines {
		if versionLine.MatchString(line) {
			lines = append(lines[:index+1], append(append([]string{""}, block[:2]...), lines[index+1:]...)...)
			return strings.Join(lines, newline)
		}
	}
	return "version: 1" + newline + strings.Join(block[:2], newline) + newline + text
}
