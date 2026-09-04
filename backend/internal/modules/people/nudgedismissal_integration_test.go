// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Setting a lapsed contact aside, against a real database.
//
// Every promise here is one only Postgres can answer: the row's own CHECK, the
// upsert that replaces a moment rather than colliding, the expiry applied in
// the read, and the audit and outbox rows the write shape owes.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aContact is one person for a dismissal to be about. The name is fixed because
// no case here turns on it — what varies between cases is who dismissed whom.
func aContact(t *testing.T, e *dedupeEnv) ids.PersonID {
	t.Helper()
	person, err := e.store.CreatePerson(e.as(),
		CreatePersonInput{FullName: "Dana Weiss", Source: "test"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	return ids.From[ids.PersonKind](ids.UUID(person.Id))
}

// dismissedNow asks the read side, in its own transaction, the way the decay
// seam does.
func dismissedNow(
	ctx context.Context, t *testing.T, e *dedupeEnv, people []ids.PersonID, at time.Time,
) map[ids.UUID]bool {
	t.Helper()
	var out map[ids.UUID]bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		got, err := e.store.DismissedNudges(ctx, tx, people, at)
		out = got
		return err
	}); err != nil {
		t.Fatalf("reading dismissals: %v", err)
	}
	return out
}

// The whole point: a contact the rep put down is off their lane.
func TestADismissedContactIsOffTheReadersLane(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	if err := e.store.DismissRelationshipNudge(e.as(), person, 30); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	if !dismissedNow(e.as(), t, e, []ids.PersonID{person}, time.Now())[person.UUID] {
		t.Fatal("a contact the rep set aside is still on their lane")
	}
}

// And comes back on the moment the rep chose, without any sweep having run.
//
// Expiry lives in the read's own predicate: a dismissal that has run out is
// simply not returned. A job would put a contact's return at the mercy of when
// that job last ran, which is not the moment the rep picked.
func TestADismissedContactReturnsWhenItsMomentPasses(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	if err := e.store.DismissRelationshipNudge(e.as(), person, 7); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	// Read as of a moment past the span, rather than by waiting or by moving a
	// clock: the read takes the instant, so the passage of time is an argument.
	past := time.Now().AddDate(0, 0, 8)
	if dismissedNow(e.as(), t, e, []ids.PersonID{person}, past)[person.UUID] {
		t.Fatal("a dismissal outlived the moment the rep chose")
	}
	// And is still down before it, so the case above is the expiry rather than
	// a read that never finds anything.
	soon := time.Now().AddDate(0, 0, 6)
	if !dismissedNow(e.as(), t, e, []ids.PersonID{person}, soon)[person.UUID] {
		t.Fatal("the dismissal lapsed before the moment the rep chose")
	}
}

// It binds ONE reader. A rep deciding not to chase somebody is judging their
// own morning, and taking that contact off a colleague's lane would remove work
// from a queue whose owner never made the call.
func TestADismissalBindsOnlyTheReaderWhoMadeIt(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	if err := e.store.DismissRelationshipNudge(e.as(), person, 30); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	if dismissedNow(e.asOther(), t, e, []ids.PersonID{person}, time.Now())[person.UUID] {
		t.Fatal("one rep's decision took the contact off a colleague's lane")
	}
}

// Restoring puts them back, and does so now rather than at the moment the
// dismissal named.
func TestRestoringPutsTheContactBackImmediately(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)
	if err := e.store.DismissRelationshipNudge(e.as(), person, 30); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	if err := e.store.RestoreRelationshipNudge(e.as(), person); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if dismissedNow(e.as(), t, e, []ids.PersonID{person}, time.Now())[person.UUID] {
		t.Fatal("a restored contact is still off the lane")
	}
}

// Restoring somebody nobody dismissed is the same success: the reader's goal
// state already holds.
func TestRestoringAContactNobodyDismissedSucceeds(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	if err := e.store.RestoreRelationshipNudge(e.as(), person); err != nil {
		t.Fatalf("restoring a contact who was never set aside: %v", err)
	}
}

