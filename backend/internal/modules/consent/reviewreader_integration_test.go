// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestReadingAnotherColleaguesReviewPreservesItsInitiator(t *testing.T) {
	e := setupChannelConsent(t)
	var opened Review
	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		opened, err = OpenReviewTx(e.ctx, tx, commsauthz.DecisionSet{
			Decisions: []commsauthz.Decision{{
				Recipient:  connector.Recipient{Email: "anna@example.test"},
				Verdict:    commsauthz.VerdictDeny,
				ReasonCode: "marketing_objection",
			}},
		}, ids.Nil)
		return err
	})
	if err != nil {
		t.Fatalf("opening a refused send: %v", err)
	}

	reader := directorCtx(e.ws, ids.NewV7())
	read, err := e.store.ReviewForReader(reader, opened.ID)
	if err != nil {
		t.Fatalf("reading another colleague's review: %v", err)
	}
	if read.InitiatedBy != e.user {
		t.Errorf("initiator = %s, want sender %s", read.InitiatedBy, e.user)
	}
	if len(read.Refusals) != 1 || read.Refusals[0].Address != "anna@example.test" {
		t.Fatalf("refusals = %+v, want the sender's refused recipient", read.Refusals)
	}

	// This is the value the initiator's ON DELETE SET NULL leaves behind.
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE communication_review SET initiated_by = NULL WHERE id = $1`, opened.ID); err != nil {
		t.Fatalf("clearing the review's initiator: %v", err)
	}
	read, err = e.store.ReviewForReader(reader, opened.ID)
	if err != nil {
		t.Fatalf("reading a review whose initiator is gone: %v", err)
	}
	if !read.InitiatedBy.IsZero() {
		t.Errorf("absent initiator = %s, want zero", read.InitiatedBy)
	}
}

// A review stored as needs_context before the answerable list narrowed reads
// as needs_repair, so no read offers an answer RecordContext refuses.
func TestAStoredNeedsContextReviewReadsByTheCurrentAnswerableList(t *testing.T) {
	e := setupChannelConsent(t)
	var opened Review
	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		opened, err = OpenReviewTx(e.ctx, tx, commsauthz.DecisionSet{
			Decisions: []commsauthz.Decision{{
				Recipient:  connector.Recipient{Email: "anna@example.test"},
				Verdict:    commsauthz.VerdictDeny,
				ReasonCode: commsauthz.ReasonClaimContradictsResolution,
			}},
		}, ids.Nil)
		return err
	})
	if err != nil {
		t.Fatalf("opening a refused send: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE communication_review SET state = 'needs_context' WHERE id = $1`, opened.ID); err != nil {
		t.Fatalf("storing the earlier state: %v", err)
	}

	byReader, err := e.store.ReviewForReader(directorCtx(e.ws, ids.NewV7()), opened.ID)
	if err != nil {
		t.Fatalf("reading as a reader: %v", err)
	}
	byInitiator, err := e.store.ReviewForInitiator(e.ctx, opened.ID)
	if err != nil {
		t.Fatalf("reading as the initiator: %v", err)
	}
	for name, got := range map[string]string{"reader": byReader.State, "initiator": byInitiator.State} {
		if got != ReviewNeedsRepair {
			t.Errorf("%s state = %q, want %q", name, got, ReviewNeedsRepair)
		}
	}
}
