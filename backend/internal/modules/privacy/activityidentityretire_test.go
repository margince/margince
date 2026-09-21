// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A page with no emails on it.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An empty page runs no statement at all.
//
// The cascade calls this with whatever the timeline redaction emptied, and a
// subject whose timeline holds no mail empties nothing — an ordinary outcome,
// not an edge case. `DELETE ... WHERE activity_id = ANY('{}')` would be
// harmless, so the short circuit is about not spending a round trip per erasure
// that had no messages in it.
//
// A nil transaction is the assertion. There is no tx to give a unit test here,
// and that is exactly what makes the check sharp: if the guard is ever removed,
// this panics rather than quietly passing against a mock that would have
// answered the statement anyway.
func TestRetiringNoIdentitiesTouchesNoTransaction(t *testing.T) {
	// Both shapes an empty page arrives in: nil from a caller that built no
	// slice, and an allocated empty one from emailIDsOf, which returns a slice
	// it filled nothing into.
	for name, page := range map[string][]ids.UUID{"nil": nil, "allocated": {}} {
		if err := retireActivityIdentities(context.Background(), nil, page); err != nil {
			t.Errorf("an %s empty page answered %v, want nothing to do", name, err)
		}
	}
}
