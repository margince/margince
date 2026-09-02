// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The commitments card shows the newest claims of every kind, capped, and the
// moment ladder reads that same list. On a record with more than a page of
// recent claims those two purposes disagree: a promise made months ago and due
// tomorrow is nowhere on the newest page, so the card would report nothing
// owed while the record owes something this week.
func TestAnOpenPromiseOffTheNewestPageStillReachesTheLadder(t *testing.T) {
	older := crmcontracts.ConversationClaim{
		Id:     openapi_types.UUID(ids.NewV7()),
		Kind:   crmcontracts.CommitmentOurs,
		Status: crmcontracts.ConversationClaimStatusOpen,
		Body:   "Send the revised quote",
	}
	var newest []crmcontracts.ConversationClaim
	for range sectionCap {
		newest = append(newest, crmcontracts.ConversationClaim{
			Id:     openapi_types.UUID(ids.NewV7()),
			Kind:   crmcontracts.Decision,
			Status: crmcontracts.ConversationClaimStatusOpen,
			Body:   "They picked the annual plan",
		})
	}

	got := withOpenCommitments(newest, []crmcontracts.ConversationClaim{older})

	if len(got) != len(newest)+1 {
		t.Fatalf("carried %d claims, want the page plus the promise it could not show", len(got))
	}
	found := false
	for _, claim := range got {
		if claim.Id == older.Id {
			found = true
		}
	}
	if !found {
		t.Error("the open promise is absent; a card ranking over this list would report nothing owed")
	}
}

// A promise the display page already carries is not carried twice. The ladder
// would rank one promise as two, and the commitments card would list it twice.
func TestAPromiseAlreadyOnThePageIsNotRepeated(t *testing.T) {
	promise := crmcontracts.ConversationClaim{
		Id:     openapi_types.UUID(ids.NewV7()),
		Kind:   crmcontracts.CommitmentOurs,
		Status: crmcontracts.ConversationClaimStatusOpen,
		Body:   "Send the questionnaire",
	}
	got := withOpenCommitments(
		[]crmcontracts.ConversationClaim{promise},
		[]crmcontracts.ConversationClaim{promise},
	)
	if len(got) != 1 {
		t.Errorf("carried %d copies of one promise, want 1", len(got))
	}
}
