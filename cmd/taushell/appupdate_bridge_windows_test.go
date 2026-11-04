//go:build windows

package main

import (
	"reflect"
	"testing"
)

// The frontend calls these through window.go.main.App; a missing method only
// fails at runtime in the page, so pin them here.
func TestAppBindsUpdateMethods(t *testing.T) {
	appType := reflect.TypeOf(&App{})
	for _, name := range []string{"CheckAppUpdate", "GetUpdateResult", "GetRollbackVersion", "InstallAppUpdate", "RollbackAppUpdate", "OpenReleasePage", "GetAppVersion"} {
		if _, ok := appType.MethodByName(name); !ok {
			t.Errorf("App has no exported method %s", name)
		}
	}
}
