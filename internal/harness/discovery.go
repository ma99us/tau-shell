package harness

import (
	"context"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

// DiscoverAndClaimSessionID applies the shared discovery policy for CLI and
// desktop launches: exclude IDs already in TauShell or present before launch,
// require an unambiguous candidate, then atomically claim it for this launch.
func DiscoverAndClaimSessionID(ctx context.Context, store *session.Store, adapter Adapter, sessionID, workingDir string, since time.Time, existingIDs map[string]bool, launchToken string) (string, error) {
	taken, err := store.HarnessSessionIDs(ctx, adapter.ID())
	if err != nil {
		return "", err
	}
	taken = CombineSessionIDs(taken, existingIDs)
	found, err := adapter.DiscoverSessionID(workingDir, since, taken)
	if err != nil || found == "" {
		return found, err
	}
	claimed, err := store.ClaimHarnessSessionID(ctx, sessionID, adapter.ID(), launchToken, found)
	if err != nil {
		return "", err
	}
	if !claimed {
		return "", session.ErrHarnessSessionIDClaimed
	}
	return found, nil
}
