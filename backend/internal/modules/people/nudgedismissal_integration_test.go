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
	"fmt"
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

// dismissedNow asks the exclusion the way the decay lane does: through the
// predicate the projection embeds, run over the person table.
//
// Asserted through NotDismissedClause rather than through a read written for
// the test, because that clause IS the read side — a helper that queried the
// table directly would agree with itself while the lane used something else.
func dismissedNow(
	ctx context.Context, t *testing.T, e *dedupeEnv, contacts []ids.PersonID, at time.Time,
) map[ids.UUID]bool {
	t.Helper()
	out := map[ids.UUID]bool{}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		// The clause names `e.person_id`, so the subject is aliased `e` here
		// too — the projection's own alias for an edge row.
		keep, err := NotDismissedClause(ctx, "e", at, arg)
		if err != nil {
			return err
		}
		ids_ := make([]ids.UUID, 0, len(contacts))
		for _, c := range contacts {
			ids_ = append(ids_, c.UUID)
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(
			`SELECT e.person_id FROM (SELECT unnest($%d::uuid[]) AS person_id) e
			  WHERE NOT (%s)`, arg(ids_), keep), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out[id] = true
		}
		return rows.Err()
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

// A RE-dismissal images the deadline it displaced.
//
// This is the one case that genuinely replaces something: the first dismissal
// recorded an occurrence, and the second overwrites a moment that was still
// standing. Without the before-image the trail would name the new deadline and
// leave nobody able to say what it replaced — a rep who quietly extended a
// week to a quarter would look the same as one who set the contact aside once.
func TestARedismissalNamesTheDeadlineItReplaced(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)
	if err := e.store.DismissRelationshipNudge(e.as(), person, 3); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	first := latestPersonAudit(t, e, person)["dismissed_until"]

	if err := e.store.DismissRelationshipNudge(e.as(), person, 30); err != nil {
		t.Fatalf("re-dismissing: %v", err)
	}

	before, after := latestPersonAuditPair(t, e, person)
	if before["dismissed_until"] != first {
		t.Errorf("the before image says %v, want the deadline that was standing (%v)",
			before["dismissed_until"], first)
	}
	if after["dismissed_until"] == before["dismissed_until"] {
		t.Error("the images agree, so the row records a replacement of nothing")
	}
}

// latestPersonAuditPair reads BOTH images of the newest update on one person.
func latestPersonAuditPair(t *testing.T, e *dedupeEnv, person ids.PersonID) (before, after map[string]any) {
	t.Helper()
	var beforeJSON, afterJSON []byte
	if err := e.store.db.Pool().QueryRow(e.as(),
		`SELECT before, after FROM audit_log
		  WHERE entity_type = 'person' AND entity_id = $1 AND action = 'update'
		  ORDER BY occurred_at DESC, id DESC LIMIT 1`, person.UUID,
	).Scan(&beforeJSON, &afterJSON); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if beforeJSON == nil {
		t.Fatal("the row carries no before image, so it cannot say what it replaced")
	}
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatalf("the before image is not an object: %v", err)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("the after image is not an object: %v", err)
	}
	return before, after
}

