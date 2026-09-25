// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A subject who was never proposed as anybody's duplicate.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An erasure with nothing to scrub runs no statement at all.
//
// Both arms empty is the ordinary case, not an edge one: most subjects were
// never filed against a twin, and the retention sweep's lead arm hands this no
// contacts by construction while its contact arm hands it no leads.
//
// A nil transaction is the assertion, the same way the identity retirement next
// door makes it: if the short circuit is ever removed, this panics rather than
// passing quietly against a statement that would have matched nothing anyway.
func TestScrubbingNoCandidatesTouchesNoTransaction(t *testing.T) {
	// Both shapes an empty side arrives in — nil from a caller that built no
	// slice, and an allocated empty one from a sweep that filled nothing in.
	for name, empty := range map[string][]ids.UUID{"nil": nil, "allocated": {}} {
		if err := scrubDedupeEvidence(context.Background(), nil, empty, empty); err != nil {
			t.Errorf("an %s empty subject set answered %v, want nothing to do", name, err)
		}
	}
}
