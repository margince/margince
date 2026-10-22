// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNotificationEligibilityRefusesMissingExpiredAndUnrepresentedDecisions(t *testing.T) {
	e := setupStaging(t)
	id, err := e.svc.Stage(e.forRep(), StageInput{
		Kind: "fx_rate_proposal", ProposedChange: []byte(`{}`), DiffHash: "notification-expiry",
	})
	if err != nil {
		t.Fatal(err)
	}
	var expires time.Time
	if err := e.owner.QueryRow(t.Context(), `SELECT expires_at FROM approval WHERE id = @id`, pgx.StrictNamedArgs{"id": id}).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	e.svc.now = func() time.Time { return expires.Add(time.Second) }
	for _, candidate := range []ids.ApprovalID{id, ids.New[ids.ApprovalKind]()} {
		could, err := e.svc.PendingDecidableBy(e.as(), candidate)
		if err != nil || could {
			t.Fatalf("ineligible decision %s: could=%t err=%v", candidate, could, err)
		}
		if err := e.svc.db.Tx(e.as(), func(tx pgx.Tx) error {
			could, err := e.svc.PendingAskedOfInTx(e.as(), tx, candidate)
			if could {
				t.Error("notification accepted a missing or expired decision")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if could, err := e.svc.PendingDecidableBy(e.sweeping(), id); could || !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unrepresented system decision: could=%t err=%v", could, err)
	}
	if could, err := e.svc.PendingAskedOfInTx(e.sweeping(), nil, id); could || !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unrepresented system notification: could=%t err=%v", could, err)
	}
}

func TestNotificationAndMaintenancePropagateUnavailableTransactions(t *testing.T) {
	e := setupStaging(t)
	tx, err := e.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	id := ids.New[ids.ApprovalKind]()
	if could, err := e.svc.PendingAskedOfInTx(e.as(), tx, id); could || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("notification on closed transaction: could=%t err=%v", could, err)
	}
	if could, err := e.svc.pendingDecidable(e.as(), tx, id); could || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("decision on closed transaction: could=%t err=%v", could, err)
	}
	if pending, err := e.svc.StillPending(e.sweeping(), tx, []ids.ApprovalID{id}); pending != nil || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("maintenance on closed transaction: pending=%v err=%v", pending, err)
	}
	if pending, err := e.svc.StillPending(e.sweeping(), nil, make([]ids.ApprovalID, 1001)); pending != nil || !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("unbounded maintenance read: pending=%v err=%v", pending, err)
	}
}
