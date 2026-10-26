// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// StrengthForContactsAsOf over a real Postgres: what §4 WOULD have said at a
// past instant. It is what "this relationship is going cold" compares against
// — today's score against the same contact's score a month ago — and it does
// it without a snapshot table.
//
// The two properties that make it usable, and the one that makes it honest:
//
//   - it excludes interactions that happened AFTER the instant asked about,
//     or every past score would equal today's and no decay would ever show;
//   - it still reads the window relative to that instant, so a contact who
//     was busy then scores as busy then;
//   - and it is a counterfactual over TODAY's corpus, not a recording. An
//     erased interaction is absent from the past answer too, which is the
//     behaviour we want: a score must not become a way to read deleted data.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// seedInteraction writes one qualifying activity linked to a contact at a
// chosen moment.
func (e *dedupeEnv) seedInteraction(t *testing.T, contactID ids.ContactID, at time.Time, direction string) {
	t.Helper()
	e.seedInteractionOfKind(t, contactID, "email", at, direction)
}

// seedInteractionOfKind is seedInteraction for a chosen activity kind.
func (e *dedupeEnv) seedInteractionOfKind(t *testing.T, contactID ids.ContactID, kind string, at time.Time, direction string) {
	t.Helper()
	id := ids.New[ids.ActivityKind]()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, occurred_at, source_system, source_id, source, captured_by)
			VALUES ($1, $5, 'Hallo', $2, $3, 'gmail', $4, 'gmail:seed', 'connector:gmail')`,
			id, direction, at, id.String(), kind); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, contactID)
		if err != nil || direction != "inbound" {
			return err
		}
		// Capture names the contact as the sender of what they wrote.
		_, err = tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'from', $2)`, id, contactID)
		return err
	}); err != nil {
		t.Fatalf("seeding an interaction: %v", err)
	}
}

