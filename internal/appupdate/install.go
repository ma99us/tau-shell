package appupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxZipBytes caps the release ZIP download.
	MaxZipBytes  = 150 << 20
	maxSumsBytes = 64 << 10
	maxExtracted = 400 << 20

	// UpdateDir and RollbackDir live inside the install folder so renames stay on one volume.
	UpdateDir   = "_update"
	RollbackDir = "_rollback"
	exeName     = "taushell.exe"
)

// Progress reports download progress; total is -1 when unknown.
type Progress func(done, total int64)

func fetch(ctx context.Context, client *http.Client, rawURL string, limit int64, sink io.Writer, progress Progress) error {
	if !strings.HasPrefix(rawURL, DownloadPrefix) {
		return fmt.Errorf("refusing to download from %q", rawURL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "TauShell-update")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return fmt.Errorf("download is larger than the %d byte limit", limit)
	}
	var done int64
	buffer := make([]byte, 64<<10)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			done += int64(count)
			if done > limit {
				return fmt.Errorf("download is larger than the %d byte limit", limit)
			}
			if _, err := sink.Write(buffer[:count]); err != nil {
				return err
			}
			if progress != nil {
				progress(done, response.ContentLength)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("download: %w", readErr)
		}
	}
}

// Download fetches the release ZIP into dir and verifies it against
// SHA256SUMS.txt. The ZIP is deleted when verification fails.
func Download(ctx context.Context, client *http.Client, release Release, dir string, progress Progress) (string, error) {
	if release.ZipURL == "" || release.SumsURL == "" {
		return "", errors.New("release has no TauShell ZIP or SHA256SUMS.txt asset")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var sums strings.Builder
	if err := fetch(ctx, client, release.SumsURL, maxSumsBytes, &sums, nil); err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}
	zipName := release.ZipURL[strings.LastIndex(release.ZipURL, "/")+1:]
	want, ok := checksumFor(sums.String(), zipName)
	if !ok {
		return "", fmt.Errorf("SHA256SUMS.txt has no entry for %s", zipName)
	}
	zipPath := filepath.Join(dir, zipName)
	file, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	err = fetch(ctx, client, release.ZipURL, MaxZipBytes, io.MultiWriter(file, hasher), progress)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil && !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), want) {
		err = errors.New("downloaded file does not match its SHA-256 checksum")
	}
	if err != nil {
		_ = os.Remove(zipPath)
		return "", err
	}
	return zipPath, nil
}

func checksumFor(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name && len(fields[0]) == 64 {
			return fields[0], true
		}
	}
	return "", false
}

// Extract unpacks the release ZIP into dir. Entries that would escape dir or
// are not regular files are refused, and taushell.exe must be present.
func Extract(zipPath, dir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open update archive: %w", err)
	}
	defer reader.Close()
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	var total int64
	for _, entry := range reader.File {
		target := filepath.Join(root, filepath.FromSlash(entry.Name))
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("update archive entry %q escapes the target folder", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			return fmt.Errorf("update archive entry %q is not a regular file", entry.Name)
		}
		total += int64(entry.UncompressedSize64)
		if total > maxExtracted {
			return errors.New("update archive expands beyond the size limit")
		}
		if err := extractFile(entry, target); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(root, exeName)); err != nil {
		return fmt.Errorf("update archive has no %s", exeName)
	}
	return nil
}

func extractFile(entry *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(source, int64(entry.UncompressedSize64)+1)); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

type move struct {
	target string
	backup string
}

func stagedFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, relative)
		}
		return nil
	})
	return files, err
}

// Install moves every file of stagedDir over installDir. Replaced files are
// renamed into backupDir first (a running exe may be renamed but not
// overwritten). On any error the changes are undone and the error returned.
func Install(installDir, stagedDir, backupDir string) error {
	files, err := stagedFiles(stagedDir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return err
	}
	var done []move
	undo := func() {
		for index := len(done) - 1; index >= 0; index-- {
			step := done[index]
			_ = os.Remove(step.target)
			if step.backup != "" {
				_ = os.Rename(step.backup, step.target)
			}
		}
		_ = os.RemoveAll(backupDir)
	}
	for _, relative := range files {
		target := filepath.Join(installDir, relative)
		step := move{target: target}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			undo()
			return err
		}
		if _, statErr := os.Stat(target); statErr == nil {
			backup := filepath.Join(backupDir, relative)
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				undo()
				return err
			}
			if err := os.Rename(target, backup); err != nil {
				undo()
				return fmt.Errorf("set aside %s: %w", relative, err)
			}
			step.backup = backup
		}
		if err := os.Rename(filepath.Join(stagedDir, relative), target); err != nil {
			if step.backup != "" {
				_ = os.Rename(step.backup, target)
			}
			undo()
			return fmt.Errorf("install %s: %w", relative, err)
		}
		done = append(done, step)
	}
	return nil
}

// Apply installs the staged update and keeps the replaced files in
// _rollback\<fromVersion>, dropping any older rollback copy.
func Apply(installDir, stagedDir, fromVersion string) error {
	root := filepath.Join(installDir, RollbackDir)
	backup := filepath.Join(root, safeName(fromVersion))
	if err := Install(installDir, stagedDir, backup); err != nil {
		return err
	}
	return pruneRollbacks(root, filepath.Base(backup))
}

func pruneRollbacks(root, keep string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != keep {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func safeName(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	var out strings.Builder
	for _, r := range version {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '.' || r == '-' {
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "previous"
	}
	return out.String()
}

// RollbackVersion returns the version kept for rollback, if any.
func RollbackVersion(installDir string) (string, bool) {
	entries, err := os.ReadDir(filepath.Join(installDir, RollbackDir))
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := os.Stat(filepath.Join(installDir, RollbackDir, entry.Name(), exeName)); err == nil {
				return entry.Name(), true
			}
		}
	}
	return "", false
}

// Rollback restores the files kept by the last update and keeps the current
// ones as the new rollback copy.
func Rollback(installDir, currentVersion string) error {
	previous, ok := RollbackVersion(installDir)
	if !ok {
		return errors.New("no previous version is available to roll back to")
	}
	root := filepath.Join(installDir, RollbackDir)
	staged := filepath.Join(installDir, UpdateDir, "rollback")
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(root, previous), staged); err != nil {
		return err
	}
	current := filepath.Join(root, safeName(currentVersion))
	if err := Install(installDir, staged, current); err != nil {
		_ = os.Rename(staged, filepath.Join(root, previous))
		return err
	}
	_ = os.RemoveAll(staged)
	return pruneRollbacks(root, filepath.Base(current))
}

// Result is handed from the old process to the new one across the restart.
type Result struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Sessions []string `json:"sessions"`
}

// WriteResult records an update outcome for the next start.
func WriteResult(path string, result Result) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// TakeResult reads and deletes the recorded result; nil when there is none.
func TakeResult(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	removeErr := os.Remove(path)
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse update result: %w", err)
	}
	return &result, removeErr
}
