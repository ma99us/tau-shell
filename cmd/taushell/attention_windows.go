//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const (
	attentionInput = "input"
	attentionDone  = "done"

	flashwTray       = 0x2
	flashwTimerNoFG  = 0xC
	toastAppTitle    = "TauShell"
	focusSessionEvnt = "focus-session"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procFlashWindowEx            = user32.NewProc("FlashWindowEx")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
)

type flashInfo struct {
	Size     uint32
	Hwnd     windows.HWND
	Flags    uint32
	Count    uint32
	TimeoutM uint32
}

// attentionHooks isolates the OS effects so the decision logic can be tested.
type attentionHooks struct {
	foreground func() bool
	flash      func() error
	toast      func(title, body, sessionID string) error
}

type attentionState struct {
	mu          sync.Mutex
	toastReady  bool
	configError string
}

func (application *App) settingsPath() string {
	return filepath.Join(filepath.Dir(application.configDirectory), appconfig.FileName)
}

// attentionDecision returns which alerts to raise. Nothing is raised while the
// user is already looking at the session.
func attentionDecision(settings appconfig.Notifications, foreground bool) (flash, toast bool) {
	if foreground {
		return false, false
	}
	return settings.TaskbarFlashEnabled(), settings.ToastEnabled()
}

// attentionBody never includes paths or terminal content.
func attentionBody(sessionName, kind string) string {
	if kind == attentionInput {
		return sessionName + " is waiting for your answer"
	}
	return sessionName + " finished"
}

// NotifyAttention raises the configured background alerts for a session that
// needs the user or has finished.
func (application *App) NotifyAttention(sessionID, kind string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("attention notification failed", err, "session_id", sessionID, "kind", kind)
		}
	}()
	if kind != attentionInput && kind != attentionDone {
		return fmt.Errorf("unknown attention kind %q", kind)
	}
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil {
		application.attention.mu.Lock()
		changed := application.attention.configError != err.Error()
		application.attention.configError = err.Error()
		application.attention.mu.Unlock()
		if changed {
			diagnostics.Error("settings file is invalid; using defaults", err)
			application.emit("settings-error", err.Error())
		}
		settings = appconfig.Config{}
	} else {
		application.attention.mu.Lock()
		application.attention.configError = ""
		application.attention.mu.Unlock()
	}
	hooks := application.hooks()
	flash, toast := attentionDecision(settings.Notifications, hooks.foreground())
	config, findErr := session.FindConfig(application.configDirectory, sessionID)
	if findErr != nil {
		return findErr
	}
	var failures []error
	if flash {
		if flashErr := hooks.flash(); flashErr != nil {
			failures = append(failures, fmt.Errorf("taskbar flash: %w", flashErr))
		}
	}
	if toast {
		application.attention.mu.Lock()
		ready := application.attention.toastReady
		application.attention.mu.Unlock()
		if ready {
			if toastErr := hooks.toast(toastAppTitle, attentionBody(config.Name, kind), sessionID); toastErr != nil {
				failures = append(failures, fmt.Errorf("toast: %w", toastErr))
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%v", failures)
	}
	diagnostics.Info("attention alert raised", "session_id", sessionID, "kind", kind, "flash", flash, "toast", toast)
	return nil
}

func (application *App) emit(name string, data ...any) {
	if application.emitEvent != nil {
		application.emitEvent(name, data...)
		return
	}
	if application.ctx != nil {
		runtime.EventsEmit(application.ctx, name, data...)
	}
}

func (application *App) hooks() attentionHooks {
	if application.attentionHooks != nil {
		return *application.attentionHooks
	}
	return attentionHooks{
		foreground: windowIsForeground,
		flash:      flashMainWindow,
		toast: func(title, body, sessionID string) error {
			return runtime.SendNotification(application.ctx, runtime.NotificationOptions{
				ID:    fmt.Sprintf("taushell-%s-%d", sessionID, os.Getpid()),
				Title: title,
				Body:  body,
				Data:  map[string]interface{}{"sessionId": sessionID},
			})
		},
	}
}

// initAttention prepares Windows toasts and routes toast clicks back to the
// session. Failure only disables toasts; it never blocks startup.
func (application *App) initAttention() {
	if err := runtime.InitializeNotifications(application.ctx); err != nil {
		diagnostics.Warn("toast notifications unavailable; taskbar flash still works", err)
		return
	}
	runtime.OnNotificationResponse(application.ctx, func(result runtime.NotificationResult) {
		if result.Error != nil {
			diagnostics.Warn("toast response failed", result.Error)
			return
		}
		// Wails' WindowUnminimise sends SW_RESTORE unconditionally, which also
		// un-maximizes a window that is merely behind others. Restore only when
		// the window is actually minimized (that restores to the prior state).
		if hwnd := mainWindowHandle(); hwnd == 0 {
			runtime.WindowUnminimise(application.ctx)
		} else if iconic, _, _ := procIsIconic.Call(uintptr(hwnd)); iconic != 0 {
			runtime.WindowUnminimise(application.ctx)
		}
		runtime.WindowShow(application.ctx)
		if sessionID, ok := result.Response.UserInfo["sessionId"].(string); ok && sessionID != "" {
			runtime.EventsEmit(application.ctx, focusSessionEvnt, sessionID)
		}
	})
	application.attention.mu.Lock()
	application.attention.toastReady = true
	application.attention.mu.Unlock()
}

func mainWindowHandle() windows.HWND {
	var found windows.HWND
	pid := windows.GetCurrentProcessId()
	callback := syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadProcessID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		if visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd)); visible == 0 {
			return 1
		}
		buffer := make([]uint16, 64)
		procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if windows.UTF16ToString(buffer) == toastAppTitle {
			found = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

func windowIsForeground() bool {
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		return false
	}
	foreground, _, _ := procGetForegroundWindow.Call()
	return windows.HWND(foreground) == hwnd
}

func flashMainWindow() error {
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		return fmt.Errorf("main window not found")
	}
	info := flashInfo{Hwnd: hwnd, Flags: flashwTray | flashwTimerNoFG}
	info.Size = uint32(unsafe.Sizeof(info))
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&info)))
	return nil
}

// isToastActivationArg reports the COM activation argument Windows passes when
// a toast click launches TauShell after it was closed.
func isToastActivationArg(args []string) bool {
	return len(args) == 1 && (args[0] == "-Embedding" || args[0] == "/Embedding" || args[0] == "-embedding")
}

// OpenSettingsFile creates the commented example settings file if missing and
// opens it in the default editor.
func (application *App) OpenSettingsFile() (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("open settings file failed", err)
		}
	}()
	return application.openEditableFile(application.settingsPath(), appconfig.EnsureExample)
}