// seedVisibleContact writes one workspace-visible contact.
func (e *dedupeEnv) seedVisibleContact(t *testing.T, name string) ids.ContactID {
	t.Helper()
	id := ids.New[ids.ContactKind]()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO contact (id, full_name, owner_id, source, captured_by, visibility)
			VALUES ($1, $2, $3, 'manual', 'human:test', 'workspace')`,
			id, name, e.rep)
		return err
	}); err != nil {
		t.Fatalf("seeding contact %s: %v", name, err)
	}
	return id
}

// strengthAt reads one contact's score at an instant, through whichever entry
// point the caller names.
func (e *dedupeEnv) strengthAt(t *testing.T, contactID ids.ContactID, at time.Time, asOf bool) relstrengthResult {
	t.Helper()
	var out relstrengthResult
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var got []ContactStrength
		var err error
		if asOf {
			got, err = StrengthForContactsAsOf(ctx, tx, []ids.ContactID{contactID}, at)
		} else {
			got, err = StrengthForContacts(ctx, tx, []ids.ContactID{contactID}, at)
		}
		if err != nil {
			return err
		}
		if len(got) != 1 {
			t.Fatalf("scored %d contacts, want 1", len(got))
		}
		out = relstrengthResult{
			strength: got[0].Strength.Strength,
			bucket:   got[0].Strength.Bucket,
			count90d: got[0].Strength.InteractionCount90d,

			lastInteraction: got[0].Strength.LastInteraction,
		}
		return nil
	}); err != nil {
		t.Fatalf("reading strength: %v", err)
	}
	return out
}

type relstrengthResult struct {
	strength int
	bucket   string
	count90d int

	lastInteraction *time.Time
}

func TestStrengthAsOfExcludesWhatHadNotHappenedYet(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC()
	contact := e.seedVisibleContact(t, "Cold Contact")

	// A burst of two-way traffic ending 40 days ago, then silence.
	for i := range 12 {
		direction := "inbound"
		if i%2 == 0 {
			direction = "outbound"
		}
		e.seedInteraction(t, contact, now.AddDate(0, 0, -40-i), direction)
	}

	// Then, as of 35 days ago, the relationship was live.
	then := e.strengthAt(t, contact, now.AddDate(0, 0, -35), true)
	if then.count90d != 12 {
		t.Errorf("as-of 35 days ago counted %d interactions in window, want 12", then.count90d)
	}
	if then.bucket == relstrength.BucketNone || then.strength == 0 {
		t.Fatalf("as-of 35 days ago scored %d (%s); the relationship was active then", then.strength, then.bucket)
	}

	// Today it has decayed. If the as-of read did not exclude later
	// interactions this comparison would be meaningless — which is the whole
	// point of the going-cold detector that consumes it.
	today := e.strengthAt(t, contact, now, false)
	if today.strength >= then.strength {
		t.Errorf("today's score %d is not below the %d it was 35 days ago; the relationship has gone quiet and the comparison cannot see it",
			today.strength, then.strength)
	}
}

func TestStrengthAsOfIgnoresInteractionsAfterTheInstantAsked(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC()
	contact := e.seedVisibleContact(t, "Recent Contact")

	// Everything happened in the last week.
	for i := range 10 {
		e.seedInteraction(t, contact, now.AddDate(0, 0, -i), "inbound")
	}

	// Asked about a month ago, the answer must be "we had never spoken" —
	// not today's score with an older label on it.
	before := e.strengthAt(t, contact, now.AddDate(0, 0, -30), true)
	if before.bucket != relstrength.BucketNone {
		t.Errorf("as-of a month before first contact = %d (%s), want the %q bucket — those interactions had not happened yet",
			before.strength, before.bucket, relstrength.BucketNone)
	}
	if before.count90d != 0 {
		t.Errorf("as-of a month before first contact counted %d interactions, want 0", before.count90d)
	}
}

func TestAnInteractionDatedAfterNowIsNotTheLatestTouch(t *testing.T) {
	e := setupDedupe(t)
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contact := e.seedVisibleContact(t, "Scheduled Contact")

	pastEmail := now.AddDate(0, 0, -20)
	e.seedInteraction(t, contact, pastEmail, "inbound")
	e.seedInteractionOfKind(t, contact, "meeting", now.AddDate(0, 0, 30), "outbound")

	live := e.strengthAt(t, contact, now, false)
	if live.count90d != 1 {
		t.Errorf("the live score counted %d interactions, want 1: the meeting next month has not happened", live.count90d)
	}
	if live.lastInteraction == nil || !live.lastInteraction.Equal(pastEmail) {
		t.Errorf("recency came from %v, want the past email at %v", live.lastInteraction, pastEmail)
	}

	asOf := e.strengthAt(t, contact, now, true)
	if asOf.lastInteraction == nil || !asOf.lastInteraction.Equal(pastEmail) {
		t.Errorf("as-of recency came from %v, want the past email at %v", asOf.lastInteraction, pastEmail)
	}
}

// The single-contact fold and the change fold bound at the same clock as the
// batch fold. An interaction dated at now has happened and counts.
func TestSingleContactAndChangeFoldsBoundAtNow(t *testing.T) {
	e := setupDedupe(t)
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contact := e.seedVisibleContact(t, "Single Fold Contact")
	pastEmail := now.AddDate(0, 0, -20)
	e.seedInteraction(t, contact, pastEmail, "inbound")
	e.seedInteraction(t, contact, now, "outbound")
	e.seedInteractionOfKind(t, contact, "meeting", now.Add(time.Hour), "outbound")

	ctx := e.as()
	single, err := e.store.ContactStrength(ctx, contact, now)
	if err != nil {
		t.Fatalf("reading one contact's strength: %v", err)
	}
	if single.InteractionCount90d != 2 {
		t.Errorf("the single-contact fold counted %d, want 2: the email and the interaction dated exactly now", single.InteractionCount90d)
	}
	if single.LastInteraction == nil || !single.LastInteraction.Equal(now) {
		t.Errorf("single-contact recency came from %v, want %v", single.LastInteraction, now)
	}
	if len(single.ContributingIDs) != 2 {
		t.Errorf("the single-contact fold cites %d interactions, want 2", len(single.ContributingIDs))
	}

	var in relstrength.ChangeInputs
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		in, err = changeInputs(ctx, tx, contact, now, nil)
		return err
	}); err != nil {
		t.Fatalf("reading the change inputs: %v", err)
	}
	if in.Current.Count90d != 2 {
		t.Errorf("the change fold counted %d, want 2", in.Current.Count90d)
	}
	if in.Current.LastInteraction == nil || !in.Current.LastInteraction.Equal(now) {
		t.Errorf("change-fold recency came from %v, want %v", in.Current.LastInteraction, now)
	}
}
