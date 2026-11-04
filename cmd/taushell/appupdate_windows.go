//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ma99us/tau-shell/internal/appupdate"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const updateProgressEvent = "app-update-progress"

var updateMu sync.Mutex

func updateResultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tau_shell", "update-result.json"), nil
}

func installFolder() (string, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", "", err
	}
	return filepath.Dir(exe), exe, nil
}

// installBlocker explains why the app cannot update itself, or returns "".
func installBlocker() string {
	if !appupdate.Comparable(version) {
		return "This is a development build."
	}
	dir, exe, err := installFolder()
	if err != nil {
		return "TauShell could not locate its install folder."
	}
	probe, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return "The install folder is not writable; update manually from the release page."
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	if others := otherInstances(exe); others > 0 {
		return "Another TauShell is running from this folder; close it first."
	}
	return ""
}

// otherInstances counts other processes running the same executable.
func otherInstances(exe string) int {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	count := 0
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if int(entry.ProcessID) == os.Getpid() || !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(exe)) {
			continue
		}
		if sameImage(entry.ProcessID, exe) {
			count++
		}
	}
	return count
}

func sameImage(pid uint32, exe string) bool {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, windows.MAX_PATH*2)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return false
	}
	return strings.EqualFold(windows.UTF16ToString(buffer[:size]), exe)
}

func (application *App) emitUpdateProgress(phase string, done, total int64) {
	if application.ctx != nil {
		runtime.EventsEmit(application.ctx, updateProgressEvent, map[string]any{"phase": phase, "done": done, "total": total})
	}
}

// GetUpdateResult returns the outcome of the previous update or rollback,
// once, and removes leftover staging files.
func (application *App) GetUpdateResult() *appupdate.Result {
	if dir, _, err := installFolder(); err == nil {
		_ = os.RemoveAll(filepath.Join(dir, appupdate.UpdateDir))
	}
	path, err := updateResultPath()
	if err != nil {
		return nil
	}
	result, err := appupdate.TakeResult(path)
	if err != nil {
		diagnostics.Warn("read update result failed", err)
	}
	return result
}

// GetRollbackVersion returns the version kept for rollback, or an empty string.
func (application *App) GetRollbackVersion() string {
	dir, _, err := installFolder()
	if err != nil {
		return ""
	}
	kept, _ := appupdate.RollbackVersion(dir)
	return kept
}

// InstallAppUpdate downloads, verifies and installs the newest release, then
// restarts TauShell. It returns only when something went wrong; the old
// version stays in place in that case.
func (application *App) InstallAppUpdate() (err error) {
	if !updateMu.TryLock() {
		return errors.New("an update is already in progress")
	}
	defer updateMu.Unlock()
	defer func() {
		if err != nil {
			diagnostics.Error("app update failed", err)
			application.emitUpdateProgress("failed", 0, 0)
		}
	}()
	if reason := installBlocker(); reason != "" {
		return errors.New(reason)
	}
	ctx := context.Background()
	release, ok, err := appupdate.Latest(ctx, http.DefaultClient, appupdate.ReleasesAPI)
	if err != nil {
		return err
	}
	if !ok || !appupdate.Newer(version, release.Tag) {
		return errors.New("TauShell is already up to date")
	}
	dir, exe, err := installFolder()
	if err != nil {
		return err
	}
	stage := filepath.Join(dir, appupdate.UpdateDir, strings.TrimPrefix(release.Tag, "v"))
	_ = os.RemoveAll(filepath.Join(dir, appupdate.UpdateDir))
	defer os.RemoveAll(filepath.Join(dir, appupdate.UpdateDir))

	application.emitUpdateProgress("downloading", 0, -1)
	last := time.Time{}
	zipPath, err := appupdate.Download(ctx, &http.Client{Timeout: 10 * time.Minute}, release, stage, func(done, total int64) {
		if time.Since(last) > 150*time.Millisecond {
			last = time.Now()
			application.emitUpdateProgress("downloading", done, total)
		}
	})
	if err != nil {
		return err
	}
	application.emitUpdateProgress("verifying", 0, 0)
	unpacked := filepath.Join(stage, "files")
	if err := appupdate.Extract(zipPath, unpacked); err != nil {
		return err
	}

	sessions, err := application.manager.List(ctx)
	if err != nil {
		return fmt.Errorf("check active sessions: %w", err)
	}
	running := make([]string, 0)
	for _, current := range sessions {
		if current.RunningHere {
			running = append(running, current.ID)
		}
	}
	if application.confirmShutdown(application.ctx) {
		return errors.New("update cancelled: running sessions were not stopped")
	}

	application.emitUpdateProgress("installing", 0, 0)
	resultPath, err := updateResultPath()
	if err != nil {
		return err
	}
	if err := appupdate.WriteResult(resultPath, appupdate.Result{From: version, To: strings.TrimPrefix(release.Tag, "v"), Sessions: running}); err != nil {
		return err
	}
	if err := backupState(version); err != nil {
		_ = os.Remove(resultPath)
		return err
	}
	if err := appupdate.Apply(dir, unpacked, version); err != nil {
		_ = os.Remove(resultPath)
		return err
	}
	return application.restartInto(dir, exe, resultPath)
}

