// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A merge can hand a contact's primary-employer slot to a kept edge. It waits
// for a writer holding that contact's employment lock, then sees its write.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAContactMergeWaitsForAPrimaryEmploymentEditOnTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Ilse Varga", "Varga Shared", "Varga Own")
	survivor := mustContact(t, e, "Ilse Varga")
	e.employAt(ctx, t, survivor, fx.first, false)
	own := e.employAt(ctx, t, survivor, fx.second, false)
	e.employAt(ctx, t, fx.contact, fx.first, true)
	if got := slotHolder(ctx, t, e, survivor); !got.IsZero() {
		t.Fatalf("the survivor starts with primary edge %v, want none", got)
	}

	holder, holderPID := e.beginPrimaryEdit(ctx, t, own.ID)
	merged := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		_, err := e.store.MergeContact(ctx, fx.contact, survivor, nil)
		merged <- err
		close(returned)
	}()
	testdb.AwaitQueuedBehind(t, e.store.db.Pool(), int32(holderPID), 1, returned, "the merge") //nolint:gosec // a backend pid fits in int32
	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("committing the primary edit: %v", err)
	}

	if err := <-merged; err != nil {
		t.Fatalf("the merge failed instead of waiting for the edit: %v", err)
	}
	if got := slotHolder(ctx, t, e, survivor); got != own.ID {
		t.Errorf("the survivor's primary slot is held by %v, want the edge %v the user made primary", got, own.ID)
	}
}

// beginPrimaryEdit makes edge the primary employment in a transaction the test
// holds open, and returns it with its backend pid.
func (e *dedupeEnv) beginPrimaryEdit(ctx context.Context, t *testing.T, edge ids.UUID) (pgx.Tx, int) {
	t.Helper()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the edit's transaction: %v", err)
	}
	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the edit's transaction: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the edit's backend pid: %v", err)
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		t.Fatalf("reading the editor: %v", err)
	}
	primary := true
	if _, err := e.store.updateRelationshipInTx(ctx, tx, edge, UpdateRelationshipInput{IsCurrentPrimary: &primary}, by); err != nil {
		t.Fatalf("the edit that holds the employment lock: %v", err)
	}
	return tx, pid
}

// Employer attach, the company roster and the seat repair take a contact's
// employment lock before its row. A merge keeps that order, so the two queue.
func TestAContactMergeQueuesBehindAWriterHoldingTheEmploymentLockFirst(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Petra Holm", "Holm Shared", "Holm Own")
	survivor := mustContact(t, e, "Petra Holm")
	e.employAt(ctx, t, fx.contact, fx.first, true)

	holder, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the writer's transaction: %v", err)
	}
	t.Cleanup(func() {
		err := holder.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the writer's transaction: %v", err)
		}
	})
	var holderPID int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatalf("reading the writer's backend pid: %v", err)
	}
	if err := storekit.LockWriteIdentity(ctx, holder, employmentKind, survivor.UUID.String()); err != nil {
		t.Fatalf("taking the survivor's employment lock: %v", err)
	}

	merged := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		_, err := e.store.MergeContact(ctx, fx.contact, survivor, nil)
		merged <- err
		close(returned)
	}()
	testdb.AwaitQueuedBehind(t, e.store.db.Pool(), int32(holderPID), 1, returned, "the merge") //nolint:gosec // a backend pid fits in int32
	if _, err := holder.Exec(ctx, `SELECT 1 FROM contact WHERE id = $1 FOR UPDATE`, survivor.UUID); err != nil {
		t.Fatalf("the writer could not take the contact row after its employment lock: %v", err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("committing the writer: %v", err)
	}
	if err := <-merged; err != nil {
		t.Fatalf("the merge failed instead of queuing behind the writer: %v", err)
	}
}
