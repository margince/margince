// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// auth.LockSubjectLive, driven against a real subject.
//
// The census in backend/liveprobelock_test.go says WHICH writers owe the lock;
// it is an AST walk and proves only that they name it. What the lock does — hold
// the subject so a concurrent erasure waits, and refuse a subject that is
// already gone — is a two-transaction claim, and this is where it is held.
//
// In `people` rather than beside the primitive: locking needs a real row, and
// this package has the fixture that seeds one. The auth package's own tests take
// no database.

import (
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALockedSubjectMakesTheEraserWait(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Ingrid Solheim", "ingrid@locked.test", "Solheim AS", "locked.test")

	held, release := make(chan struct{}), make(chan struct{})
	holding := make(chan error, 1)
	// once, because WithWorkspaceTx may run the closure again and a second
	// close would panic in a goroutine the test cannot recover.
	var announce sync.Once
	go func() {
		holding <- e.store.tx(ctx, func(tx pgx.Tx) error {
			if err := auth.LockSubjectLive(ctx, tx, entityPerson, personID.UUID); err != nil {
				return err
			}
			announce.Do(func() { close(held) })
			<-release
			return nil
		})
	}()
	<-held

	// No sleep and no clock: the erasure's own lock_timeout reports the
	// blocking, and it can only fire while something holds the row.
	erasure := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE person SET archived_at = now() WHERE id = $1`, personID)
		return err
	})
	close(release)
	if err := <-holding; err != nil {
		t.Fatalf("the lock itself failed, so this proves nothing about the erasure: %v", err)
	}

	var pgErr *pgconn.PgError
	if !errors.As(erasure, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("the erasure got %v, want a lock timeout (55P03) — FOR NO KEY UPDATE must conflict "+
			"with the archive's UPDATE, or every writer that takes this lock is racing the eraser "+
			"while reporting that it is not", erasure)
	}
}

func TestLockSubjectLiveRefusesWhatItCannotHold(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Bjørn Rype", "bjorn@refuses.test", "Rype AS", "refuses.test")

	for _, tc := range []struct {
		name  string
		table string
		id    ids.UUID
		// archive the subject before the attempt.
		archived bool
	}{
		{
			// The case every caller of this lock is defending against: the
			// erasure got there first, and the write that follows must not land.
			name: "a subject archived before the lock", table: entityPerson, id: personID.UUID, archived: true,
		}, {
			name: "a subject that never existed", table: entityPerson, id: ids.NewV7(),
		}, {
			// ai.Record passes a request-body value here, so an unknown table is
			// reachable input rather than a typo. It owes the caller a sentinel,
			// not the raw SQL error a table with no archived_at would raise.
			name: "a table outside the row-scoped set", table: "audit_log", id: personID.UUID,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.archived {
				if err := e.store.tx(ctx, func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE person SET archived_at = now() WHERE id = $1`, tc.id)
					return err
				}); err != nil {
					t.Fatalf("archive the subject: %v", err)
				}
			}
			err := e.store.tx(ctx, func(tx pgx.Tx) error {
				return auth.LockSubjectLive(ctx, tx, tc.table, tc.id)
			})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("got %v, want not found", err)
			}
		})
	}
}
