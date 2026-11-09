//go:build windows

package sessionhost

import (
	"context"
	"testing"

	"github.com/ma99us/tau-shell/internal/session"
)

// A WSL session's state is unreadable until its distro home is known, so a
// reader never has to start a distro; PowerShell sessions use the Windows home.
func TestReaderHome(t *testing.T) {
	manager := &Manager{}
	if home, ok := manager.readerHome(context.Background(), session.Config{ID: "ps"}); home != "" || !ok {
		t.Fatalf("PowerShell session = %q, %v", home, ok)
	}
	wslConfig := session.Config{ID: "wsl", Runtime: "wsl"}
	wslConfig.WSL.Distro = "Ubuntu"
	if _, ok := manager.readerHome(context.Background(), wslConfig); ok {
		t.Fatal("WSL session readable before its home was learned")
	}
	manager.wslHomes.remember("Ubuntu", `\\wsl.localhost\Ubuntu\home\me`)
	if got := manager.wslHomes.get("Ubuntu"); got != `\\wsl.localhost\Ubuntu\home\me` {
		t.Fatalf("remembered home = %q", got)
	}
}
