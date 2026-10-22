package diagnostics

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	maxFrontendMessage = 4096
	maxFrontendStack   = 8192
)

var (
	credentialPattern = regexp.MustCompile(`(?i)\b(authorization|access[_ -]?token|refresh[_ -]?token|api[_ -]?key|password|secret)\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	bearerPattern     = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/-]+=*`)
	githubToken       = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)
	frontendSources   = map[string]struct{}{
		"window.error":              {},
		"window.unhandledrejection": {},
		"react.render":              {},
		"storage.recent-folders":    {},
		"storage.tabs":              {},
		"ui.refresh":                {},
		"ui.session-launch":         {},
		"ui.stop-session":           {},
		"ui.terminal":               {},
		"ui.terminal-layout":        {},
		"ui.create-session":         {},
	}
)

type WailsLogger struct{}

func LogPath() string {
	return filepath.Join(os.TempDir(), "TauShell", "taushell.log")
}

func Init() (func() error, error) {
	return InitAt(LogPath())
}

func InitAt(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create diagnostics directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open diagnostics log: %w", err)
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})))
	slog.Info("diagnostics initialized", "log_path", path, "pid", os.Getpid())

	var once sync.Once
	var closeErr error
	return func() error {
		once.Do(func() {
			slog.Info("diagnostics closing")
			closeErr = errors.Join(file.Sync(), file.Close())
			slog.SetDefault(previous)
		})
		return closeErr
	}, nil
}

func Debug(message string, args ...any) {
	slog.Debug(Sanitize(message, maxFrontendMessage), sanitizeArgs(args)...)
}

func Info(message string, args ...any) {
	slog.Info(Sanitize(message, maxFrontendMessage), sanitizeArgs(args)...)
}

func Warn(message string, err error, args ...any) {
	slog.Warn(Sanitize(message, maxFrontendMessage), append(sanitizeArgs(args), "error", sanitizeError(err))...)
}

func Error(message string, err error, args ...any) {
	slog.Error(Sanitize(message, maxFrontendMessage), append(sanitizeArgs(args), "error", sanitizeError(err))...)
}

func FrontendFailure(source, message, stack string) {
	if _, ok := frontendSources[source]; !ok {
		source = "frontend.unknown"
	}
	slog.Error("frontend failure",
		"source", source,
		"message", Sanitize(message, maxFrontendMessage),
		"stack", Sanitize(stack, maxFrontendStack),
	)
}

func Sanitize(value string, maxBytes int) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = credentialPattern.ReplaceAllString(value, "$1$2[REDACTED]")
	value = githubToken.ReplaceAllString(value, "[REDACTED_TOKEN]")
	if maxBytes > 0 && len(value) > maxBytes {
		value = value[:maxBytes]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
		value += " [truncated]"
	}
	return value
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return Sanitize(err.Error(), maxFrontendMessage)
}

func sanitizeArgs(args []any) []any {
	clean := append([]any(nil), args...)
	for index := 0; index+1 < len(clean); index += 2 {
		key, ok := clean[index].(string)
		if !ok {
			continue
		}
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "prompt") || strings.Contains(lowerKey, "terminal_input") ||
			strings.Contains(lowerKey, "terminal_output") || strings.Contains(lowerKey, "environment") ||
			strings.Contains(lowerKey, "credential") || strings.Contains(lowerKey, "password") ||
			strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "secret") {
			clean[index+1] = "[REDACTED]"
		} else if value, ok := clean[index+1].(string); ok {
			clean[index+1] = Sanitize(value, maxFrontendMessage)
		} else if value, ok := clean[index+1].(error); ok {
			clean[index+1] = sanitizeError(value)
		}
	}
	return clean
}

func (WailsLogger) Print(message string) {
	Info("Wails", "message", safeWailsMessage(message))
}

func (WailsLogger) Trace(message string) {
	Debug("Wails trace", "message", safeWailsMessage(message))
}

func (WailsLogger) Debug(message string) {
	Debug("Wails debug", "message", safeWailsMessage(message))
}

func (WailsLogger) Info(message string) {
	Info("Wails info", "message", safeWailsMessage(message))
}

func (WailsLogger) Warning(message string) {
	Warn("Wails warning", errors.New(safeWailsMessage(message)))
}

func (WailsLogger) Error(message string) {
	Error("Wails error", errors.New(safeWailsMessage(message)))
}

func (WailsLogger) Fatal(message string) {
	Error("Wails fatal", errors.New(safeWailsMessage(message)))
}

func safeWailsMessage(message string) string {
	if strings.Contains(message, "process message error:") {
		return "Wails bridge request failed; request content redacted"
	}
	return Sanitize(message, maxFrontendMessage)
}
