// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The strength dates a contact stores are the live fold of its history after
// every kind of write. They come down when their source is archived,
// restricted, held, erased or moves to another contact.

import (
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedDates is what a contact row stores, read back as the fold reports it.
type storedDates struct {
	folded                                  bool
	interaction, engaged, inbound, outbound *time.Time
	inboundPrecision, outboundPrecision     *string
}

func (e *dedupeEnv) storedDatesOf(t *testing.T, contactID ids.ContactID) storedDates {
	t.Helper()
	var out storedDates
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT strength_folded, strength_last_interaction_at, strength_last_engaged_at,
			       strength_last_inbound_at, `+storedPrecisionSQL("strength_last_inbound_day")+`,
			       strength_last_outbound_at, `+storedPrecisionSQL("strength_last_outbound_day")+`
			  FROM contact WHERE id = $1`, contactID).Scan(&out.folded, &out.interaction, &out.engaged,
			&out.inbound, &out.inboundPrecision, &out.outbound, &out.outboundPrecision)
	}); err != nil {
		t.Fatalf("reading the stored strength dates: %v", err)
	}
	return out
}

// bothFolds reads the contact through the stored dates and through the history
// at the same instant.
func (e *dedupeEnv) bothFolds(t *testing.T, contactID ids.ContactID, now time.Time) (stored, live RelationshipStrength) {
	t.Helper()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		fromStore, err := contactStrengths(ctx, tx, []ids.ContactID{contactID}, now, nil, true)
		if err != nil {
			return err
		}
		fromHistory, err := contactStrengths(ctx, tx, []ids.ContactID{contactID}, now, nil, false)
		if err != nil {
			return err
		}
		stored, live = fromStore[0].Strength, fromHistory[0].Strength
		return nil
	}); err != nil {
		t.Fatalf("reading both folds: %v", err)
	}
	return stored, live
}

func sameMoment(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

func sameActivity(a, b *ids.ActivityID) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// assertStoredIsLive compares every field the two folds report, and the row's
// own columns with the history's dates.
func (e *dedupeEnv) assertStoredIsLive(t *testing.T, what string, contactID ids.ContactID, now time.Time) RelationshipStrength {
	t.Helper()
	stored, live := e.bothFolds(t, contactID, now)
	row := e.storedDatesOf(t, contactID)
	if !sameMoment(row.interaction, live.LastInteraction) || !sameMoment(row.engaged, live.LastEngaged) ||
		!sameMoment(row.inbound, live.LastInbound) || !sameMoment(row.outbound, live.LastOutbound) {
		t.Errorf("after %s the row stores %v %v %v %v, the history gives %v %v %v %v", what,
			row.interaction, row.engaged, row.inbound, row.outbound,
			live.LastInteraction, live.LastEngaged, live.LastInbound, live.LastOutbound)
	}
	if !sameMoment(stored.LastInteraction, live.LastInteraction) || !sameMoment(stored.LastEngaged, live.LastEngaged) ||
		!sameMoment(stored.LastInbound, live.LastInbound) || !sameMoment(stored.LastOutbound, live.LastOutbound) ||
		!sameActivity(stored.LastInboundActivity, live.LastInboundActivity) {
		t.Errorf("after %s the stored fold reads %+v, the history %+v", what, stored, live)
	}
	stored.LastInteraction, stored.LastEngaged, stored.LastInbound, stored.LastOutbound = nil, nil, nil, nil
	snapshot := live
	live.LastInteraction, live.LastEngaged, live.LastInbound, live.LastOutbound = nil, nil, nil, nil
	stored.LastInboundActivity, live.LastInboundActivity = nil, nil
	if !reflect.DeepEqual(stored, live) {
		t.Errorf("after %s the stored fold scores %+v, the history %+v", what, stored, live)
	}
	return snapshot
}

func TestTheStoredStrengthDatesAreTheLiveFold(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	contact := e.seedVisibleContact(t, "Stored Strength Contact")
	if got := e.storedDatesOf(t, contact); !got.folded || got.interaction != nil {
		t.Fatalf("a new contact stores %+v, want folded with no dates", got)
	}
	for _, ago := range []int{3, 12, 40, 200} {
		e.seedInteraction(t, contact, day(ago), "inbound")
	}
	for _, ago := range []int{1, 15, 400} {
		e.seedInteraction(t, contact, day(ago), "outbound")
	}
	e.seedHeldMeeting(t, contact, day(20))
	e.seedInteraction(t, contact, now.AddDate(0, 0, 2), "inbound")
	e.execTx(e.as(), t, `UPDATE activity SET occurred_precision = 'day',
		occurred_at = (date_trunc('day', occurred_at AT TIME ZONE 'UTC') + interval '12 hours') AT TIME ZONE 'UTC'
		WHERE id IN (SELECT activity_id FROM activity_link WHERE contact_id = $1) AND occurred_at = $2`, contact, day(3))

	live := e.assertStoredIsLive(t, "a history with a day entry and a message still ahead", contact, now)
	if live.InteractionCount90d != 6 || live.Inbound90d != 3 || live.Outbound90d != 2 {
		t.Errorf("the fold counts %d (%d in, %d out), want 6 (3 in, 2 out)", live.InteractionCount90d, live.Inbound90d, live.Outbound90d)
	}
	if got := e.storedDatesOf(t, contact); got.inboundPrecision == nil || *got.inboundPrecision != "day" ||
		got.interaction == nil || got.interaction.After(now) {
		t.Errorf("the contact stores %+v, want a day entry as its last inbound and nothing ahead of now", got)
	}

	e.execTx(e.as(), t, `UPDATE contact SET strength_folded = false WHERE id = $1`, contact)
	unfolded, _ := e.bothFolds(t, contact, now)
	if !sameMoment(unfolded.LastInbound, live.LastInbound) || unfolded.Strength != live.Strength {
		t.Errorf("an unfolded contact reads %+v, want the history's %+v", unfolded, live)
	}
	e.execTx(e.as(), t, `SELECT refresh_contact_strength_dates(ARRAY[$1::uuid])`, contact)
	if got := e.storedDatesOf(t, contact); !got.folded {
		t.Errorf("a refold leaves the contact unfolded: %+v", got)
	}
	e.assertStoredIsLive(t, "a refold", contact, now)
}

// Every writer that can lower a date recomputes it. Every one that adds a date
// merges it. After each the stored dates are the history's.
func TestEveryWriterKeepsTheStoredDatesAtTheFold(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	ann := e.seedVisibleContact(t, "Ann Stored")
	ben := e.seedVisibleContact(t, "Ben Stored")
	newest := e.seedInteractionOfKind(t, ann, "email", day(2), "inbound")
	restricted := e.seedInteractionOfKind(t, ann, "email", day(5), "inbound")
	unsent := e.seedInteractionOfKind(t, ann, "email", day(8), "inbound")
	shared := e.seedInteractionOfKind(t, ann, "email", day(11), "inbound")
	held := e.seedInteractionOfKind(t, ann, "email", day(14), "inbound")
	e.seedInteraction(t, ann, day(30), "inbound")
	e.seedInteraction(t, ben, day(40), "outbound")
	e.execTx(e.as(), t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, shared, ben)
	e.execTx(e.as(), t, `UPDATE activity SET thread_key = 'held-thread' WHERE id = $1`, held)
	e.execTx(e.as(), t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, held, e.rep)
	both := func(what string) {
		t.Helper()
		e.assertStoredIsLive(t, what+" (Ann)", ann, now)
		e.assertStoredIsLive(t, what+" (Ben)", ben, now)
	}
	both("seeding")

	steps := []struct {
		what string
		sql  string
		args []any
	}{
		{"archiving the newest message", `UPDATE activity SET archived_at = now() WHERE id = $1`, []any{newest}},
		{"restricting a message to its participants", `UPDATE activity SET audience = 'participants' WHERE id = $1`, []any{restricted}},
		{"removing a message's sender", `DELETE FROM activity_participant WHERE activity_id = $1`, []any{unsent}},
		{"a seat holding its thread", `INSERT INTO capture_thread_verdict (thread_key, user_id, kind, status) VALUES ('held-thread', $1, 'personal', 'held')`, []any{e.rep}},
		{"erasing a message two contacts share", `DELETE FROM activity WHERE id = $1`, []any{shared}},
		{"re-dating a message into the past", `UPDATE activity SET occurred_at = $2 WHERE id = $1`, []any{unsent, day(60)}},
		{"merging Ben's history into Ann", `UPDATE activity_link SET contact_id = $1 WHERE contact_id = $2`, []any{ann, ben}},
		{"releasing the held thread", `DELETE FROM capture_thread_verdict WHERE thread_key = 'held-thread'`, nil},
	}
	for _, step := range steps {
		e.execTx(e.as(), t, step.sql, step.args...)
		both(step.what)
	}

	e.execTx(e.as(), t, `UPDATE contact SET archived_at = now() WHERE id = $1`, ann)
	if got := e.storedDatesOf(t, ann); got.interaction != nil || got.inbound != nil || got.engaged != nil || got.outbound != nil {
		t.Errorf("an archived contact still stores %+v", got)
	}
	e.execTx(e.as(), t, `UPDATE contact SET archived_at = NULL WHERE id = $1`, ann)
	e.assertStoredIsLive(t, "bringing the contact back", ann, now)
}

// An address another contact comes to hold no longer proves who wrote mail
// that carries no sender row. So the first contact's inbound date comes down.
func TestAnAddressReassignedLowersTheStoredInbound(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	ann := e.seedVisibleContact(t, "Ann Address")
	bea := e.seedVisibleContact(t, "Bea Address")
	e.execTx(e.as(), t, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'ann@example.test', 'manual', 'human:test')`, ann)
	mail := e.seedInteractionOfKind(t, ann, "email", now.AddDate(0, 0, -3), "inbound")
	e.execTx(e.as(), t, `DELETE FROM activity_participant WHERE activity_id = $1`, mail)
	e.execTx(e.as(), t, `UPDATE activity SET counterparty_email = 'ann@example.test' WHERE id = $1`, mail)
	if got := e.storedDatesOf(t, ann); got.inbound == nil {
		t.Fatalf("the address proves Ann wrote the mail, but she stores no last inbound: %+v", got)
	}
	e.execTx(e.as(), t, `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, ann)
	e.execTx(e.as(), t, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, 'ann@example.test', 'manual', 'human:test')`, bea)
	if got := e.storedDatesOf(t, ann); got.inbound != nil {
		t.Errorf("the address moved to Bea, and Ann still stores a last inbound at %v", got.inbound)
	}
	e.assertStoredIsLive(t, "the address moving", ann, now)
}

