// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The commitment card's receipt is written in both shapes, and they name the
// same conversation. A card whose two receipts disagreed would open one message
// from the link and cite another beside it.
func TestACommitmentCardCitesTheSameConversationItLinksTo(t *testing.T) {
	t.Parallel()
	source := ids.NewV7()

	card := commitmentAttention(people.ProjectCommitment{
		Body:       "We will send the fallback matrix by Friday.",
		ActivityID: source,
	})

	if card == nil {
		t.Fatal("a commitment with a body produced no card")
	}
	if card.SourceActivityId == nil {
		t.Fatal("the card carries no source activity")
	}
	if card.SourceEvidence == nil {
		t.Fatal("the card carries no citation, so the one renderer cannot decide whether to open it")
	}
	if card.SourceEvidence.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
		t.Errorf("cited a %s; the receipt is the conversation it was read from", card.SourceEvidence.EntityType)
	}
	if card.SourceEvidence.EntityId != *card.SourceActivityId {
		t.Errorf("the citation names %s and the link names %s", card.SourceEvidence.EntityId, *card.SourceActivityId)
	}
}

// The refusal case: a commitment with nothing said produces no card at all, so
// the citation above is not simply written unconditionally.
func TestACommitmentWithNoBodyProducesNoCard(t *testing.T) {
	t.Parallel()
	if card := commitmentAttention(people.ProjectCommitment{ActivityID: ids.NewV7()}); card != nil {
		t.Errorf("got a card for a commitment that said nothing: %+v", card)
	}
}

// An overdue task is the other lane, and it was read from no conversation. A
// citation there would be a receipt pointing at nothing.
func TestAnOverdueTaskCardCitesNothing(t *testing.T) {
	t.Parallel()
	card := taskAttention(overdueTask{
		Subject: "Send the matrix",
		DueAt:   time.Date(2026, time.September, 4, 9, 0, 0, 0, time.UTC),
	})
	if card == nil {
		t.Fatal("an overdue task with a due date produced no card")
	}
	if card.SourceEvidence != nil {
		t.Error("the task card cites a conversation; it was not read from one")
	}
}
