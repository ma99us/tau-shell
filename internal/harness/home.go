package harness

import "os"

// HomeDir returns the directory, readable from this process, that holds
// harness state (.claude, .copilot, .codex and so on). Every state reader goes
// through it so a runtime other than the Windows user profile can supply its
// own home. It is a variable only so tests and runtimes can replace it.
var HomeDir = os.UserHomeDir