// A read whose now runs ahead of the database's may already count what the
// stored dates have not, so it folds the history. One behind it reads the row.
func TestAReadAheadOfTheDatabaseFoldsTheHistory(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	contact := e.seedVisibleContact(t, "Clock Contact")
	e.seedInteraction(t, contact, now.AddDate(0, 0, -4), "inbound")
	planted := now.AddDate(0, 0, -50)
	e.execTx(e.as(), t, `UPDATE contact SET strength_last_inbound_at = $2, strength_last_interaction_at = $2,
		strength_last_engaged_at = $2 WHERE id = $1`, contact, planted)
	read := func(at time.Time) RelationshipStrength {
		t.Helper()
		ctx := e.as()
		var got []ContactStrength
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			var err error
			got, err = StrengthForContacts(ctx, tx, []ids.ContactID{contact}, at)
			return err
		}); err != nil {
			t.Fatalf("reading the strength: %v", err)
		}
		return got[0].Strength
	}
	if got := read(now.Add(2 * time.Minute)); !sameMoment(got.LastInbound, ptr(now.AddDate(0, 0, -4))) {
		t.Errorf("a read ahead of the database took the stored inbound %v, want the history's", got.LastInbound)
	}
	if got := read(now.Add(-time.Second)); !sameMoment(got.LastInbound, &planted) {
		t.Errorf("a read behind the database gave %v, want the stored %v: the stored dates were not read", got.LastInbound, planted)
	}
}

