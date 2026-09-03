// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A synced calendar meeting is filed under the people who were in it.
//
// A meeting names no counterparty — attendance is a list — so the tiered gate
// concludes "captured, named nobody", the ensure that files mail never runs, and
// before this the meeting landed with participant rows and NO activity_link. It
// was then unreachable from every surface that finds activity through links:
// the company page's Next meeting, its last-meeting date, the person timeline.
// 517 of 546 meetings in the dev workspace were in that state, and the account
// you were meeting that afternoon showed nothing booked.
//
// This runs the real gcal connector against the stubbed Google, through the real
// Registry and the ONE Sink, because the thing under test is an interaction
// between writes inside the capture transaction: the participant resolution, the
// filing that reads it, and the audience a link-less record is born with.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// personCreator is a principal that may CREATE a contact. The shared capture
// helper grants person:read only — deliberately, because a capture connector
// does not create people through that path — so seeding a contact needs its own.
func personCreator(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs:  []ids.UUID{e.Team1},
		SeatType: principal.SeatFull,
		Scopes:   principal.NewScopeSet(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"person": {Create: true, Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// syncOneGcalMeeting connects the stubbed calendar and runs one sync, answering
// the captured meeting's id. The stub's external meeting has buyer@acme.com on
// it, which is the attendee these tests care about.
func syncOneGcalMeeting(t *testing.T, e *integration.SearchEnv) ids.ActivityID {
	t.Helper()
	const owner = "rep@ws.example"
	stub := gcalStub(t, owner)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)

	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(gcal.New(oauth, api))
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})

	authReq, err := gcal.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gcal/callback")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := gcal.New(oauth, api).Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	connID, err := registry.Connect(grantCtx, "gcal", auth)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := registry.SyncOnce(grantCtx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	var activity ids.ActivityID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_system = 'gcal' AND kind = 'meeting'`).Scan(&activity)
	}); err != nil {
		t.Fatal(err)
	}
	return activity
}

// meetingFiling answers how the captured meeting was filed and what audience it
// was born with — the two halves this change moves together.
func meetingFiling(t *testing.T, e *integration.SearchEnv, activity ids.ActivityID) (people []ids.UUID, orgs int, audience string, reason *string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT person_id FROM activity_link
			  WHERE activity_id = $1 AND entity_type = 'person' ORDER BY person_id`, activity)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			people = append(people, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'organization'`,
			activity).Scan(&orgs); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT audience, audience_reason FROM activity WHERE id = $1`, activity).Scan(&audience, &reason)
	}); err != nil {
		t.Fatal(err)
	}
	return people, orgs, audience, reason
}

// The attendee is already a contact, so the meeting is filed under them and is
// born readable by the workspace — a meeting with a record behind it is not the
// link-less mail the audience limiter exists to hold.
func TestACapturedMeetingIsFiledUnderTheAttendeeWhoIsAContact(t *testing.T) {
	e := integration.SetupSearch(t)

	// Through the real people store, not a hand-built INSERT: the resolution
	// this depends on reads person_email, and a row a test invents is not the
	// row production writes.
	store := people.NewStore(e.DB())
	buyer, err := store.EnsurePersonByEmail(personCreator(e), "Buyer Example", "buyer@acme.com", "manual")
	if err != nil {
		t.Fatalf("seeding the attendee: %v", err)
	}

	activity := syncOneGcalMeeting(t, e)
	linked, orgs, audience, reason := meetingFiling(t, e, activity)

	if len(linked) != 1 || linked[0] != buyer {
		t.Fatalf("meeting filed under %v, want exactly the attendee %s — without a link the meeting reaches no company or person page",
			linked, buyer)
	}
	// A meeting may never link straight to a company: the account is reached
	// through the attendee's employment, and the DB trigger refuses the direct
	// link outright.
	if orgs != 0 {
		t.Errorf("meeting carries %d organization links, want 0 — a meeting is a person's, and the company is reached through their employer", orgs)
	}
	if audience != "workspace" || reason != nil {
		t.Errorf("meeting born audience=%q reason=%v, want workspace/nil — it was filed under a record, so it is not the link-less mail the limiter holds",
			audience, reason)
	}
}

// Nobody has a record for the attendee, so there is nothing to file the meeting
// under, and the limiter's hold still applies. An invite is not correspondence:
// this path must not create a person to have something to link.
func TestACapturedMeetingWithNoKnownAttendeeStaysHeld(t *testing.T) {
	e := integration.SetupSearch(t)

	activity := syncOneGcalMeeting(t, e)
	linked, _, audience, reason := meetingFiling(t, e, activity)

	if len(linked) != 0 {
		t.Fatalf("meeting filed under %v, want nothing — no attendee has a record, and an invite does not create one", linked)
	}
	if audience != "participants" || reason == nil || *reason != "no_record" {
		t.Errorf("meeting born audience=%q reason=%v, want participants/no_record — a meeting filed under nothing is still held to the people on it",
			audience, reason)
	}
	var persons int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM person_email WHERE email = 'buyer@acme.com'`).Scan(&persons)
	}); err != nil {
		t.Fatal(err)
	}
	if persons != 0 {
		t.Errorf("the capture created %d records for an unknown attendee, want 0 — an invitation is not correspondence", persons)
	}
}

// systemRepairCtx is the context a scheduled repair runs under: the workspace,
// a correlation id (storekit refuses to publish without one), and the system
// principal that has no human behind it.
func systemRepairCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:test-repair",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// The other half of the hold: once the meeting IS filed, the reason it was held
// has stopped being true.
//
// This is the ordering Chris's meeting was in — captured while nobody had a
// record for the attendee, filed under them by the cohort repair a day later,
// and held to its participants ever since. The invitation EMAILS beside it on
// the same page were workspace-readable the whole time, so the meeting was the
// one row a colleague could not see.
func TestAMeetingFiledLaterStopsBeingHeld(t *testing.T) {
	e := integration.SetupSearch(t)

	activity := syncOneGcalMeeting(t, e)
	if _, _, audience, reason := meetingFiling(t, e, activity); audience != "participants" ||
		reason == nil || *reason != "no_record" {
		t.Fatalf("the meeting was not born held: audience=%q reason=%v", audience, reason)
	}

	// The contact arrives, and the repair files the meeting under them. Through
	// the real store, so the seam compose wires is the one under test.
	store := people.NewStore(e.DB()).WithAudienceRecompute(activities.RecomputeAudienceTx)
	buyer, err := store.EnsurePersonByEmail(personCreator(e), "Buyer Example", "buyer@acme.com", "manual")
	if err != nil {
		t.Fatalf("seeding the attendee: %v", err)
	}
	if _, err := store.RepairPersonCohort(systemRepairCtx(e), ids.From[ids.PersonKind](buyer)); err != nil {
		t.Fatalf("repairing the cohort: %v", err)
	}

	linked, _, audience, reason := meetingFiling(t, e, activity)
	if len(linked) != 1 || linked[0] != buyer {
		t.Fatalf("the repair filed the meeting under %v, want the attendee %s", linked, buyer)
	}
	if audience != "workspace" || reason != nil {
		t.Errorf("meeting audience=%q reason=%v, want workspace/nil — it is filed under a record now, "+
			"so the hold that said it was filed under none is no longer true", audience, reason)
	}
}
