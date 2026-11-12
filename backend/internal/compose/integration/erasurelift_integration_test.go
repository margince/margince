// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A lift lets the subject's own address be stored again, and nothing else. A
// second erasure arms it again. A link lifts once. A restore still erases what
// the subject had before the lift.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const liftedAddress = "zurueck@betroffene.example"

func TestALiftedAddressMayBeStoredAgain(t *testing.T) {
	e := Setup(t)
	armForLift(t)
	if !addressRefused(t, e) {
		t.Fatal("an erased address was not refused before the lift")
	}

	liftAddress(t, e)

	if addressRefused(t, e) {
		t.Fatal("the address is still refused after the subject lifted it")
	}
}

func TestASecondErasureArmsALiftedAddressAgain(t *testing.T) {
	e := Setup(t)
	armForLift(t)
	liftAddress(t, e)
	returned := contactHolding(t, e, "Die Zurueckgekehrte")

	eraseForLift(t, e, returned)

	if !addressRefused(t, e) {
		t.Fatal("a second erasure left the earlier lift in place")
	}
}

func TestALinkLiftsOneErasureOnce(t *testing.T) {
	e := Setup(t)
	armForLift(t)
	erasedAt := erasedAtOf(t, e, liftedAddress)
	if err := tryLift(e, erasedAt); err != nil {
		t.Fatalf("lifting: %v", err)
	}

	if err := tryLift(e, erasedAt); !errors.Is(err, privacy.ErrLiftNotCurrent) {
		t.Fatalf("a second use of one link answered %v, want ErrLiftNotCurrent", err)
	}
	eraseForLift(t, e, contactHolding(t, e, "Die Zurueckgekehrte"))
	if err := tryLift(e, erasedAt); !errors.Is(err, privacy.ErrLiftNotCurrent) {
		t.Fatalf("a link mailed for the first erasure answered %v against the second, want ErrLiftNotCurrent", err)
	}
}

// A restore brings back the contact the subject had before the erasure. The
// lift is their request to be stored from now on, not a pardon for that record.
// One address is held by one live contact at a time, so the two run in turn.
func TestAReplayErasesWhatCameBackAndKeepsTheSignup(t *testing.T) {
	e := Setup(t)
	eraser := compose.ComposedEraser(e.Pool, nil, nil)
	armForLift(t)
	restored := contactHolding(t, e, "Vor der Loeschung")
	liftAddress(t, e)

	assertReplayKeeps(t, e, eraser, restored, false)
	signup := contactHolding(t, e, "Nach der Rueckkehr")
	assertReplayKeeps(t, e, eraser, signup, true)
}

// The journal carries the lift across a restore that took the list with it.
// A contact from before the lift is erased again.
func TestTheJournalErasesWhatCameFromBeforeTheLift(t *testing.T) {
	e := Setup(t)
	eraser := compose.ComposedEraser(e.Pool, blobstore.NewMemory(), nil)
	armForLift(t)
	restored := contactHolding(t, e, "Vor der Loeschung")
	liftAddress(t, e)
	exportAndRollBack(t, e, eraser)

	assertReplayKeeps(t, e, eraser, restored, false)
}

func TestTheJournalKeepsWhatCameAfterTheLift(t *testing.T) {
	e := Setup(t)
	eraser := compose.ComposedEraser(e.Pool, blobstore.NewMemory(), nil)
	armForLift(t)
	liftAddress(t, e)
	signup := contactHolding(t, e, "Nach der Rueckkehr")
	exportAndRollBack(t, e, eraser)

	assertReplayKeeps(t, e, eraser, signup, true)
}

// A journal exported before the lift is older than the list, and the list wins.
func TestAJournalOlderThanTheLiftDoesNotEraseTheSignup(t *testing.T) {
	e := Setup(t)
	eraser := compose.ComposedEraser(e.Pool, blobstore.NewMemory(), nil)
	armForLift(t)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	liftAddress(t, e)
	signup := contactHolding(t, e, "Nach der Rueckkehr")

	assertReplayKeeps(t, e, eraser, signup, true)
}