func ptr(at time.Time) *time.Time { return &at }

// An import writes a batch of messages in a few statements. Each statement
// moves the contact's stored dates in one UPDATE, whatever the batch size.
func TestAnImportBatchUpdatesTheContactOncePerStatement(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	contact := e.seedVisibleContact(t, "Batch Contact")
	e.seedInteraction(t, contact, now.AddDate(0, 0, -1), "outbound")
	ctx := e.as()
	var updates int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		updated := func() (n int64, err error) {
			err = tx.QueryRow(ctx, `SELECT coalesce(sum(n_tup_upd), 0) FROM pg_stat_xact_user_tables WHERE relname = 'contact'`).Scan(&n)
			return n, err
		}
		before, err := updated()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE batch ON COMMIT DROP AS
			SELECT uuidv7() AS id, $1::timestamptz - make_interval(days => g) AS at FROM generate_series(2, 21) g`, now); err != nil {
			return err
		}
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO activity (id, kind, subject, direction, occurred_at, source_system, source_id, source, captured_by)
			  SELECT id, 'email', 'Batch', 'inbound', at, 'gmail', id::text, 'gmail:seed', 'connector:gmail' FROM batch`, nil},
			{`INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', $1 FROM batch`, []any{contact}},
			{`INSERT INTO activity_participant (activity_id, role, contact_id) SELECT id, 'from', $1 FROM batch`, []any{contact}},
		} {
			if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
				return err
			}
		}
		after, err := updated()
		updates = after - before
		return err
	}); err != nil {
		t.Fatalf("importing a batch: %v", err)
	}
	if updates < 1 || updates > 2 {
		t.Errorf("a batch of 20 messages in three statements updated the contact %d times, want one per statement that moved a date", updates)
	}
	e.assertStoredIsLive(t, "a batch import", contact, now)
}

