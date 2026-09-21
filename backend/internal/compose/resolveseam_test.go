// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The seam carries the EXACT flag, and that is the one field a silent drop would
// change the answer over.
//
// `Exact` is what turns a single surviving match into `matched` rather than
// `ambiguous`. A translation that forgot it would compile, pass every contacts
// test and every agents test — and quietly downgrade every key hit on the
// surface to "a human decides", which reads as caution rather than as a bug.
// Neither module can see the other, so this is the only place the two ends meet.
func TestTheSeamCarriesWhetherAKeyOrASimilarityNamedTheRecord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ref   contacts.ResolveRef
		exact bool
	}{
		{"an exact lane", contacts.ResolveRef{Kind: contacts.ResolveContact, ID: ids.NewV7(), Exact: true, Confidence: 1}, true},
		{"a name similarity", contacts.ResolveRef{Kind: contacts.ResolveContact, ID: ids.NewV7(), Confidence: 0.8}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveOutcomesFor([]contacts.ResolveOutcome{{Refs: []contacts.ResolveRef{tc.ref}}})
			if len(got) != 1 || len(got[0].Refs) != 1 {
				t.Fatalf("the adapter answered %+v, want the one ref carried through", got)
			}
			if got[0].Refs[0].Exact != tc.exact {
				t.Errorf("Exact = %v, want %v — the decision word is computed from this",
					got[0].Refs[0].Exact, tc.exact)
			}
			if got[0].Refs[0].ID != tc.ref.ID || got[0].Refs[0].Confidence != tc.ref.Confidence {
				t.Errorf("ref = %+v, want the ladder's own id and score", got[0].Refs[0])
			}
		})
	}
}
