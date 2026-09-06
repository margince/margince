// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// WHO MAY DISCOVER A CAPTURED MEETING.
//
// A meeting is not a workspace-shared note, and the difference is the whole
// point of this file. An unlinked NOTE is somebody writing something down for
// the workspace, so ActivityDiscoverClause's empty-link rule — coalesce to
// true, every seat may read it — is right for it.
//
// A calendar event is the opposite. It arrives from one person's mailbox,
// carries their private appointments, and links nothing at all until somebody
// files it against a record. Under the note rule every seat in the
// installation could discover every colleague's calendar: on the staging
// installation this was 465 of 468 meetings readable by the single most
// restricted account on it, "Drinks at Xu" and a partner negotiation included.
//
// So a captured meeting is born held to its participants, exactly as a
// captured message is, and these tests drive the capture path rather than
// asserting over a hand-written row: what is at stake is what the WRITER
// stamps, and a fixture that set the audience itself would pass while the
// writer stamped anything at all.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarSinkContext binds the per-seat calendar connector principal, the way
// the sync loop mints one: it carries the SEAT's user id, which is what makes
// "whose calendar is this" answerable at the write.
func calendarSinkContext(ctx context.Context, ws, seat ids.UUID) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type:   principal.PrincipalConnector,
		ID:     "connector:gcal:" + seat.String(),
		UserID: seat,
		Permissions: principal.Permissions{
			RoleKeys: []string{"capture"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true},
				"person":   {Create: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// captureAMeeting drives the real capture path for one calendar event and
// answers the activity id it wrote.
//
// It goes through Sink.Upsert rather than an INSERT of its own because what is
// under test is what the WRITER stamps. A fixture that set the audience itself
// would pass whatever the writer did.
func captureAMeeting(
	ctx context.Context, t *testing.T, db *database.DB, seat ids.UUID, sourceID string, attendee *string,
) ids.UUID {
	t.Helper()
	ws, _ := principal.WorkspaceID(ctx)
	seatCtx := calendarSinkContext(ctx, ids.UUID(ws), seat)
	if err := seedCapturingSeat(seatCtx, t, db, seat); err != nil {
		t.Fatalf("seeding the capturing seat: %v", err)
	}
	rec := connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: "gcal", SourceID: sourceID},
		Fields: capture.ActivityFields{
			Kind:       "meeting",
			Subject:    "Drinks at Xu",
			OccurredAt: time.Now().Add(2 * time.Hour),
		},
		Source:     "gcal:" + sourceID,
		CapturedBy: "connector:gcal:" + seat.String(),
	}
	if attendee != nil {
		rec.Counterparty = connector.Counterparty{Email: *attendee}
		rec.Addresses = []string{*attendee}
	}
	ref, err := capture.NewSink(db).Upsert(seatCtx, rec)
	if err != nil {
		t.Fatalf("capturing a meeting: %v", err)
	}
	return ids.UUID(ref.ID)
}