// One statement can hold threads for several seats at once, with the thread of
// one seat and the seat of another side by side. Each seat's hold must reach
// the mail that seat imported on that thread, and no other pair.
func TestAStatementHoldingSeveralSeatsThreadsLowersEachContact(t *testing.T) {
	e := setupDedupe(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	held := map[ids.ContactID]time.Time{}
	seed := func(name, thread string, seat ids.UUID) ids.ContactID {
		t.Helper()
		contact := e.seedVisibleContact(t, name)
		mail := e.seedInteractionOfKind(t, contact, "email", day(2), "inbound")
		e.seedInteraction(t, contact, day(9), "inbound")
		e.execTx(e.as(), t, `UPDATE activity SET thread_key = $2 WHERE id = $1`, mail, thread)
		e.execTx(e.as(), t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, mail, seat)
		held[contact] = day(2)
		return contact
	}
	ann := seed("Ann Held", "thread-ann", e.rep)
	ben := seed("Ben Held", "thread-ben", e.otherRep)
	e.execTx(e.as(), t, `
		INSERT INTO capture_thread_verdict (thread_key, user_id, kind, status) VALUES
		  ('thread-ann', $1, 'personal', 'pending'), ('thread-ben', $2, 'personal', 'pending'),
		  ('thread-ann', $2, 'personal', 'pending'), ('thread-ben', $1, 'personal', 'pending')`, e.rep, e.otherRep)
	e.execTx(e.as(), t, `
		UPDATE capture_thread_verdict SET status = 'held'
		 WHERE (thread_key, user_id) IN (('thread-ann', $1::uuid), ('thread-ben', $2::uuid))`, e.rep, e.otherRep)
	for _, contact := range []ids.ContactID{ann, ben} {
		got := e.storedDatesOf(t, contact)
		if got.inbound == nil || !got.inbound.Before(held[contact]) {
			t.Errorf("contact %s still stores the held message's date as its last inbound: %v", contact, got.inbound)
		}
		e.assertStoredIsLive(t, "holding two seats' threads in one statement", contact, now)
	}
}
