//go:build windows

package main

import "golang.org/x/sys/windows"

// Administrator status values reported to Home. This is information only:
// TauShell never elevates itself.
const (
	adminElevated  = "elevated"  // running with an administrator token
	adminAvailable = "available" // user is a local administrator but TauShell is not elevated
	adminStandard  = "standard"  // standard user
	adminUnknown   = "unknown"
)

func detectAdminStatus() string {
	token := windows.GetCurrentProcessToken()
	if token.IsElevated() {
		return adminElevated
	}
	// A filtered (UAC) token of an administrator has a linked full token; a standard
	// user has none that is elevated.
	if linked, err := token.GetLinkedToken(); err == nil {
		defer linked.Close()
		if linked.IsElevated() {
			return adminAvailable
		}
	}
	return adminStandard
}
