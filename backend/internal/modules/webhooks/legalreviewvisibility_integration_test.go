// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// A subscriber receives a legal review's events when they may read legal reviews
// and the review's deal is in their row scope. The deal grant is not asked, as
// the review's own read opens its pack without one.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *contractVisEnv) seedReview(t *testing.T) ids.UUID {
	t.Helper()
	e.seedDealAndContract(t)
	review := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO legal_review (id, deal_id, packet, captured_by)
		VALUES ($1, $2, '{}'::jsonb, 'human:test')`, review, e.deal); err != nil {
		t.Fatalf("seeding the review: %v", err)
	}
	return review
}

func TestAReviewSubjectNeedsNoGrantOnItsDeal(t *testing.T) {
	e := setupContractVis(t)
	review := e.seedReview(t)
	visible, err := e.store.legalReviewVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"legal_review": {Read: true}}), review)
	if err != nil || !visible {
		t.Fatalf("visible = %v, %v; a reader who opens the pack without deal.read must receive its events", visible, err)
	}
}

func TestAReviewSubjectStillNeedsLegalReviewRead(t *testing.T) {
	e := setupContractVis(t)
	review := e.seedReview(t)
	visible, err := e.store.legalReviewVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"deal": {Read: true}}), review)
	if err != nil || visible {
		t.Fatalf("visible = %v, %v; a seat without legal_review.read received the event", visible, err)
	}
}

func TestAMissingReviewIsWithheld(t *testing.T) {
	e := setupContractVis(t)
	visible, err := e.store.legalReviewVisibleTo(
		e.asHolding(map[string]principal.ObjectGrant{"legal_review": {Read: true}}), ids.NewV7())
	if err != nil || visible {
		t.Fatalf("visible = %v, %v; an absent review was delivered", visible, err)
	}
}