// RollbackAppUpdate swaps the previous version back in and restarts.
func (application *App) RollbackAppUpdate() (err error) {
	if !updateMu.TryLock() {
		return errors.New("an update is already in progress")
	}
	defer updateMu.Unlock()
	defer func() {
		if err != nil {
			diagnostics.Error("app rollback failed", err)
		}
	}()
	if reason := installBlocker(); reason != "" {
		return errors.New(reason)
	}
	dir, exe, err := installFolder()
	if err != nil {
		return err
	}
	previous, ok := appupdate.RollbackVersion(dir)
	if !ok {
		return errors.New("no previous version is available")
	}
	sessions, err := application.manager.List(context.Background())
	if err != nil {
		return fmt.Errorf("check active sessions: %w", err)
	}
	running := make([]string, 0)
	for _, current := range sessions {
		if current.RunningHere {
			running = append(running, current.ID)
		}
	}
	if application.confirmShutdown(application.ctx) {
		return errors.New("rollback cancelled: running sessions were not stopped")
	}
	resultPath, err := updateResultPath()
	if err != nil {
		return err
	}
	if err := appupdate.WriteResult(resultPath, appupdate.Result{From: version, To: previous, Sessions: running}); err != nil {
		return err
	}
	if err := backupState(version); err != nil {
		_ = os.Remove(resultPath)
		return err
	}
	if err := appupdate.Rollback(dir, version); err != nil {
		_ = os.Remove(resultPath)
		return err
	}
	return application.restartInto(dir, exe, resultPath)
}

// backupState keeps a copy of the state database before the app files change.
func backupState(label string) error {
	_, databasePath, err := appPaths()
	if err != nil {
		return err
	}
	dest, err := appupdate.BackupState(databasePath, filepath.Join(filepath.Dir(databasePath), "backups"), label)
	if err != nil {
		return fmt.Errorf("back up session state before the update: %w", err)
	}
	diagnostics.Info("session state backed up", "path", dest)
	return nil
}

// restartInto starts the freshly installed executable and quits this process.
// If it cannot be started the previous files are put back.
func (application *App) restartInto(dir, exe, resultPath string) error {
	application.emitUpdateProgress("restarting", 0, 0)
	command := exec.Command(exe)
	command.Dir = dir
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		_ = os.Remove(resultPath)
		if rollbackErr := appupdate.Rollback(dir, "failed"); rollbackErr != nil {
			return fmt.Errorf("start the updated TauShell: %w (restoring the old version also failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("start the updated TauShell: %w; the previous version was restored", err)
	}
	_ = command.Process.Release()
	application.shutdownMu.Lock()
	application.allowShutdown = true
	application.shutdownMu.Unlock()
	application.requestApplicationQuit(application.ctx)
	return nil
}
