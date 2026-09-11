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

func TestReadingAnotherPersonsReviewPreservesItsInitiator(t *testing.T) {
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
		t.Fatalf("reading another person's review: %v", err)
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
