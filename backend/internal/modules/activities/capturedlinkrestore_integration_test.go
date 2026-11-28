// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A link the calendar lists again comes back as the same file. A subscriber
// counting uploads sees it archived, then restored, and never created twice.
func TestARelistedMeetingDocumentIsAnnouncedAsRestoredNotCreated(t *testing.T) {
	e := setupSend(t)
	meeting := e.seedAnchor(t, "", "")
	ctx := principal.WithCorrelationID(
		principal.WithActor(context.Background(),
			principal.Principal{
				Type: principal.PrincipalSystem, ID: "connector:gcal",
				Permissions: principal.Permissions{
					Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}},
				},
			}), ids.NewV7())
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	from := CapturedFileSource{
		System: "meeting_document", MessageID: meeting.String(), CapturedBy: "connector:gcal", Category: "link",
	}
	const doc = "https://docs.google.com/document/d/notes-1"
	step := func(name string, run func(tx pgx.Tx) (int, error)) {
		t.Helper()
		if err := database.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
			n, err := run(tx)
			if err == nil && n != 1 {
				t.Fatalf("%s changed %d link(s), want 1", name, n)
			}
			return err
		}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	step("record", func(tx pgx.Tx) (int, error) {
		return store.RecordCapturedLinks(ctx, tx, meeting, from, []CapturedLink{{URL: doc, Text: "Notes"}})
	})
	// nil: the calendar now lists no document, as for an untrusted organizer.
	step("retire", func(tx pgx.Tx) (int, error) { return store.RetireCapturedLinks(ctx, tx, meeting, from, nil) })
	step("restore", func(tx pgx.Tx) (int, error) {
		return store.RestoreCapturedLinks(ctx, tx, meeting, from, []string{doc})
	})

	rows, err := e.owner.Query(context.Background(), `
		SELECT o.envelope->>'type' FROM event_outbox o JOIN attachment l ON l.id::text = o.envelope->'entity'->>'id'
		 WHERE l.entity_id = $1 AND o.envelope->>'type' LIKE 'attachment.%' ORDER BY o.seq`, meeting)
	if err != nil {
		t.Fatalf("reading the link's events: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the link's events: %v", err)
	}
	want := []string{"attachment.archived", "attachment.restored"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("the link's events are %v, want %v", got, want)
	}
}
