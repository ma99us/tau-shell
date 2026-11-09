package runtime

import (
	"reflect"
	"testing"
)

func TestInstallArgs(t *testing.T) {
	got, err := InstallArgs(ActionInstallDistro, "Ubuntu-24.04")
	if err != nil || !reflect.DeepEqual(got, []string{"--install", "-d", "Ubuntu-24.04", "--no-launch"}) {
		t.Fatalf("distro args = %v, %v", got, err)
	}
	if got, _ := InstallArgs(ActionInstallWSL, ""); !reflect.DeepEqual(got, []string{"--install", "--no-distro"}) {
		t.Fatalf("wsl args = %v", got)
	}
	if got, _ := InstallArgs(ActionUpdateWSL, ""); !reflect.DeepEqual(got, []string{"--update"}) {
		t.Fatalf("update args = %v", got)
	}
	for _, bad := range []string{"", "a b", "x&calc", "-d", "Ubuntu\n"} {
		if _, err := InstallArgs(ActionInstallDistro, bad); err == nil {
			t.Errorf("distro %q accepted", bad)
		}
	}
	if _, err := InstallArgs("reboot", ""); err == nil {
		t.Error("unknown action accepted")
	}
}

func TestParseOnlineDistros(t *testing.T) {
	text := "The following is a list.\n\nInstall using 'wsl.exe --install <Distro>'.\n\nNAME      FRIENDLY NAME\nUbuntu    Ubuntu\nDebian    Debian GNU/Linux\nOracleLinux_7_9   Oracle Linux 7.9\n\n"
	want := []string{"Ubuntu", "Debian", "OracleLinux_7_9"}
	if got := ParseOnlineDistros(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := ParseOnlineDistros("no table here"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