// Dismissing again replaces the moment. A rep who set somebody aside for a week
// and then wants a month is saying one thing, not colliding with themselves.
func TestDismissingAgainReplacesTheMoment(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)
	if err := e.store.DismissRelationshipNudge(e.as(), person, 3); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	if err := e.store.DismissRelationshipNudge(e.as(), person, 30); err != nil {
		t.Fatalf("re-dismissing: %v", err)
	}

	// Past the FIRST span and inside the second, which is where the two answers
	// differ: a write that collided or was ignored leaves the contact back.
	between := time.Now().AddDate(0, 0, 10)
	if !dismissedNow(e.as(), t, e, []ids.PersonID{person}, between)[person.UUID] {
		t.Fatal("the second dismissal did not extend the first")
	}
}

// A span outside the published range is refused rather than clamped: a caller
// asking for a year has made a mistake, and quietly storing a quarter hands
// them a moment they will read as the one they asked for.
func TestASpanOutsideTheRangeIsRefused(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	for _, days := range []int{0, -1, nudgeDismissalMaxDays + 1} {
		if err := e.store.DismissRelationshipNudge(e.as(), person, days); err == nil {
			t.Errorf("a %d-day dismissal was accepted", days)
		}
	}
	// And the boundary itself is admitted, without which the refusals above
	// pass against a range that refuses everything.
	if err := e.store.DismissRelationshipNudge(e.as(), person, nudgeDismissalMaxDays); err != nil {
		t.Fatalf("the longest published span was refused: %v", err)
	}
}

// The write shape: domain row, audit row and outbox row in ONE transaction.
func TestADismissalWritesTheAuditAndTheAnnouncement(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	if err := e.store.DismissRelationshipNudge(e.as(), person, 14); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	after := latestPersonAudit(t, e, person)
	if after["nudge_dismissal"] != "dismissed" {
		t.Errorf("the audit row says %v, want the judgement that was made", after["nudge_dismissal"])
	}
	if after["dismissed_until"] == nil {
		t.Error("the audit row names no moment, so nothing says when the contact returns")
	}
	if got := latestPersonEvent(t, e, person); got != "relationship_nudge.dismissed" {
		t.Errorf("the announcement is %q, want relationship_nudge.dismissed", got)
	}
}

// The undo is itself a decision, carried rather than left to be inferred from a
// row going quiet.
func TestARestoreIsAnnouncedRatherThanInferred(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)
	if err := e.store.DismissRelationshipNudge(e.as(), person, 14); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	if err := e.store.RestoreRelationshipNudge(e.as(), person); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	after := latestPersonAudit(t, e, person)
	if after["nudge_dismissal"] != "restored" {
		t.Errorf("the audit row says %v, want the restore", after["nudge_dismissal"])
	}
	// A restore takes effect now, so it names no moment.
	if after["dismissed_until"] != nil {
		t.Errorf("the restore named a moment: %v", after["dismissed_until"])
	}
}

// latestPersonAudit reads the after-image of the newest update on one person.
func latestPersonAudit(t *testing.T, e *dedupeEnv, person ids.PersonID) map[string]any {
	t.Helper()
	var raw []byte
	if err := e.store.db.Pool().QueryRow(e.as(),
		`SELECT after FROM audit_log
		  WHERE entity_type = 'person' AND entity_id = $1 AND action = 'update'
		  ORDER BY occurred_at DESC, id DESC LIMIT 1`, person.UUID,
	).Scan(&raw); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	var after map[string]any
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatalf("the after image is not an object: %v", err)
	}
	return after
}

// latestPersonEvent reads the type of the newest outbox row for one person.
func latestPersonEvent(t *testing.T, e *dedupeEnv, person ids.PersonID) string {
	t.Helper()
	// The type rides INSIDE the envelope; the outbox row itself carries only a
	// stream, a payload and its ordering columns.
	var eventType string
	if err := e.store.db.Pool().QueryRow(e.as(),
		`SELECT envelope->>'type' FROM event_outbox
		  WHERE envelope->'entity'->>'id' = $1 ORDER BY seq DESC LIMIT 1`,
		person.UUID.String(),
	).Scan(&eventType); err != nil {
		t.Fatalf("reading the outbox row: %v", err)
	}
	return eventType
}
