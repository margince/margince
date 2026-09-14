// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The acquisition is dated from the message, not from the write.
//
// Everything here is about one column, contact_acquisition_evidence.occurred_at,
// and one consequence: compose/noticecaseopen.go runs the Art. 14 disclosure
// deadline from `coalesce(occurred_at, captured_at)`. A capture that dates the
// acquisition from its own write gives a contact acquired in March, captured in
// September, a deadline in October — six months late, and showing as a month
// away.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedDatedActivity writes one captured message that happened at a stated time,
// and answers an ensure input naming it.
func (e *dedupeEnv) datedEnsureInput(
	ctx context.Context, t *testing.T, email, domain string, occurred time.Time,
) EnsureCounterpartyInput {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, occurred_at,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'hi', 'inbound', $2, 'gmail', $3, 'gmail:seed', 'connector:gmail')`,
			activityID, occurred, activityID.String())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The participant row is what acquisitionTimeFor reads: participation is
	// recorded by address for every captured message, which is how the earliest
	// one is found regardless of the order they were processed in.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, address)
			VALUES ($1, 'from', $2)`, activityID, email)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return EnsureCounterpartyInput{
		Email: email, Domain: domain,
		OwnerID: e.rep, ActivityID: activityID,
		Source: "gmail:" + activityID.String(), CapturedBy: "connector:gmail",
		Replied: true,
	}
}

// TestTheAcquisitionIsDatedFromTheEarliestMessageNotTheFirstProcessed is the
// defect Codex found in the first version of this change, and the reason
// acquisitionTimeFor reads the captured history rather than the one activity
// that triggered the creation.
//
// Two things make the triggering message arbitrary. A backfill walks a mailbox
// in whatever order the provider returns, with no chronological sort. And an
// ambiguous sender's pending row is written ON CONFLICT DO NOTHING, so it keeps
// the FIRST activity seen and later messages do not replace it — the verdict
// that eventually creates the contact forwards that retained id.
//
// So a contact whose oldest captured message is from last year can be created
// from a message captured out of order this month. Dating from the trigger
// would give that duty a deadline eleven months fresher than the truth, which
// is the direction a compliance deadline must never be wrong in.
func TestTheAcquisitionIsDatedFromTheEarliestMessageNotTheFirstProcessed(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "outoforder@backfill.test"
	oldest := time.Now().Add(-300 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	newest := time.Now().Add(-2 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	// The OLD message is captured, but it is not the one the ensure names —
	// exactly the shape a backfill produces when the provider hands back the
	// recent page first.
	e.datedEnsureInput(ctx, t, email, "backfill.test", oldest)
	trigger := e.datedEnsureInput(ctx, t, email, "backfill.test", newest)

	res, err := e.store.EnsureCounterparty(ctx, trigger)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}

	_, occurred := acquisitionOf(ctx, t, e.store, res.ContactID)
	if occurred == nil {
		t.Fatal("the acquisition has no time")
	}
	if !occurred.Equal(oldest) {
		t.Errorf("the acquisition is dated %v, want the earliest captured message (%v). "+
			"Dating it from the message that happened to trigger the creation makes the "+
			"Art. 14 deadline depend on the order a backfill walked the mailbox in.",
			*occurred, oldest)
	}
}

// acquisitionOf reads the evidence row a creation door left.
func acquisitionOf(ctx context.Context, t *testing.T, s *Store, id ids.ContactID) (string, *time.Time) {
	t.Helper()
	var kind string
	var occurred *time.Time
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT kind, occurred_at FROM contact_acquisition_evidence
			 WHERE contact_id = $1`, id).Scan(&kind, &occurred)
	}); err != nil {
		t.Fatalf("reading the acquisition evidence: %v", err)
	}
	return kind, occurred
}

// TestACapturedContactIsAcquiredWhenTheMessageArrived is the whole point. A
// backfill connecting a mailbox with a year of history creates contacts today
// from messages that arrived months ago, and every one of those acquisitions
// happened when the message did.
func TestACapturedContactIsAcquiredWhenTheMessageArrived(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	// Truncated because Postgres stores microseconds and Go carries
	// nanoseconds; an untruncated comparison fails on the round trip rather
	// than on the behaviour.
	arrived := time.Now().Add(-300 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	res, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "wrote@lastyear.test", "lastyear.test", arrived))
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}

	kind, occurred := acquisitionOf(ctx, t, e.store, res.ContactID)
	if kind != AcquiredSubjectInitiated {
		t.Errorf("a contact who wrote to us is recorded as %q, want %q", kind, AcquiredSubjectInitiated)
	}
	if occurred == nil {
		t.Fatal("the acquisition has no time: the Art. 14 deadline then runs from this " +
			"write rather than from the message, so a contact acquired last year is owed a " +
			"disclosure dated a month from today")
	}
	if !occurred.Equal(arrived) {
		t.Errorf("the acquisition is dated %v, want the message's own time %v", *occurred, arrived)
	}
}

// TestEveryCapturedActivityCarriesATimeToDateTheAcquisitionFrom.
//
// This is the invariant that makes the dating reliable rather than
// best-effort: activity.occurred_at is NOT NULL, so a contact created from a
// captured message ALWAYS gets a dated acquisition. There is no ordinary path
// that leaves it NULL, which is why acquiredwhen.go's nil answer is documented
// as nearly unreachable rather than as a case it handles.
//
// Written as a test rather than trusted from the catalog, because a later
// migration making the column nullable would silently turn every capture back
// into a write-dated acquisition — the exact failure this slice exists to end,
// and one nothing else would report.
func TestEveryCapturedActivityCarriesATimeToDateTheAcquisitionFrom(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	in := e.ensureInput(ctx, t, "dated@always.test", "", "always.test")
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE activity SET occurred_at = NULL WHERE id = $1`, in.ActivityID)
		return err
	})
	if err == nil {
		t.Fatal("an activity accepted a NULL occurred_at: every captured contact was " +
			"dated from its message because that column cannot be empty, and a nullable " +
			"one silently returns every capture to being dated from its own write")
	}

	// And the ordinary path still produces a dated acquisition.
	res, ensureErr := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "second@always.test", "", "always.test"))
	if ensureErr != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, ensureErr)
	}
	if _, occurred := acquisitionOf(ctx, t, e.store, res.ContactID); occurred == nil {
		t.Error("a captured contact has an undated acquisition")
	}
}
