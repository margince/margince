// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A promotion locks the contact it merges into before the lead, as a contact
// merge does. It names that contact without a lock, so a merge can retire it
// in between; the promotion then starts again on the survivor.

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promoteResult is one QualifyLead answer handed back from its goroutine.
type promoteResult struct {
	out PromoteOutcome
	err error
}

func (e *promoteConsentEnv) createContactWithEmail(t *testing.T, name, email string) ids.ContactID {
	t.Helper()
	created, err := e.store.CreateContact(e.ctx, CreateContactInput{
		FullName: name,
		Emails:   []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatalf("create contact %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

// mergeTargetAfter creates a contact whose id sorts after source, so LockPair
// takes source first and a holder of the target parks the merge holding it.
func (e *promoteConsentEnv) mergeTargetAfter(t *testing.T, source ids.ContactID) ids.ContactID {
	t.Helper()
	for range 5 {
		target := e.createContactWithEmail(t, "Survivor Contact", "survivor-"+ids.NewV7().String()+"@example.test")
		if bytes.Compare(target.UUID[:], source.UUID[:]) > 0 {
			return target
		}
	}
	t.Fatal("no contact id sorted after the source in five tries")
	return ids.ContactID{}
}

func (e *promoteConsentEnv) promoteInBackground(lead ids.LeadID) <-chan promoteResult {
	done := make(chan promoteResult, 1)
	go func() {
		out, err := e.store.QualifyLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
		done <- promoteResult{out: out, err: err}
	}()
	return done
}

// waiterOn names the one backend in this database waiting on a lock pid holds.
func waiterOn(t *testing.T, probe pgx.Tx, pid int) int {
	t.Helper()
	ctx := context.Background()
	if _, err := probe.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		t.Fatal(err)
	}
	var waiter int
	if err := probe.QueryRow(ctx, `
		SELECT a.pid FROM pg_stat_activity a
		 WHERE a.datname = current_database() AND $1 = ANY (pg_blocking_pids(a.pid))`,
		pid).Scan(&waiter); err != nil {
		t.Fatalf("name the backend waiting on %d: %v", pid, err)
	}
	return waiter
}

// endTx rolls back a holder the test may already have committed.
func endTx(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("roll back the holder: %v", err)
	}
}

func assertLandedOn(t *testing.T, got promoteResult, want ids.ContactID) {
	t.Helper()
	if got.err != nil {
		t.Fatalf("promote: %v (deadlock=%t)", got.err, storekit.IsDeadlock(got.err))
	}
	if !got.out.Merged || ids.UUID(got.out.Contact.Id) != want.UUID {
		t.Fatalf("promote landed on %s merged=%t, want a merge into the survivor %s",
			got.out.Contact.Id, got.out.Merged, want.UUID)
	}
}

// The deadlock the order prevents. The lead is worked from the source contact,
// so the merge rewrites the lead row after it has locked the source. A promote
// that held the lead and then waited on the source closed the cycle.
func TestPromoteRacingAContactMergeBothComplete(t *testing.T) {
	e := setupPromoteConsent(t)
	ctx := context.Background()
	source := e.createContactWithEmail(t, "Source Contact", "racing@example.test")
	target := e.mergeTargetAfter(t, source)
	leadID := seedHistoricalLeadWithEmail(e.ctx, t, e.store, CreateLeadInput{
		FullName: new("Historic lead"), Source: "manual",
	}, "racing@example.test")
	if _, err := e.owner.Exec(ctx, `UPDATE lead SET from_contact_id = $2 WHERE id = $1`, leadID, source); err != nil {
		t.Fatalf("restore the historic contact link: %v", err)
	}

	holder, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { endTx(t, holder) })
	var holderPid int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPid); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `SELECT id FROM contact WHERE id = $1 FOR UPDATE`, target); err != nil {
		t.Fatal(err)
	}

	mergeDone := make(chan error, 1)
	go func() {
		_, err := e.store.MergeContact(e.ctx, source, target, nil)
		mergeDone <- err
	}()
	mustBlockOn(t, holder, holderPid, mergeDone)
	mergePid := waiterOn(t, holder, holderPid)

	promoted := e.promoteInBackground(leadID)
	if got, finished := waitUntilBlocked(t, holder, mergePid, promoted); finished {
		t.Fatalf("the promote finished (%v) without waiting on the merge, so the race was never run", got.err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-mergeDone; err != nil {
		t.Fatalf("merge: %v (deadlock=%t)", err, storekit.IsDeadlock(err))
	}
	assertLandedOn(t, <-promoted, target)
}

// The match moves between the unlocked read and the lock: a merge still open
// moves the lead's address to the survivor. The promotion must not merge into
// the retired contact, nor answer it as missing, but start again on the survivor.
func TestPromoteStartsAgainWhenItsMatchMovesBeforeTheLock(t *testing.T) {
	e := setupPromoteConsent(t)
	ctx := context.Background()
	retired := e.createContactWithEmail(t, "Retired Contact", "moving@example.test")
	survivor := e.createContactWithEmail(t, "Survivor Contact", "kept@example.test")
	lead := e.seedLead(t, "moving@example.test")

	held := make(chan int, 1)
	release := make(chan struct{})
	mergeDone := make(chan error, 1)
	go func() {
		mergeDone <- e.store.tx(e.ctx, func(tx pgx.Tx) error {
			var pid int
			if err := tx.QueryRow(e.ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			if _, err := e.store.mergeContactTx(e.ctx, tx, retired, survivor, nil, nil); err != nil {
				return err
			}
			held <- pid
			<-release
			return nil
		})
	}()
	var mergePid int
	select {
	case mergePid = <-held:
	case err := <-mergeDone:
		t.Fatalf("merge: %v", err)
	}

	probe, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { endTx(t, probe) })
	promoted := e.promoteInBackground(lead)
	if got, finished := waitUntilBlocked(t, probe, mergePid, promoted); finished {
		close(release)
		t.Fatalf("the promote finished (%v) without waiting on the open merge", got.err)
	}
	close(release)
	if err := <-mergeDone; err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertLandedOn(t, <-promoted, survivor)
}
