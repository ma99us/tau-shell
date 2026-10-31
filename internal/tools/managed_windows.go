//go:build windows

package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var managedVersionPattern = regexp.MustCompile(`^[0-9][A-Za-z0-9.-]*$`)

type ManagedToolLayout struct {
	Root string
}

func ManagedToolLayoutFromEnvironment() (ManagedToolLayout, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return ManagedToolLayout{}, errors.New("LOCALAPPDATA is not set")
	}
	return ManagedToolLayout{Root: filepath.Join(localAppData, "TauShell", "tools")}, nil
}

func (layout ManagedToolLayout) NodeRoot() string  { return filepath.Join(layout.Root, "node") }
func (layout ManagedToolLayout) NpmPrefix() string { return filepath.Join(layout.Root, "npm") }
func (layout ManagedToolLayout) GitRoot() string   { return filepath.Join(layout.Root, "git") }

func (layout ManagedToolLayout) NodeVersionDir(version string) string {
	return filepath.Join(layout.NodeRoot(), version)
}

func (layout ManagedToolLayout) GitVersionDir(version string) string {
	return filepath.Join(layout.GitRoot(), version)
}

func (layout ManagedToolLayout) ActiveVersion(id ID) (string, error) {
	marker := ""
	switch id {
	case NodeJS:
		marker = filepath.Join(layout.Root, "node-version")
	case Git:
		marker = filepath.Join(layout.Root, "git-version")
	default:
		return "", fmt.Errorf("tool %q has no managed version marker", id)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	if !managedVersionPattern.MatchString(version) || version == "." || version == ".." {
		return "", fmt.Errorf("invalid managed %s version marker", id)
	}
	return version, nil
}

func (layout ManagedToolLayout) SetActiveVersion(id ID, version string) error {
	if id != NodeJS && id != Git {
		return fmt.Errorf("tool %q has no managed version marker", id)
	}
	if !managedVersionPattern.MatchString(version) || version == "." || version == ".." {
		return fmt.Errorf("invalid managed %s version %q", id, version)
	}
	if err := os.MkdirAll(layout.Root, 0o700); err != nil {
		return fmt.Errorf("create managed tools directory: %w", err)
	}
	marker := filepath.Join(layout.Root, strings.ToLower(string(id))+"-version")
	temporary, err := os.CreateTemp(layout.Root, ".active-version-*")
	if err != nil {
		return fmt.Errorf("create active version marker: %w", err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.WriteString(version + "\n"); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write active version marker: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close active version marker: %w", err)
	}
	if err := os.Rename(temporary.Name(), marker); err != nil {
		return fmt.Errorf("activate managed %s version: %w", id, err)
	}
	return nil
}

func (layout ManagedToolLayout) clearActiveVersion(id ID) error {
	if id != NodeJS && id != Git {
		return fmt.Errorf("tool %q has no managed version marker", id)
	}
	marker := filepath.Join(layout.Root, strings.ToLower(string(id))+"-version")
	if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (layout ManagedToolLayout) PathEntries() []string {
	var entries []string
	if version, err := layout.ActiveVersion(NodeJS); err == nil {
		directory := layout.NodeVersionDir(version)
		if exists(filepath.Join(directory, "node.exe")) && exists(filepath.Join(directory, "npm.cmd")) {
			if isDir(layout.NpmPrefix()) {
				entries = append(entries, layout.NpmPrefix())
			}
			entries = append(entries, directory)
		}
	}
	if version, err := layout.ActiveVersion(Git); err == nil {
		commandDirectory := filepath.Join(layout.GitVersionDir(version), "cmd")
		if exists(filepath.Join(commandDirectory, "git.exe")) {
			entries = append(entries, commandDirectory)
		}
	}
	return entries
}

func (layout ManagedToolLayout) ManagedRoots() []string {
	return []string{layout.NodeRoot(), layout.NpmPrefix(), layout.GitRoot()}
}

func (layout ManagedToolLayout) ProcessPath(current string) string {
	return MergePathEntries(strings.Join(layout.PathEntries(), ";"), current)
}

func (layout ManagedToolLayout) PrependToProcessPath() error {
	entries := layout.PathEntries()
	if len(entries) == 0 {
		return nil
	}
	if err := os.Setenv("PATH", MergePathEntries(strings.Join(entries, ";"), os.Getenv("PATH"))); err != nil {
		return err
	}
	if version, err := layout.ActiveVersion(NodeJS); err == nil &&
		exists(filepath.Join(layout.NodeVersionDir(version), "node.exe")) && exists(filepath.Join(layout.NodeVersionDir(version), "npm.cmd")) {
		return os.Setenv("NPM_CONFIG_PREFIX", layout.NpmPrefix())
	}
	return nil
}

func IsManagedPath(path string, roots []string) bool {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(absoluteRoot, absolutePath)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