// seedCapturingSeat gives the connector's seat a real app_user row, because
// host_user_id references one.
func seedCapturingSeat(ctx context.Context, t *testing.T, db *database.DB, seat ids.UUID) error {
	t.Helper()
	return db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name, status)
			VALUES ($1, $2, 'Calendar Seat', 'active')
			ON CONFLICT (id) DO NOTHING`, seat, "seat-"+seat.String()+"@example.test")
		return err
	})
}

// meetingAudienceOf answers the audience and reason the capture path stamped on
// one activity, read as the owner so the assertion is about what was WRITTEN
// rather than about what some reader may see.
func meetingAudienceOf(ctx context.Context, t *testing.T, owner *pgx.Conn, id ids.UUID) (string, string) {
	t.Helper()
	var audience, reason string
	if err := owner.QueryRow(ctx, `
		SELECT audience, coalesce(audience_reason, '')
		FROM activity WHERE id = $1`, id).Scan(&audience, &reason); err != nil {
		t.Fatalf("reading the audience of %s: %v", id, err)
	}
	return audience, reason
}

// discoverableBy answers whether ActivityDiscoverClause lets this reader learn
// the activity exists, by running the clause the readers actually compose
// rather than a paraphrase of it.
func discoverableBy(ctx context.Context, t *testing.T, db *database.DB, reader, id ids.UUID) bool {
	t.Helper()
	var found bool
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM activity a
			  WHERE a.id = $1
			    AND a.restricted_at IS NULL
			    AND coalesce((SELECT bool_or(
			          l.person_id IS NOT NULL AND EXISTS (
			            SELECT 1 FROM person sp
			            WHERE sp.id = l.person_id
			              AND (sp.visibility <> 'owner' OR sp.owner_id = $2)))
			        FROM activity_link l WHERE l.activity_id = a.id),
			        a.kind <> 'meeting' OR a.host_user_id IS NULL OR a.host_user_id = $2))`,
			id, reader).Scan(&found)
	}); err != nil {
		t.Fatalf("probing discoverability: %v", err)
	}
	return found
}

// TestACapturedMeetingIsNotBornWorkspaceReadable is the leak, stated as the
// rule it broke.
//
// A meeting captured from one seat's calendar, linking nothing, must not be
// born readable by the whole installation. It is the unlinked case precisely
// because that is the common one: 450 of the 468 meetings on the staging
// installation linked no record at all.
func TestACapturedMeetingIsNotBornWorkspaceReadable(t *testing.T) {
	ctx, db, owner := meetingWorkspaceWithOwner(t)
	host := ids.NewV7()
	meeting := captureAMeeting(ctx, t, db, host, "cal-unlinked-1", nil)

	audience, reason := meetingAudienceOf(ctx, t, owner, meeting)
	if audience == "workspace" {
		t.Errorf("a captured meeting was born audience=workspace (reason %q) — "+
			"every seat in the installation may read this person's calendar", reason)
	}
	if audience != "participants" {
		t.Errorf("audience = %q, want participants — a calendar event is held to "+
			"the people on it until something files it", audience)
	}
}

// TestACapturedMeetingNamesItsHost pins the other half: the row records WHOSE
// calendar it came from.
//
// Without it the brief cannot attribute the meeting, so a lane that lists
// meetings under no owner puts every colleague's appointment on every reader's
// morning — which is how the leak reached a human, as "a meeting with Lucy" on
// an account that had never met her.
func TestACapturedMeetingNamesItsHost(t *testing.T) {
	ctx, db, owner := meetingWorkspaceWithOwner(t)
	host := ids.NewV7()
	meeting := captureAMeeting(ctx, t, db, host, "cal-host-1", nil)

	var got *ids.UUID
	if err := owner.QueryRow(ctx,
		`SELECT host_user_id FROM activity WHERE id = $1`, meeting).Scan(&got); err != nil {
		t.Fatalf("reading host_user_id: %v", err)
	}
	if got == nil {
		t.Fatal("host_user_id is NULL on a captured meeting — the brief has no owner to attribute it to")
	}
	if *got != host {
		t.Errorf("host_user_id = %s, want the capturing seat %s", *got, host)
	}
}

// TestAColleaguesLinklessMeetingIsNotDiscoverable is the leak as the reader
// meets it, through the clause every activity reader composes.
func TestAColleaguesLinklessMeetingIsNotDiscoverable(t *testing.T) {
	ctx, db, _ := meetingWorkspaceWithOwner(t)
	host, colleague := ids.NewV7(), ids.NewV7()
	meeting := captureAMeeting(ctx, t, db, host, "cal-unlinked-2", nil)

	if discoverableBy(ctx, t, db, colleague, meeting) {
		t.Error("a colleague can discover a meeting captured from somebody else's " +
			"calendar that links no record — this is the staging leak")
	}
	if !discoverableBy(ctx, t, db, host, meeting) {
		t.Error("the capturing seat cannot discover its own meeting — the hold is too wide")
	}
}
