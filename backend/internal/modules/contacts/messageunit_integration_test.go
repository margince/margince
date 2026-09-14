// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What ONE interaction is, over a real Postgres, when the interaction is a
// channel message.
//
// A chat conversation arrives one row per line, so the §4 fold has to collapse
// them or an afternoon of replies fills a ninety-day quota of contact:
// FreqSaturation is 20, and twenty lines is five minutes of typing. The unit is
// the conversation-day — relstrength.InteractionUnitSQL — and these are the
// cases that separate it from counting rows and from counting a whole
// conversation once however long it runs.
//
// Seeded as rows rather than through capture: the subject here is the READ,
// and the shape a row may take is held by the activity table's own checks
// (a message carries a channel_provider) which these seeds satisfy. Each test
// asserts the raw row count first, so a fold that returned a small number
// because the rows were never written cannot pass as a fold that collapsed
// them.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedMessage writes one channel message linked to a contact, on a named
// conversation, at a chosen instant.
func (e *dedupeEnv) seedMessage(t *testing.T, contactID ids.ContactID, thread string, at time.Time, direction string) {
	t.Helper()
	id := ids.New[ids.ActivityKind]()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, body, direction, occurred_at, thread_key,
			                      channel_provider, source_system, source_id, source, captured_by)
			VALUES ($1, 'message', 'Hallo', $2, $3, $4, 'telegram', 'telegram', $5, 'telegram:seed', 'connector:telegram')`,
			id, direction, at, thread, id.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, contactID)
		return err
	}); err != nil {
		t.Fatalf("seeding a channel message: %v", err)
	}
}

// rawActivityRows is what the fold is collapsing — read separately so a test
// whose seeds never landed fails as a seeding problem rather than passing as a
// collapse.
func (e *dedupeEnv) rawActivityRows(t *testing.T, contactID ids.ContactID) int {
	t.Helper()
	var rows int
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM activity a
			  JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = $1
			 WHERE a.archived_at IS NULL`, contactID).Scan(&rows)
	}); err != nil {
		t.Fatalf("counting the seeded rows: %v", err)
	}
	return rows
}

// An afternoon of chat is one day of talking, and the email beside it still
// counts as itself: the collapse is the message kind's, not every kind's.
func TestAChannelConversationCountsOnceForTheDayItRanOn(t *testing.T) {
	env := setupDedupe(t)
	contact := env.seedVisibleContact(t, "Ines Chatterjee")
	now := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
	day := now.Add(-2 * time.Hour)

	for minute, direction := range []string{"inbound", "outbound", "inbound", "inbound", "outbound"} {
		env.seedMessage(t, contact, "telegram:bot:900", day.Add(time.Duration(minute)*time.Minute), direction)
	}
	env.seedInteraction(t, contact, day.Add(-time.Hour), "outbound")

	if rows := env.rawActivityRows(t, contact); rows != 6 {
		t.Fatalf("seeded %d rows, expected 6 — the case below is about collapsing them, not about their absence", rows)
	}

	got, err := env.store.ContactStrength(env.as(), contact, now)
	if err != nil {
		t.Fatalf("reading the score: %v", err)
	}
	if got.InteractionCount90d != 2 {
		t.Errorf("counted %d interactions over five messages on one conversation plus one email; want 2 — a conversation-day and the email. Counting the rows lets five minutes of typing weigh five times a meeting", got.InteractionCount90d)
	}
	// Both directions ran inside the one conversation-day, so the day is
	// itself in each: a two-way exchange reads as balanced rather than as
	// three-to-two.
	if got.Inbound90d != 1 {
		t.Errorf("counted %d inbound; want 1 — the conversation-day the contact wrote on", got.Inbound90d)
	}
	if got.Outbound90d != 2 {
		t.Errorf("counted %d outbound; want 2 — the conversation-day we wrote on, and the email", got.Outbound90d)
	}
}

// The other side of the same unit: a conversation is not counted once however
// long it runs, or a chat relationship of months would score as a single touch.
// Two days on one conversation are two, and two conversations in one day are
// two as well — the unit is the pair, not either half.
func TestAConversationCountsOncePerDayItRanOn(t *testing.T) {
	env := setupDedupe(t)
	acrossDays := env.seedVisibleContact(t, "Ola Vermeer")
	sameDay := env.seedVisibleContact(t, "Pia Lindqvist")
	now := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

	for _, at := range []time.Time{
		now.AddDate(0, 0, -3),
		now.AddDate(0, 0, -3).Add(time.Minute),
		now.AddDate(0, 0, -1),
		now.AddDate(0, 0, -1).Add(time.Minute),
	} {
		env.seedMessage(t, acrossDays, "telegram:bot:901", at, "inbound")
	}
	for _, thread := range []string{"telegram:bot:902", "telegram:bot:903"} {
		env.seedMessage(t, sameDay, thread, now.AddDate(0, 0, -1), "inbound")
		env.seedMessage(t, sameDay, thread, now.AddDate(0, 0, -1).Add(time.Minute), "inbound")
	}

	if rows := env.rawActivityRows(t, acrossDays); rows != 4 {
		t.Fatalf("seeded %d rows on the two-day conversation, expected 4", rows)
	}
	if rows := env.rawActivityRows(t, sameDay); rows != 4 {
		t.Fatalf("seeded %d rows across the two conversations, expected 4", rows)
	}

	for _, tc := range []struct {
		name    string
		contact ids.ContactID
		want    int
	}{
		{"one conversation over two days", acrossDays, 2},
		{"two conversations on one day", sameDay, 2},
	} {
		got, err := env.store.ContactStrength(env.as(), tc.contact, now)
		if err != nil {
			t.Fatalf("reading the score for %s: %v", tc.name, err)
		}
		if got.InteractionCount90d != tc.want {
			t.Errorf("%s counted %d interactions; want %d", tc.name, got.InteractionCount90d, tc.want)
		}
	}
}
