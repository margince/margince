// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A contact row is skipped when the caller can see one of the matched contacts.
// The visibility question must therefore reach every ranked match.
func TestEveryMatchedContactIsAskedAboutNotJustTheTopHit(t *testing.T) {
	hidden := ids.From[ids.ContactKind](ids.NewV7())
	visible := ids.From[ids.ContactKind](ids.NewV7())

	fuzzy := contacts.ContactResolution{
		Decision: contacts.DecisionFuzzyReview, ContactID: hidden,
		Reviews: []contacts.ReviewMatch{{ContactID: hidden}, {ContactID: visible}},
	}
	if got := matchedContacts(fuzzy); !slices.Equal(got, []ids.ContactID{hidden, visible}) {
		t.Fatalf("matchedContacts answered %v, want both ranked matches in order", got)
	}

	exact := contacts.ContactResolution{Decision: contacts.DecisionExactCollision, ContactID: visible}
	if got := matchedContacts(exact); !slices.Equal(got, []ids.ContactID{visible}) {
		t.Fatalf("matchedContacts answered %v for an exact collision, want its own id", got)
	}
}
