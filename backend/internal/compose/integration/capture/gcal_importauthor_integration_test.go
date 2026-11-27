// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A colleague's import that names the calendar's owner as the author lands on
// the owner's calendar meeting and brings its contact link. The guest is a
// known contact, so the colleague may read the meeting.
func TestAnImportNamingTheCalendarOwnerLandsOnTheirMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	ensureGuestContact(t, e, guestKnown)
	contact := ensureGuestContact(t, e, guestLater)
	start := captureNow.AddDate(0, 0, 2).Truncate(time.Hour)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, captureOwner, guestEntry(guestKnown, "accepted")))
	connectFeed(t, env, feed)()
	calendarMeeting := onlyCalendarMeeting(t, e)

	landed := importAuthoredMeeting(t, e, contact, e.Rep1, "review@google.com", start)

	if landed != calendarMeeting {
		t.Fatalf("the import filed meeting %v, want the calendar's meeting %v", landed, calendarMeeting)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'meeting'`); got != 1 {
		t.Errorf("%d meetings after the import, want the calendar's one", got)
	}
	if got := contactLinksOn(t, e, calendarMeeting, contact); got != 1 {
		t.Errorf("the import's contact is linked to the calendar meeting %d times, want 1", got)
	}
}

// An import naming any other seat as the author files its own meeting. The
// calendar's meeting is its owner's, and a claim about someone else never joins it.
func TestAnImportNamingAnotherAuthorFilesItsOwnMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	ensureGuestContact(t, e, guestKnown)
	contact := ensureGuestContact(t, e, guestLater)
	start := captureNow.AddDate(0, 0, 2).Truncate(time.Hour)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, captureOwner, guestEntry(guestKnown, "accepted")))
	connectFeed(t, env, feed)()
	calendarMeeting := onlyCalendarMeeting(t, e)

	landed := importAuthoredMeeting(t, e, contact, e.Rep3, "review@google.com", start)

	if landed == calendarMeeting {
		t.Fatal("an import naming another author landed on the calendar owner's meeting")
	}
	if got := contactLinksOn(t, e, calendarMeeting, contact); got != 0 {
		t.Errorf("the import linked its contact to the calendar owner's meeting %d times, want 0", got)
	}
}

// An importer that may read the calendar owner's meeting but not change it
// files its own meeting, because the bind could move the meeting's status.
func TestAnImporterWhoCannotChangeTheMeetingFilesItsOwn(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	ensureGuestContact(t, e, guestKnown)
	own, err := contacts.NewStore(e.DB()).CreateContact(declaredImporterCtx(e, e.Rep3, principal.RowScopeOwn),
		contacts.CreateContactInput{FullName: "Importer's own contact", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(own.Id))
	start := captureNow.AddDate(0, 0, 2).Truncate(time.Hour)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, captureOwner, guestEntry(guestKnown, "accepted")))
	connectFeed(t, env, feed)()
	calendarMeeting := onlyCalendarMeeting(t, e)

	landed := importTitledMeeting(t, e, principal.RowScopeOwn, contact, e.Rep1, "review@google.com", "Kick-off review", start)

	if landed == calendarMeeting {
		t.Fatal("an importer who may not change the meeting landed on it")
	}
}

// A copy stored before imports bound to the author's calendar is folded onto
// the author's calendar meeting at the same instant under the same title. The
// calendar meeting gains the copy's records. Any other import stays as it is.
// So does a copy whose importer may not change the calendar meeting now, since
// the live import would not have bound it either.
func TestTheImportedCopiesTaskFoldsOnlyTheAuthorsCalendarCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subject string
		author  func(e *integration.SearchEnv) ids.UUID
		scope   principal.RowScope
		// changed alters the importer's seat after the import, as $1.
		changed string
		folded  bool
	}{
		{
			name: "the author's calendar holds it", subject: "Kick-off review", scope: principal.RowScopeAll, folded: true,
			author: func(e *integration.SearchEnv) ids.UUID { return e.Rep1 },
		},
		{
			name: "another title", subject: "Another meeting", scope: principal.RowScopeAll,
			author: func(e *integration.SearchEnv) ids.UUID { return e.Rep1 },
		},
		{
			name: "another author", subject: "Kick-off review", scope: principal.RowScopeAll,
			author: func(e *integration.SearchEnv) ids.UUID { return e.Rep3 },
		},
		{
			name: "an importer who may not change the meeting", subject: "Kick-off review", scope: principal.RowScopeOwn,
			author: func(e *integration.SearchEnv) ids.UUID { return e.Rep1 },
		},
		{
			name: "an importer who has left", subject: "Kick-off review", scope: principal.RowScopeAll,
			changed: `UPDATE app_user SET status = 'deactivated' WHERE id = $1`,
			author:  func(e *integration.SearchEnv) ids.UUID { return e.Rep1 },
		},
		{
			name: "an importer moved to a read seat", subject: "Kick-off review", scope: principal.RowScopeAll,
			changed: `UPDATE app_user SET seat_type = 'read' WHERE id = $1`,
			author:  func(e *integration.SearchEnv) ids.UUID { return e.Rep1 },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e := env.e
			ensureGuestContact(t, e, guestKnown)
			grantImporterRole(t, e, e.Rep3, tc.scope)
			own, err := contacts.NewStore(e.DB()).CreateContact(declaredImporterCtx(e, e.Rep3, tc.scope),
				contacts.CreateContactInput{FullName: "Importer's own contact", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			contact := ids.From[ids.ContactKind](ids.UUID(own.Id))
			start := captureNow.AddDate(0, 0, 2).Truncate(time.Hour)
			feed := &calendarFeed{}
			feed.set(lateGuestEvent("review", "confirmed", start, captureOwner, guestEntry(guestKnown, "accepted")))
			connectFeed(t, env, feed)()
			calendarMeeting := onlyCalendarMeeting(t, e)
			imported := importTitledMeeting(t, e, tc.scope, contact, tc.author(e), "", tc.subject, start)
			if tc.changed != "" {
				if _, err := e.Owner.Exec(context.Background(), tc.changed, e.Rep3); err != nil {
					t.Fatal(err)
				}
			}
			task := activities.ImportedMeetingCopiesDataTask(activities.NewStore(e.DB()), identity.MemberPrincipalTx)
			ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:datatask-test")

			for range 2 {
				e.RunDataTask(ctx, t, task)
			}

			live := countRows(t, e, `SELECT count(*) FROM activity WHERE archived_at IS NULL AND id = '`+imported.String()+`'`)
			if tc.folded != (live == 0) {
				t.Fatalf("the import is live=%v after the task, want folded=%v", live == 1, tc.folded)
			}
			want := 0
			if tc.folded {
				want = 1
			}
			if got := contactLinksOn(t, e, calendarMeeting, contact); got != want {
				t.Errorf("the calendar meeting is filed under the import's contact %d times, want %d", got, want)
			}
		})
	}
}

// grantImporterRole gives the importing seat a stored role, because the task
// reads the importer's grants from the database and not from the import's context.
func grantImporterRole(t *testing.T, e *integration.SearchEnv, seat ids.UUID, scope principal.RowScope) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			WITH importer AS (
			  INSERT INTO role (key, name, permissions)
			  VALUES ('importer', 'Importer', jsonb_build_object(
			    'objects', '{"activity":{"create":true,"read":true,"update":true},"contact":{"create":true,"read":true,"update":true}}'::jsonb,
			    'row_scope', $2::text))
			  RETURNING id)
			INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM importer`, seat, string(scope))
		return err
	})
	if err != nil {
		t.Fatalf("assigning the importer's role: %v", err)
	}
}

