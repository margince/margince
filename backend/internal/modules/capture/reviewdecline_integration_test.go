// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A review the CAS on `unsure` turned away is two different answers.
//
// A redeemed approval replayed asks for the decision the row already carries,
// and the no-op is what makes the replay safe. A second reviewer asks for a
// different one, and answering nil there drops a human's decision while their
// screen says saved.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASecondReviewersDifferentDecisionIsRefused(t *testing.T) {
	ctx, _, db, _ := traceReadWorkspace(t)
	pending := capture.NewPendingStore(db)
	id := seedUnsureDisposition(ctx, t, db, "contested@review.test")

	// The reviewer who gets there first decides.
	if err := inTx(ctx, t, db, func(tx pgx.Tx) error {
		return pending.ResolveReviewedAs(ctx, tx, id, capture.PendingStatusReal,
			capture.KindContact, "accepted in the review queue")
	}); err != nil {
		t.Fatalf("the first reviewer's decision: %v", err)
	}

	// The same redemption replayed asks for what the row already says, so it
	// stays a no-op rather than becoming a failure.
	if err := inTx(ctx, t, db, func(tx pgx.Tx) error {
		return pending.ResolveReviewedAs(ctx, tx, id, capture.PendingStatusReal,
			capture.KindContact, "accepted in the review queue")
	}); err != nil {
		t.Errorf("replaying the same decision → %v, want nil: the CAS is what makes a replay safe", err)
	}

	// A second reviewer deciding otherwise is told, rather than losing it.
	err := inTx(ctx, t, db, func(tx pgx.Tx) error {
		return pending.ResolveReviewedAs(ctx, tx, id, capture.PendingStatusNoise, "", "judged noise")
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second, different decision → %v, want ErrConflict", err)
	}

	// The row keeps the first decision.
	var stored string
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT status FROM capture_pending_counterparty WHERE id = $1`, id).Scan(&stored)
	}); err != nil {
		t.Fatalf("reading the disposition back: %v", err)
	}
	if stored != capture.PendingStatusReal {
		t.Errorf("status = %q, want the first reviewer's %q", stored, capture.PendingStatusReal)
	}
}

// A disposition that is gone answers not-found rather than conflict: there is
// no competing decision, there is no row.
func TestResolvingAMissingDispositionIsNotFound(t *testing.T) {
	ctx, _, db, _ := traceReadWorkspace(t)
	pending := capture.NewPendingStore(db)

	err := inTx(ctx, t, db, func(tx pgx.Tx) error {
		return pending.ResolveReviewedAs(ctx, tx, ids.NewV7(), capture.PendingStatusReal, "", "no such row")
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("resolving a missing disposition → %v, want ErrNotFound", err)
	}
}

// seedUnsureDisposition files one ledger row a human still has to judge.
//
// Seeded through seedDeferredMessage rather than by hand, because the ledger's
// activity_id and owner_id are foreign keys.
//
// A row written straight into the table would be a test about referential
// integrity.
func seedUnsureDisposition(ctx context.Context, t *testing.T, db *database.DB, email string) ids.UUID {
	t.Helper()
	owner := ids.NewV7()
	seedDeferredMessage(ctx, t, db, owner, "review-"+email, email, true)
	var id ids.UUID
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE capture_pending_counterparty SET status = 'unsure'
			 WHERE email = $1 RETURNING id`, email).Scan(&id)
	}); err != nil {
		t.Fatalf("putting the seeded disposition back to unsure: %v", err)
	}
	return id
}

// inTx runs one store call on its own transaction, which is how the review
// doors call it.
func inTx(ctx context.Context, t *testing.T, db *database.DB, fn func(pgx.Tx) error) error {
	t.Helper()
	return db.Tx(ctx, fn)
}

// A proposal attached to a row a human decided mid-flight is refused.
//
// The review queue is never left pointing at an offer for a settled question.
func TestLinkingAProposalToADecidedRowIsRefused(t *testing.T) {
	ctx, _, db, _ := traceReadWorkspace(t)
	pending := capture.NewPendingStore(db)
	id := seedUnsureDisposition(ctx, t, db, "decided@review.test")

	// The human decides before the sweep attaches its offer.
	if err := inTx(ctx, t, db, func(tx pgx.Tx) error {
		return pending.ResolveReviewedAs(ctx, tx, id, capture.PendingStatusReal, "", "decided first")
	}); err != nil {
		t.Fatalf("the reviewer's decision: %v", err)
	}

	err := pending.LinkProposal(ctx, id, ids.NewV7())
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("linking a proposal to a decided row → %v, want ErrConflict", err)
	}
}

// A retirement whose claim another worker took is refused.
//
// The verdict it carries is not discarded while the caller counts the attempt
// as spent.
func TestRetiringWithALostClaimIsRefused(t *testing.T) {
	ctx, _, db, _ := traceReadWorkspace(t)
	pending := capture.NewPendingStore(db)
	id := seedUnsureDisposition(ctx, t, db, "raced@review.test")

	// Put it back to pending, claimed by somebody else.
	theirs := ids.NewV7()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_pending_counterparty
			   SET status = 'pending', resolved_at = NULL, claimed_by = $2,
			       claimed_until = now() + interval '5 minutes'
			 WHERE id = $1`, id, theirs)
		return err
	}); err != nil {
		t.Fatalf("handing the claim to another worker: %v", err)
	}

	err := pending.Retire(ctx, capture.PendingCounterparty{ID: id, Claim: ids.NewV7()},
		"below the confidence floor", capture.VerdictMeasurement{})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("retiring under a lost claim → %v, want ErrConflict", err)
	}
}
