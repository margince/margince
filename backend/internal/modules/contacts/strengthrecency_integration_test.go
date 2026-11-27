// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Recency counts from the last time the contact took part, so one send of ours
// they have not answered does not raise the band.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedHeldMeeting files a meeting that took place, with no direction: both
// sides were in it.
func (e *dedupeEnv) seedHeldMeeting(t *testing.T, contactID ids.ContactID, at time.Time) {
	t.Helper()
	id := ids.New[ids.ActivityKind]()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
			VALUES ($1, 'meeting', 'Review', $2, 'held', 'manual', 'human:seed')`, id, at); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, contactID)
		return err
	}); err != nil {
		t.Fatalf("seeding a held meeting: %v", err)
	}
}

func TestAnUnansweredSendLeavesTheBandWhereItWas(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	contact := e.seedVisibleContact(t, "Recency Contact")
	for ago := 10; ago < 14; ago++ {
		e.seedInteraction(t, contact, day(ago), "inbound")
	}
	for ago := 14; ago < 20; ago++ {
		e.seedInteraction(t, contact, day(ago), "outbound")
	}
	for ago := 20; ago < 26; ago++ {
		e.seedHeldMeeting(t, contact, day(ago))
	}
	ctx := e.as()
	before, err := e.store.ContactStrength(ctx, contact, now)
	if err != nil {
		t.Fatalf("reading the strength: %v", err)
	}
	if before.Bucket != "moderate" || before.InteractionCount90d != 16 {
		t.Fatalf("the case starts %s over %d interactions, want moderate over 16", before.Bucket, before.InteractionCount90d)
	}

	e.seedInteraction(t, contact, now, "outbound")
	after, err := e.store.ContactStrength(ctx, contact, now)
	if err != nil {
		t.Fatalf("reading the strength after the send: %v", err)
	}
	if after.Bucket != before.Bucket {
		t.Errorf("an unanswered send moved the band from %s to %s", before.Bucket, after.Bucket)
	}
	if after.LastInteraction == nil || !after.LastInteraction.Equal(now) {
		t.Errorf("last contact = %v, want the send at %v", after.LastInteraction, now)
	}
	if after.LastEngaged == nil || !after.LastEngaged.Equal(day(10)) {
		t.Errorf("last engaged = %v, want their message at %v", after.LastEngaged, day(10))
	}
}

// Nor does it balance the exchange: 10 in and 9 out stay moderate after one
// more send of ours, and their reply then raises the band.
func TestAnUnansweredSendDoesNotBalanceTheExchange(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	contact := e.seedVisibleContact(t, "Balance Contact")
	for ago := 19; ago < 29; ago++ {
		e.seedInteraction(t, contact, day(ago), "inbound")
	}
	for ago := 29; ago < 38; ago++ {
		e.seedInteraction(t, contact, day(ago), "outbound")
	}
	ctx := e.as()
	read := func(step string) RelationshipStrength {
		t.Helper()
		got, err := e.store.ContactStrength(ctx, contact, now)
		if err != nil {
			t.Fatalf("reading the strength %s: %v", step, err)
		}
		return got
	}
	if before := read("before"); before.Bucket != "moderate" {
		t.Fatalf("the case starts %s (%d), want moderate", before.Bucket, before.Strength)
	}
	e.seedInteraction(t, contact, now.Add(-time.Hour), "outbound")
	if after := read("after our send"); after.Bucket != "moderate" {
		t.Errorf("an unanswered send moved the band to %s (%d)", after.Bucket, after.Strength)
	}
	e.seedInteraction(t, contact, now, "inbound")
	if replied := read("after their reply"); replied.Bucket != "strong" {
		t.Errorf("their reply left the band %s (%d), want strong", replied.Bucket, replied.Strength)
	}
}

// A meeting the contact declined is not them taking part.
func TestADeclinedMeetingDoesNotRefreshRecency(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	contact := e.seedVisibleContact(t, "Declining Contact")
	wrote := now.AddDate(0, 0, -40)
	e.seedInteraction(t, contact, wrote, "inbound")
	meeting := ids.New[ids.ActivityKind]()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
			VALUES ($1, 'meeting', 'Review', $2, 'held', 'manual', 'human:seed')`, meeting, now.AddDate(0, 0, -2)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, meeting, contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, contact_id, response)
			VALUES ($1, 'attendee', $2, 'declined')`, meeting, contact)
		return err
	}); err != nil {
		t.Fatalf("seeding the declined meeting: %v", err)
	}
	got, err := e.store.ContactStrength(ctx, contact, now)
	if err != nil {
		t.Fatalf("reading the strength: %v", err)
	}
	if got.LastEngaged == nil || !got.LastEngaged.Equal(wrote) {
		t.Errorf("last engaged = %v, want their message at %v and not the declined meeting", got.LastEngaged, wrote)
	}
}

// A message held to its participants is not visible in the number either. It
// moves no count, no date and no cited message, on the contact page or in a set.
func TestAHeldMessageDoesNotMoveTheStrength(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	day := func(ago int) time.Time { return now.AddDate(0, 0, -ago) }
	contact := e.seedVisibleContact(t, "Held Message Contact")
	for _, ago := range []int{10, 20, 30} {
		e.seedInteraction(t, contact, day(ago), "inbound")
	}
	for _, ago := range []int{15, 25} {
		e.seedInteraction(t, contact, day(ago), "outbound")
	}
	read := func() (RelationshipStrength, RelationshipStrength) {
		t.Helper()
		ctx := e.as()
		page, err := e.store.ContactStrength(ctx, contact, now)
		if err != nil {
			t.Fatalf("reading the contact's strength: %v", err)
		}
		var set []ContactStrength
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			set, err = StrengthForContacts(ctx, tx, []ids.ContactID{contact}, now)
			return err
		}); err != nil || len(set) != 1 {
			t.Fatalf("reading the set's strength: %v (%d rows)", err, len(set))
		}
		return page, set[0].Strength
	}
	pageBefore, setBefore := read()

	held := e.seedInteractionOfKind(t, contact, "email", day(2), "inbound")
	e.hideInteraction(t, held, "audience = 'participants'")
	pageAfter, setAfter := read()

	for _, pair := range []struct {
		where         string
		before, after RelationshipStrength
	}{{"the contact page", pageBefore, pageAfter}, {"a contact set", setBefore, setAfter}} {
		b, a := pair.before, pair.after
		if a.Strength != b.Strength || a.InteractionCount90d != b.InteractionCount90d || a.Inbound90d != b.Inbound90d ||
			!a.LastInteraction.Equal(*b.LastInteraction) || !a.LastInbound.Equal(*b.LastInbound) ||
			a.LastInboundActivity == nil || *a.LastInboundActivity != *b.LastInboundActivity {
			t.Errorf("a held message moved %s's strength from %+v to %+v", pair.where, b, a)
		}
	}
}
