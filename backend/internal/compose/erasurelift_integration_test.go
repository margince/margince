// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lift lets the person's own address be stored again, and nothing else: a
// second erasure arms it again, a link lifts once, and a restore still erases
// what the person had before the lift.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const liftedAddress = "zurueck@betroffene.example"

func TestALiftedAddressMayBeStoredAgain(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, liftedAddress)
	if !addressRefused(t, e, liftedAddress) {
		t.Fatal("an erased address was not refused before the lift")
	}

	liftAddress(t, e, liftedAddress)

	if addressRefused(t, e, liftedAddress) {
		t.Fatal("the address is still refused after the person lifted it")
	}
}

func TestASecondErasureArmsALiftedAddressAgain(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, liftedAddress)
	liftAddress(t, e, liftedAddress)
	returned := seedContactWithEmail(t, e, "Die Zurueckgekehrte", liftedAddress)

	if err := eraserWithGraph(e.DB()).EraseContact(e.Admin(), returned, "subject request"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}

	if !addressRefused(t, e, liftedAddress) {
		t.Fatal("a second erasure left the earlier lift in place")
	}
}

func TestALinkLiftsOneErasureOnce(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, liftedAddress)
	erasedAt := erasedAtOf(t, e, liftedAddress)
	liftAt(t, e, liftedAddress, erasedAt)

	if err := tryLift(e, liftedAddress, erasedAt); !errors.Is(err, privacy.ErrLiftNotCurrent) {
		t.Fatalf("a second use of one link answered %v, want ErrLiftNotCurrent", err)
	}

	returned := seedContactWithEmail(t, e, "Die Zurueckgekehrte", liftedAddress)
	if err := eraserWithGraph(e.DB()).EraseContact(e.Admin(), returned, "subject request"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
	if err := tryLift(e, liftedAddress, erasedAt); !errors.Is(err, privacy.ErrLiftNotCurrent) {
		t.Fatalf("a link mailed for the first erasure answered %v against the second, want ErrLiftNotCurrent", err)
	}
}

// A restore brings back the contact the person had before the erasure. The
// lift is their request to be stored from now on, not a pardon for that record.
// One address is held by one live contact at a time, so the two run in turn.
func TestAReplayErasesWhatCameBackAndKeepsTheSignup(t *testing.T) {
	e := integration.Setup(t)
	eraser := eraserWithGraph(InstallationDB(e.Pool))
	seedSuppressedEmail(t, liftedAddress)
	restored := seedContactWithEmail(t, e, "Vor der Loeschung", liftedAddress)
	liftAddress(t, e, liftedAddress)

	assertReplayKeeps(t, e, eraser, restored, false)
	signup := seedContactWithEmail(t, e, "Nach der Rueckkehr", liftedAddress)
	assertReplayKeeps(t, e, eraser, signup, true)
}

// The journal carries the lift across a restore that took the list with it: a
// contact from before the lift is erased again, and one from after it is kept.
func TestTheJournalErasesWhatCameFromBeforeTheLift(t *testing.T) {
	e := integration.Setup(t)
	eraser := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory())
	seedSuppressedEmail(t, liftedAddress)
	restored := seedContactWithEmail(t, e, "Vor der Loeschung", liftedAddress)
	liftAddress(t, e, liftedAddress)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	clearSuppressionList(t)

	assertReplayKeeps(t, e, eraser, restored, false)
}

func TestTheJournalKeepsWhatCameAfterTheLift(t *testing.T) {
	e := integration.Setup(t)
	eraser := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory())
	seedSuppressedEmail(t, liftedAddress)
	liftAddress(t, e, liftedAddress)
	signup := seedContactWithEmail(t, e, "Nach der Rueckkehr", liftedAddress)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	clearSuppressionList(t)

	assertReplayKeeps(t, e, eraser, signup, true)
}

// assertReplayKeeps runs the replay and checks whether the contact kept its address.
func assertReplayKeeps(t *testing.T, e *integration.Env, eraser *privacy.Eraser, contact ids.UUID, kept bool) {
	t.Helper()
	if _, err := eraser.ReapplySuppressions(e.Admin()); err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if live := liveEmails(t, e, contact); (live == 1) != kept {
		t.Fatalf("after the replay the contact holds %d address(es); want kept=%v", live, kept)
	}
}

// A journal exported before the lift is older than the list, and the list wins.
func TestAJournalOlderThanTheLiftDoesNotEraseTheSignup(t *testing.T) {
	e := integration.Setup(t)
	eraser := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory())
	seedSuppressedEmail(t, liftedAddress)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	liftAddress(t, e, liftedAddress)
	signup := seedContactWithEmail(t, e, "Nach der Rueckkehr", liftedAddress)

	if _, err := eraser.ReapplySuppressions(e.Admin()); err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if liveEmails(t, e, signup) != 1 {
		t.Fatal("a journal exported before the lift erased the person's signup")
	}
}

