//go:build windows

package ide

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIconDataURLReadsExecutableIcon(t *testing.T) {
	notepad := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	if got := IconDataURL(notepad); !strings.HasPrefix(got, "data:image/png;base64,") || len(got) < 200 {
		t.Fatalf("icon = %.40q", got)
	}
	if got := IconDataURL(filepath.Join(t.TempDir(), "missing.exe")); got != "" {
		t.Fatalf("missing exe returned an icon: %.40q", got)
	}
}
