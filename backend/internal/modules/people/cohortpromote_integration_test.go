// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The cohort repair over a real Postgres: a person who arrives AFTER their mail
// still ends up with that mail on their record.
//
// This needs a database because everything that makes the repair safe is a
// constraint or a predicate the unit lane cannot see — uq_activity_participant
// refusing a second row for one party, uq_activity_link making a re-run a no-op,
// and the merge redirect deciding which id the link is written under.
//
// The failure it guards is silent and total, and it shipped: a Gmail backfill
// walks newest-first, so the person is created partway through the run and every
// message captured before that moment kept an address-only participant row and no
// link. Nothing errored. The company page just said "no reply for 47 days" about
// somebody who had written that week.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedCapturedMail writes one connector-captured message the way capture writes
// it before any person exists: a counterparty_email on the activity, an
// address-only participant row, and no link at all.
func (e *dedupeEnv) seedCapturedMail(ctx context.Context, t *testing.T, email, role string) ids.ActivityID {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, counterparty_email,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'hi', 'inbound', $2, 'gmail', $3, 'gmail:seed', 'connector:gmail')`,
			activityID, email, activityID.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (id, activity_id, address, role)
			VALUES ($1, $2, $3, $4)`, ids.NewV7(), activityID, email, role)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return activityID
}

// cohortStateOf answers, for one activity, whether it carries a person link and
// whether its participant row names that person — the two halves that must move
// together.
func (e *dedupeEnv) cohortStateOf(ctx context.Context, t *testing.T, activity ids.ActivityID, person ids.PersonID) (linked, named bool) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM activity_link
			                WHERE activity_id = $1 AND entity_type = 'person' AND person_id = $2)`,
			activity, person).Scan(&linked); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM activity_participant
			                WHERE activity_id = $1 AND person_id = $2)`,
			activity, person).Scan(&named)
	}); err != nil {
		t.Fatal(err)
	}
	return linked, named
}

// The ordinary late-person case: three messages are captured while nobody knows
// who the sender is, the person is then created, and all three are on their
// record — not just the one the ensure ran for.
func TestTheCohortRepairFindsMailCapturedBeforeThePersonExisted(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	const email = "late@cohort.test"
	// Captured while the sender was an open question: from-lines and a CC,
	// because a person CC'd on a thread is as stranded as its sender.
	earlier := []ids.ActivityID{
		e.seedCapturedMail(ctx, t, email, "from"),
		e.seedCapturedMail(ctx, t, email, "from"),
		e.seedCapturedMail(ctx, t, email, "cc"),
	}

	// Now the person arrives — by the ensure, on a message of its own.
	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, email, "Late Sender", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !res.PersonCreated {
		t.Fatalf("ensure = %+v, want the person created", res)
	}

	// The ensure alone settles only its own message; the repair is what reaches
	// the rest, and it is what the person event will run.
	var done CohortPromotion
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		done, err = e.store.PromotePersonCohortTx(ctx, tx, res.PersonID)
		return err
	}); err != nil {
		t.Fatalf("cohort repair: %v", err)
	}
	if done.Linked != int64(len(earlier)) {
		t.Errorf("repair linked %d activities, want %d — a message captured before the person is still that person's mail",
			done.Linked, len(earlier))
	}
	for i, activity := range earlier {
		linked, named := e.cohortStateOf(ctx, t, activity, res.PersonID)
		if !linked {
			t.Errorf("earlier message %d has no person link: it is invisible to every reader that reaches an account through activity_link", i)
		}
		if !named {
			t.Errorf("earlier message %d still names the sender only by address: the interaction graph cannot see who was in it", i)
		}
	}

	// Idempotent, because the bus is at-least-once and the verdict path calls
	// this on its own transaction as well.
	var second CohortPromotion
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		second, err = e.store.PromotePersonCohortTx(ctx, tx, res.PersonID)
		return err
	}); err != nil {
		t.Fatalf("second repair: %v", err)
	}
	if second.Linked != 0 || second.Promoted != 0 {
		t.Errorf("second repair did %+v, want nothing left to do — a repair that keeps finding work never terminates", second)
	}
}

