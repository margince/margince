// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// auth.EnsureLinkTarget's lock, driven against a real target row.
//
// The probe answers "may this caller reference this record, and is it still
// live", and every caller writes the reference afterwards in the same
// transaction. What makes the answer still true at the write is a two-
// transaction claim about a row lock, so it is held here rather than beside the
// primitive: locking needs a real row, and the auth package's own tests take no
// database. Its sibling for the exclusive subject lock is
// subjectlock_integration_test.go.

import (
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/auth"
)

func TestAProbedLinkTargetMakesTheArchiveWait(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Marit Fossum", "marit@held.test", "Fossum AS", "held.test")

	held, release := make(chan struct{}), make(chan struct{})
	holding := make(chan error, 1)
	// once, because WithWorkspaceTx may run the closure again and a second
	// close would panic in a goroutine the test cannot recover.
	var announce sync.Once
	go func() {
		holding <- e.store.tx(ctx, func(tx pgx.Tx) error {
			if err := auth.EnsureLinkTarget(ctx, tx, entityPerson, personID.UUID); err != nil {
				return err
			}
			announce.Do(func() { close(held) })
			<-release
			return nil
		})
	}()
	// Either outcome, never just the good one: a probe that FAILED sends on
	// holding and closes nothing, so waiting on held alone would hang to the
	// package timeout and report a deadline where the real answer is one line
	// of error text.
	select {
	case <-held:
	case err := <-holding:
		t.Fatalf("the probe failed, so this proves nothing about the archive: %v", err)
	}

	// No sleep and no clock: the archive's own lock_timeout reports the
	// blocking, and it can only fire while something holds the row.
	archive := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE person SET archived_at = now() WHERE id = $1`, personID)
		return err
	})
	close(release)
	if err := <-holding; err != nil {
		t.Fatalf("the probe itself failed, so this proves nothing about the archive: %v", err)
	}

	var pgErr *pgconn.PgError
	if !errors.As(archive, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("the archive got %v, want a lock timeout (55P03) — FOR SHARE must conflict with it, "+
			"or every reference written after this probe can still land on a record archived in between",
			archive)
	}
}

// The other half of the pairing, and the reason the lock is SHARE rather than
// exclusive: a person mid-ingest is referenced by every message captured for
// them, and two references onto one record are not in conflict. An exclusive
// lock here would be correct and would serialize the whole of capture behind
// one contact.
func TestTwoReferencesOntoOneRecordDoNotWaitForEachOther(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Sindre Vang", "sindre@shared.test", "Vang AS", "shared.test")

	held, release := make(chan struct{}), make(chan struct{})
	holding := make(chan error, 1)
	var announce sync.Once
	go func() {
		holding <- e.store.tx(ctx, func(tx pgx.Tx) error {
			if err := auth.EnsureLinkTarget(ctx, tx, entityPerson, personID.UUID); err != nil {
				return err
			}
			announce.Do(func() { close(held) })
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-holding:
		t.Fatalf("the first probe failed, so this proves nothing about the second: %v", err)
	}

	// The same lock_timeout the archive above trips on. Here it must NOT fire.
	second := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		return auth.EnsureLinkTarget(ctx, tx, entityPerson, personID.UUID)
	})
	close(release)
	if err := <-holding; err != nil {
		t.Fatalf("the first probe failed: %v", err)
	}
	if second != nil {
		t.Fatalf("the second probe got %v, want it to pass while the first still holds the row: "+
			"a shared lock is what keeps concurrent references off each other's queue", second)
	}
}