// A journal newer than a restored lift holds a second erasure the restore
// rolled back, and the replay applies it.
func TestAJournalNewerThanARestoredLiftErasesAgain(t *testing.T) {
	e := integration.Setup(t)
	eraser := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory())
	seedSuppressedEmail(t, liftedAddress)
	liftAddress(t, e, liftedAddress)
	var restoredList struct {
		erasedAt, liftedAt time.Time
		consent            ids.UUID
	}
	owner := integration.OwnerConn(t)
	if err := owner.QueryRow(context.Background(),
		`SELECT created_at, lifted_at, lift_consent_event_id FROM erasure_suppression WHERE value_hash = $1`,
		storekit.SuppressionHash(liftedAddress)).Scan(&restoredList.erasedAt, &restoredList.liftedAt, &restoredList.consent); err != nil {
		t.Fatalf("reading the lifted row: %v", err)
	}
	returned := seedContactWithEmail(t, e, "Die Zurueckgekehrte", liftedAddress)
	if err := eraserWithGraph(e.DB()).EraseContact(e.Admin(), returned, "subject request"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	// The restore: the list is back at the lift, and the contact is back.
	if _, err := owner.Exec(context.Background(), `
		UPDATE erasure_suppression SET created_at = $2, lifted_at = $3, lift_consent_event_id = $4 WHERE value_hash = $1`,
		storekit.SuppressionHash(liftedAddress), restoredList.erasedAt, restoredList.liftedAt, restoredList.consent); err != nil {
		t.Fatalf("restoring the lifted row: %v", err)
	}
	back := seedContactWithEmail(t, e, "Wieder da", liftedAddress)

	if _, err := eraser.ReapplySuppressions(e.Admin()); err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if liveEmails(t, e, back) != 0 {
		t.Fatal("the second erasure in the journal was lost to the older lift in the restored list")
	}
}

func addressRefused(t *testing.T, e *integration.Env, email string) bool {
	t.Helper()
	var refused error
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		refused = storekit.RefuseErasedEmail(e.Admin(), tx, "email", email)
		return nil
	}); err != nil {
		t.Fatalf("asking the erasure list: %v", err)
	}
	if _, isErased := errors.AsType[*storekit.ErasedIdentifierError](refused); refused != nil && !isErased {
		t.Fatalf("asking the erasure list: %v", refused)
	}
	return refused != nil
}

func erasedAtOf(t *testing.T, e *integration.Env, email string) time.Time {
	t.Helper()
	var erasedAt time.Time
	var erased bool
	if err := database.WithWorkspaceTx(liftContext(e), e.Pool, func(tx pgx.Tx) error {
		var err error
		erasedAt, erased, err = privacy.EmailErasedAt(liftContext(e), tx, email)
		return err
	}); err != nil || !erased {
		t.Fatalf("reading when %s was erased: erased=%v err=%v", email, erased, err)
	}
	return erasedAt
}

func liftAddress(t *testing.T, e *integration.Env, email string) {
	t.Helper()
	liftAt(t, e, email, erasedAtOf(t, e, email))
}

func liftAt(t *testing.T, e *integration.Env, email string, erasedAt time.Time) {
	t.Helper()
	if err := tryLift(e, email, erasedAt); err != nil {
		t.Fatalf("lifting %s: %v", email, err)
	}
}

func tryLift(e *integration.Env, email string, erasedAt time.Time) error {
	return database.WithWorkspaceTx(liftContext(e), e.Pool, func(tx pgx.Tx) error {
		return privacy.LiftEmailSuppressionTx(liftContext(e), tx, email, erasedAt, ids.NewV7())
	})
}

func liftContext(e *integration.Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:erasure-lift-test")
}

// The rekey moves a lifted row to its keyed hash with its lift, so a person who
// came back before the key was bound is not refused after it.
func TestTheRekeyKeepsALift(t *testing.T) {
	e := integration.Setup(t)
	useRootKey(t, "")
	subject := keyedRecipient(t, e, "Erased Subject", liftedAddress)
	if err := ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), subject, "test"); err != nil {
		t.Fatal(err)
	}
	liftAddress(t, e, liftedAddress)

	useRootKey(t, firstRootKey)
	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, keyedIdentifiersDataTask())

	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NOT NULL AND lifted_at IS NOT NULL`); n != 1 {
		t.Fatalf("%d keyed rows carry the lift after the rekey, want 1", n)
	}
	if addressRefused(t, e, liftedAddress) {
		t.Fatal("the rekey dropped the lift, and the address is refused again")
	}
}