// An alias added to an existing contact is a new address, and the mail under it
// is exactly as stranded as a new person's would be.
func TestTheCohortRepairReachesMailUnderAnAliasAddedLater(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "primary@cohort.test", "Two Address", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	const alias = "alias@cohort.test"
	stranded := e.seedCapturedMail(ctx, t, alias, "from")

	// Before the alias is known, the mail under it is nobody's.
	if linked, _ := e.cohortStateOf(ctx, t, stranded, res.PersonID); linked {
		t.Fatal("mail under an unknown alias is already linked; the test proves nothing")
	}

	// Through the module's own email writer, not a hand-built INSERT: the row a
	// test invents is not the row production writes, and this one carries the
	// provenance columns the schema requires.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		ws, err := e.store.db.Workspace(ctx)
		if err != nil {
			return err
		}
		return insertPersonEmails(ctx, tx, ws, res.PersonID, "manual", "human:test",
			[]PersonEmailInput{{Email: alias, EmailType: "work", Position: 1}})
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.PromotePersonCohortTx(ctx, tx, res.PersonID)
		return err
	}); err != nil {
		t.Fatalf("cohort repair: %v", err)
	}

	linked, named := e.cohortStateOf(ctx, t, stranded, res.PersonID)
	if !linked || !named {
		t.Errorf("mail under the alias: linked=%v named=%v, want both — an address the person owns is the person's mail whenever it was learned",
			linked, named)
	}
}

// A message somebody else already owns is not relabelled. The repair infers from
// an ADDRESS, and an inference must not overrule a link a human or a verdict
// placed on the row.
func TestTheCohortRepairLeavesMailThatAlreadyBelongsToSomebody(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	incumbent, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "owner@cohort.test", "Record Owner", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure incumbent: %v", err)
	}
	claimed := e.seedCapturedMail(ctx, t, "shared@cohort.test", "from")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (id, activity_id, entity_type, person_id)
			VALUES ($1, $2, 'person', $3)`, ids.NewV7(), claimed, incumbent.PersonID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// A second person now takes that address — the shared-mailbox shape.
	other, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "shared@cohort.test", "Second Claimant", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure other: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.PromotePersonCohortTx(ctx, tx, other.PersonID)
		return err
	}); err != nil {
		t.Fatalf("cohort repair: %v", err)
	}

	if linked, _ := e.cohortStateOf(ctx, t, claimed, other.PersonID); linked {
		t.Error("the repair relabelled a message that already belonged to another record; an inference from an address must not overrule a placed link")
	}
	if linked, _ := e.cohortStateOf(ctx, t, claimed, incumbent.PersonID); !linked {
		t.Error("the original owner's link is gone")
	}
}

// A merged-away person's cohort belongs to the survivor, because no reader of
// activity_link walks merged_into_id: a link written to the retired id leaves the
// message on a record nobody opens.
func TestTheCohortRepairWritesUnderTheSurvivorOfAMerge(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "survivor@cohort.test", "Survivor", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure survivor: %v", err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "retired@cohort.test", "Retired", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure retired: %v", err)
	}
	stranded := e.seedCapturedMail(ctx, t, "retired@cohort.test", "from")

	// The merge, spelled the way merge.go leaves it: the source redirects, and
	// its addresses come across to the survivor.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE person SET merged_into_id = $2 WHERE id = $1`, retired.PersonID, survivor.PersonID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE person_email SET person_id = $2, is_primary = false WHERE person_id = $1`,
			retired.PersonID, survivor.PersonID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Asked about the RETIRED id, exactly as a replayed event would ask.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.PromotePersonCohortTx(ctx, tx, retired.PersonID)
		return err
	}); err != nil {
		t.Fatalf("cohort repair: %v", err)
	}

	if linked, named := e.cohortStateOf(ctx, t, stranded, survivor.PersonID); !linked || !named {
		t.Errorf("survivor: linked=%v named=%v, want both — a merge redirects the record, and the mail follows it", linked, named)
	}
	if linked, _ := e.cohortStateOf(ctx, t, stranded, retired.PersonID); linked {
		t.Error("the repair wrote a link under the merged-away id, which no reader of activity_link ever follows")
	}
}

// A hand-logged activity carries the links a human chose; an address inference
// must not add to them.
func TestTheCohortRepairLeavesHandLoggedActivitiesAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "typed@cohort.test", "Typed In", "cohort.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	manual := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, counterparty_email, source, captured_by)
			VALUES ($1, 'email', 'logged by hand', 'outbound', $2, 'manual', 'human:x')`,
			manual, "typed@cohort.test")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.PromotePersonCohortTx(ctx, tx, res.PersonID)
		return err
	}); err != nil {
		t.Fatalf("cohort repair: %v", err)
	}
	if linked, _ := e.cohortStateOf(ctx, t, manual, res.PersonID); linked {
		t.Error("the repair linked a hand-logged activity; the cohort is captured mail, and a human's filing is not an address inference to redo")
	}
}
