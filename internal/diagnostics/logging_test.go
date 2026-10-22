package diagnostics

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAtOverwritesAndWritesStructuredRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "taushell.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("previous run"), 0o600); err != nil {
		t.Fatal(err)
	}

	closeLog, err := InitAt(path)
	if err != nil {
		t.Fatalf("initialize diagnostics: %v", err)
	}
	Info("test event", "session_id", "test-session")
	if err := closeLog(); err != nil {
		t.Fatalf("close diagnostics: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read diagnostics log: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "previous run") {
		t.Fatalf("diagnostics log retained previous run contents: %q", content)
	}
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log record is not JSON: %v (%q)", err, line)
		}
		if record["time"] == nil || record["level"] == nil || record["msg"] == nil {
			t.Fatalf("log record lacks standard fields: %#v", record)
		}
	}
	if !strings.Contains(content, `"msg":"test event"`) {
		t.Fatalf("diagnostics log does not contain test event: %q", content)
	}
}

func TestSanitizeRedactsCredentialPatternsAndBoundsText(t *testing.T) {
	text := "Authorization: Bearer abcdefghijklmnop api_key=secret-value ghp_123456789012345678901234567890"
	sanitized := Sanitize(text, 256)
	for _, secret := range []string{"abcdefghijklmnop", "secret-value", "ghp_123456789012345678901234567890"} {
		if strings.Contains(sanitized, secret) {
			t.Fatalf("sanitized text retained secret %q: %q", secret, sanitized)
		}
	}

	truncated := Sanitize(strings.Repeat("x", 32), 16)
	if len(truncated) > 32 || !strings.Contains(truncated, "[truncated]") {
		t.Fatalf("bounded text = %q", truncated)
	}
}

func TestWailsBridgeFailureOmitsRequestContent(t *testing.T) {
	message := safeWailsMessage(`process message error: {"method":"WriteTerminal","input":"sensitive command"} -> rejected`)
	if strings.Contains(message, "sensitive command") || strings.Contains(message, "WriteTerminal") {
		t.Fatalf("Wails bridge failure retained request data: %q", message)
	}
}

func TestFrontendFailureValidatesSourceAndBoundsSensitiveData(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)

	FrontendFailure("untrusted-source", "api_key=secret-value", strings.Repeat("stack", 2000))

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("frontend failure record is not JSON: %v", err)
	}
	if record["source"] != "frontend.unknown" {
		t.Fatalf("frontend failure source = %v, want validated fallback", record["source"])
	}
	if strings.Contains(output.String(), "secret-value") {
		t.Fatal("frontend failure record retained credential-like data")
	}
	if len(record["message"].(string)) > maxFrontendMessage+12 {
		t.Fatalf("frontend message was not bounded: %d bytes", len(record["message"].(string)))
	}
	if len(record["stack"].(string)) > maxFrontendStack+12 {
		t.Fatalf("frontend stack was not bounded: %d bytes", len(record["stack"].(string)))
	}
}
