package citations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnresolvedStatusesCoverEverySkip guards --reset. ResetUnlinked deletes
// exactly the statuses in UnresolvedStatuses, so a new non-linked status that
// is not added there would survive a reset and never be re-derived from the
// corrected whitelist -- the hazard that made match_tier a column rather than
// a family of statuses. Reads every Status* constant out of the source so there
// is no second list to keep in step.
func TestUnresolvedStatusesCoverEverySkip(t *testing.T) {
	statuses := stringConstants(t, "Status")
	require.NotEmpty(t, statuses, "found no Status* constants; has the naming changed?")

	unresolved := make(map[string]bool, len(UnresolvedStatuses))
	for _, s := range UnresolvedStatuses {
		unresolved[s] = true
	}

	for name, value := range statuses {
		if strings.HasPrefix(value, "linked_") {
			require.False(t, unresolved[value], "%s = %q is a link and must not be reset", name, value)
			continue
		}
		require.True(t, unresolved[value], "%s = %q is not in UnresolvedStatuses, so --reset would keep it", name, value)
	}
}