func contactLinksOn(t *testing.T, e *integration.SearchEnv, meeting ids.UUID, contact ids.ContactID) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, meeting, contact).Scan(&n); err != nil {
		t.Fatalf("counting the meeting's contact links: %v", err)
	}
	return n
}

func onlyCalendarMeeting(t *testing.T, e *integration.SearchEnv) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM activity WHERE source_system = 'gcal' AND kind = 'meeting'`).Scan(&id); err != nil {
		t.Fatalf("reading the calendar's meeting: %v", err)
	}
	return id
}

// importAuthoredMeeting files a meeting through the declared importer's door
// as Rep3, naming author as the seat who wrote it in the source system.
func importAuthoredMeeting(
	t *testing.T, e *integration.SearchEnv, contact ids.ContactID, author ids.UUID, uid string, start time.Time,
) ids.UUID {
	t.Helper()
	return importTitledMeeting(t, e, principal.RowScopeAll, contact, author, uid, "Kick-off review", start)
}

// importTitledMeeting is importAuthoredMeeting under subject. An empty uid
// states no calendar identity.
func importTitledMeeting(
	t *testing.T, e *integration.SearchEnv, scope principal.RowScope,
	contact ids.ContactID, author ids.UUID, uid, subject string, start time.Time,
) ids.UUID {
	t.Helper()
	system, sourceID := "mirror:hubspot", "engagement-1"
	authorID := openapi_types.UUID(author)
	req := crmcontracts.CreateActivityRequest{
		Kind:           crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
		Subject:        &subject,
		OccurredAt:     &start,
		SourceSystem:   &system,
		SourceId:       &sourceID,
		Source:         "import",
		SourceAuthorId: &authorID,
	}
	if uid != "" {
		instance := start.UTC().Format(time.RFC3339)
		req.IcalUid, req.IcalInstance = &uid, &instance
	}
	// The link list is an anonymous contract struct, so it is filled the way a client sends it.
	if err := json.Unmarshal([]byte(`[{"entity_type":"contact","entity_id":"`+contact.String()+`"}]`), &req.Links); err != nil {
		t.Fatal(err)
	}
	in, err := activities.LogActivityInputFromImporter(req)
	if err != nil {
		t.Fatalf("mapping the import: %v", err)
	}
	created, _, err := activities.NewStore(e.DB()).LogActivity(declaredImporterCtx(e, e.Rep3, scope), in)
	if err != nil {
		t.Fatalf("importing the meeting: %v", err)
	}
	return ids.UUID(created.Id)
}

func declaredImporterCtx(e *integration.SearchEnv, seat ids.UUID, scope principal.RowScope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat, OnBehalfOf: seat, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
			},
			RowScope: scope,
		},
	})
}