// The database refuses a dismissal that would already have lapsed.
//
// The store's own range check answers first for every caller, and returns
// before SQL runs — so nothing above reaches the CHECK, and removing it from
// the migration would leave every test green. This writes the row the store
// would never build, which is the only way to ask the constraint anything.
//
// It matters because the read side reads `dismissed_until > now()` and never
// wonders whether a row was meant to apply. A row that expired before it was
// written is one nobody can act on and nobody can see.
func TestTheDatabaseRefusesADismissalThatHasAlreadyLapsed(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)

	_, err := e.store.db.Pool().Exec(e.as(), `
		INSERT INTO relationship_nudge_dismissal
		       (person_id, reader_id, dismissed_until, set_by)
		VALUES ($1, $2, now() - interval '1 day', 'test')`,
		person.UUID, e.rep)

	if err == nil {
		t.Fatal("a dismissal expiring before it was written was stored — the read " +
			"would never return it, so nothing else would notice")
	}
	// And the honest row IS accepted, without which the refusal above would pass
	// against a constraint that refused everything.
	if _, err := e.store.db.Pool().Exec(e.as(), `
		INSERT INTO relationship_nudge_dismissal
		       (person_id, reader_id, dismissed_until, set_by)
		VALUES ($1, $2, now() + interval '1 day', 'test')`,
		person.UUID, e.rep); err != nil {
		t.Fatalf("a dismissal ending tomorrow was refused: %v", err)
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
	typ, action := latestPersonEvent(t, e, person)
	if typ != "relationship_nudge.decided" {
		t.Errorf("the announcement is %q, want relationship_nudge.decided", typ)
	}
	if action != "dismissed" {
		t.Errorf("the announcement carries action %q, want dismissed", action)
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
	// The ANNOUNCEMENT too, and its action. The audit row alone would stay green
	// over a restore that emitted nothing, or one that emitted the dismissal's
	// action — and a consumer counting how often reps put relationships down
	// would then count this as another one.
	typ, action := latestPersonEvent(t, e, person)
	if typ != "relationship_nudge.decided" {
		t.Errorf("the announcement is %q, want relationship_nudge.decided", typ)
	}
	if action != "restored" {
		t.Errorf("the announcement carries action %q, want restored", action)
	}
}

// Restoring a dismissal that had already LAPSED announces nothing.
//
// There is no sweep, so an expired row sits in the table until somebody
// restores. Deleting it is right; calling it a judgement is not — the contact
// was already back on the lane, nothing a reader can see changed, and an event
// would report a decision nobody made.
func TestRestoringALapsedDismissalAnnouncesNothing(t *testing.T) {
	e := setupDedupe(t)
	person := aContact(t, e)
	if err := e.store.DismissRelationshipNudge(e.as(), person, 1); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	// Age the whole row into the past, which is what the passage of a day does.
	// BOTH columns move: the CHECK holds dismissed_until after set_at, so a row
	// whose end moved back while its start stayed put is one the table refuses
	// — a shape the passage of time never produces.
	if _, err := e.store.db.Pool().Exec(e.as(), `
		UPDATE relationship_nudge_dismissal
		   SET set_at = now() - interval '2 days',
		       dismissed_until = now() - interval '1 hour'
		 WHERE person_id = $1`, person.UUID); err != nil {
		t.Fatalf("ageing the dismissal: %v", err)
	}
	before := countPersonEvents(t, e, person)

	if err := e.store.RestoreRelationshipNudge(e.as(), person); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if got := countPersonEvents(t, e, person); got != before {
		t.Errorf("restoring a lapsed dismissal announced %d event(s) — the contact "+
			"was already back on the lane, so nothing was decided", got-before)
	}
	// And the row is gone, because nothing else would ever remove it.
	var left int
	if err := e.store.db.Pool().QueryRow(e.as(),
		`SELECT count(*) FROM relationship_nudge_dismissal WHERE person_id = $1`,
		person.UUID).Scan(&left); err != nil {
		t.Fatalf("counting what is left: %v", err)
	}
	if left != 0 {
		t.Errorf("the lapsed row survived the restore, and no sweep will take it")
	}
}

// countPersonEvents is how many announcements this contact has drawn.
func countPersonEvents(t *testing.T, e *dedupeEnv, person ids.PersonID) int {
	t.Helper()
	var n int
	if err := e.store.db.Pool().QueryRow(e.as(),
		`SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1`,
		person.UUID.String()).Scan(&n); err != nil {
		t.Fatalf("counting outbox rows: %v", err)
	}
	return n
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

// latestPersonEvent reads the type and action of the newest outbox row.
func latestPersonEvent(t *testing.T, e *dedupeEnv, person ids.PersonID) (string, string) {
	t.Helper()
	// The type rides INSIDE the envelope; the outbox row itself carries only a
	// stream, a payload and its ordering columns.
	var eventType, action string
	if err := e.store.db.Pool().QueryRow(e.as(),
		`SELECT envelope->>'type', envelope->'payload'->>'action' FROM event_outbox
		  WHERE envelope->'entity'->>'id' = $1 ORDER BY seq DESC LIMIT 1`,
		person.UUID.String(),
	).Scan(&eventType, &action); err != nil {
		t.Fatalf("reading the outbox row: %v", err)
	}
	return eventType, action
}
