//go:build windows

package main

import (
	"errors"
	"sort"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/vault"
)

// SecretInfo describes one vault secret. It never carries the value.
type SecretInfo struct {
	Name   string   `json:"name"`
	UsedBy []string `json:"usedBy"`
	// Missing marks a secret a session references that the vault does not hold.
	Missing bool `json:"missing"`
}

// ListSecrets returns every stored secret and every referenced one, by name,
// with the sessions that reference them.
func (application *App) ListSecrets() ([]SecretInfo, error) {
	names, err := application.vault.Names()
	if err != nil {
		diagnostics.Error("list secrets failed", err)
		return nil, err
	}
	configs, err := session.ListConfigs(application.configDirectory)
	if err != nil {
		return nil, err
	}
	byName := map[string]*SecretInfo{}
	for _, name := range names {
		byName[name] = &SecretInfo{Name: name, UsedBy: []string{}}
	}
	for _, config := range configs {
		for _, secret := range config.Secrets {
			info := byName[secret]
			if info == nil {
				info = &SecretInfo{Name: secret, UsedBy: []string{}, Missing: true}
				byName[secret] = info
			}
			info.UsedBy = append(info.UsedBy, config.Name)
		}
	}
	result := make([]SecretInfo, 0, len(byName))
	for _, info := range byName {
		sort.Strings(info.UsedBy)
		result = append(result, *info)
	}
	sort.Slice(result, func(a, b int) bool { return result[a].Name < result[b].Name })
	return result, nil
}

// SetSecret stores or replaces a secret. Failures never include the value.
func (application *App) SetSecret(name, value string) error {
	if err := application.vault.Set(name, value); err != nil {
		diagnostics.Error("store secret failed", err, "secret", name)
		return err
	}
	return nil
}

// DeleteSecret removes a secret. Sessions that reference it will refuse to
// start until it is added again.
func (application *App) DeleteSecret(name string) error {
	if err := application.vault.Delete(name); err != nil && !errors.Is(err, vault.ErrNotFound) {
		diagnostics.Error("delete secret failed", err, "secret", name)
		return err
	}
	return nil
}