// A journal newer than a restored lift holds a second erasure the restore
// rolled back, and the replay applies it.
func TestAJournalNewerThanARestoredLiftErasesAgain(t *testing.T) {
	e := Setup(t)
	eraser := compose.ComposedEraser(e.Pool, blobstore.NewMemory(), nil)
	armForLift(t)
	liftAddress(t, e)
	owner := OwnerConn(t)
	var erasedAt, liftedAt time.Time
	var consent, contact ids.UUID
	hash := storekit.SuppressionHash(liftedAddress)
	if err := owner.QueryRow(context.Background(),
		`SELECT created_at, lifted_at, lift_consent_event_id, lift_contact_id FROM erasure_suppression WHERE value_hash = $1`,
		hash).Scan(&erasedAt, &liftedAt, &consent, &contact); err != nil {
		t.Fatalf("reading the lifted row: %v", err)
	}
	eraseForLift(t, e, contactHolding(t, e, "Die Zurueckgekehrte"))
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	// The restore: the list is back at the lift, and the contact is back.
	if _, err := owner.Exec(context.Background(), `
		UPDATE erasure_suppression SET created_at = $2, lifted_at = $3, lift_consent_event_id = $4, lift_contact_id = $5
		 WHERE value_hash = $1`, hash, erasedAt, liftedAt, consent, contact); err != nil {
		t.Fatalf("restoring the lifted row: %v", err)
	}
	back := contactHolding(t, e, "Wieder da")

	assertReplayKeeps(t, e, eraser, back, false)
}

// The rekey moves a lifted row to its keyed hash with its lift. A subject who
// came back before the key was bound is not refused after it.
func TestTheRekeyKeepsALift(t *testing.T) {
	apptest.BindEnv(t, nil)
	e := Setup(t)
	eraseForLift(t, e, contactHolding(t, e, "Erased Subject"))
	liftAddress(t, e)

	apptest.BindEnv(t, []string{"MARGINCE_KEYVAULT_ROOT_KEY=" + returnRootKey})
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := privacy.RekeySuppressionListPageTx(ctx, tx, privacy.SuppressionCursor{}, 100)
		return err
	}); err != nil {
		t.Fatalf("rekeying the erasure list: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM erasure_suppression WHERE key_id IS NOT NULL AND lifted_at IS NOT NULL`); n != 1 {
		t.Fatalf("%d keyed rows carry the lift after the rekey, want 1", n)
	}
	if addressRefused(t, e) {
		t.Fatal("the rekey dropped the lift, and the address is refused again")
	}
}

// armForLift puts liftedAddress on the list as an erasure from before this test.
func armForLift(t *testing.T) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(context.Background(),
		`INSERT INTO erasure_suppression (kind, value_hash, key_id) VALUES ('email', $1, $2)`,
		storekit.SuppressionHash(liftedAddress), storekit.SuppressionKeyID()); err != nil {
		t.Fatalf("arming the erasure list: %v", err)
	}
}

// contactHolding is a live contact holding liftedAddress, as a restore brings it back.
func contactHolding(t *testing.T, e *Env, name string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, nil)
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', true, 'manual', 'human:restore')`,
		ids.NewV7(), contact, liftedAddress); err != nil {
		t.Fatalf("seeding the address: %v", err)
	}
	return contact
}

func eraseForLift(t *testing.T, e *Env, contact ids.UUID) {
	t.Helper()
	if err := compose.ComposedEraser(e.Pool, nil, nil).EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
}

func exportAndRollBack(t *testing.T, e *Env, eraser *privacy.Eraser) {
	t.Helper()
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	if _, err := OwnerConn(t).Exec(context.Background(), `DELETE FROM erasure_suppression`); err != nil {
		t.Fatalf("rolling the list back: %v", err)
	}
}

// assertReplayKeeps runs the replay and checks whether the contact kept its address.
func assertReplayKeeps(t *testing.T, e *Env, eraser *privacy.Eraser, contact ids.UUID, kept bool) {
	t.Helper()
	if _, err := eraser.ReapplySuppressions(e.Admin()); err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if live := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE contact_id = $1`, contact); (live == 1) != kept {
		t.Fatalf("after the replay the contact holds %d address(es); want kept=%v", live, kept)
	}
}

func addressRefused(t *testing.T, e *Env) bool {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return storekit.RefuseErasedEmail(e.Admin(), tx, "email", liftedAddress)
	})
	if _, isErased := errors.AsType[*storekit.ErasedIdentifierError](err); err != nil && !isErased {
		t.Fatalf("asking the erasure list: %v", err)
	}
	return err != nil
}

func erasedAtOf(t *testing.T, e *Env, email string) time.Time {
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

func liftAddress(t *testing.T, e *Env) {
	t.Helper()
	if err := tryLift(e, erasedAtOf(t, e, liftedAddress)); err != nil {
		t.Fatalf("lifting %s: %v", liftedAddress, err)
	}
}

func tryLift(e *Env, erasedAt time.Time) error {
	return database.WithWorkspaceTx(liftContext(e), e.Pool, func(tx pgx.Tx) error {
		return privacy.LiftEmailSuppressionTx(liftContext(e), tx, liftedAddress, erasedAt, ids.NewV7(), ids.NewV7())
	})
}

func liftContext(e *Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:erasure-lift-test")
}
