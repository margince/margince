// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
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
	instance := start.UTC().Format(time.RFC3339)
	system, sourceID, subject := "mirror:hubspot", "engagement-1", "Kick-off review"
	authorID := openapi_types.UUID(author)
	req := crmcontracts.CreateActivityRequest{
		Kind:           crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
		Subject:        &subject,
		OccurredAt:     &start,
		SourceSystem:   &system,
		SourceId:       &sourceID,
		Source:         "hubspot_import:1",
		SourceAuthorId: &authorID,
		IcalUid:        &uid,
		IcalInstance:   &instance,
	}
	// The link list is an anonymous contract struct, so it is filled the way a client sends it.
	if err := json.Unmarshal([]byte(`[{"entity_type":"contact","entity_id":"`+contact.String()+`"}]`), &req.Links); err != nil {
		t.Fatal(err)
	}
	in, err := activities.LogActivityInputFromImporter(req)
	if err != nil {
		t.Fatalf("mapping the import: %v", err)
	}
	created, _, err := activities.NewStore(e.DB()).LogActivity(declaredImporterCtx(e, e.Rep3), in)
	if err != nil {
		t.Fatalf("importing the meeting: %v", err)
	}
	return ids.UUID(created.Id)
}

func declaredImporterCtx(e *integration.SearchEnv, seat ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat, OnBehalfOf: seat,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}
