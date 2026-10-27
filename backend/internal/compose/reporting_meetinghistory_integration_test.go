// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportingMixedMeetingHistoryPreservesRecordedCredit(t *testing.T) {
	e := setupForecast(t)
	human := reportingActor(e)
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return at }
	customer, err := contacts.NewStore(InstallationDB(e.Pool)).WithClock(clock).CreateContact(human, contacts.CreateContactInput{FullName: "Customer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	store := activities.NewStore(InstallationDB(e.Pool)).WithClock(clock)
	for _, tc := range []struct {
		name                              string
		legacy, linked, linkLater, future bool
	}{
		{name: "modern host", linked: true},
		{name: "modern internal gains link", linkLater: true},
		{name: "legacy host", legacy: true, linked: true},
		{name: "legacy internal", legacy: true},
		{name: "legacy gains link", legacy: true, linkLater: true},
		{name: "legacy future", legacy: true, linked: true, future: true},
	} {
		start, held := at.Add(-time.Hour), "held"
		if tc.future {
			start = at.Add(time.Hour)
		}
		input := activities.LogActivityInput{Kind: "meeting", Subject: &tc.name, OccurredAt: &start, MeetingStatus: &held, Source: "manual"}
		if tc.linked {
			input.Links = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}}
		}
		meeting, _, err := store.LogActivity(human, input)
		if err != nil {
			t.Fatal(err)
		}
		if tc.linkLater {
			if _, err := store.RelinkActivity(human, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), activities.RelinkActivityInput{EntityType: "contact", EntityID: ids.UUID(customer.Id)}); err != nil {
				t.Fatal(err)
			}
		}
		// Imported legacy state may differ from a captured transition's host and links.
		var b reportingBindings
		if _, err := e.owner.Exec(human, "UPDATE activity SET host_user_id="+b.add(e.Rep3)+" WHERE id="+b.add(meeting.Id), b.values...); err != nil {
			t.Fatal(err)
		}
		if tc.legacy {
			var history reportingBindings
			if _, err := e.owner.Exec(human, "DELETE FROM activity_meeting_history WHERE activity_id="+history.add(meeting.Id), history.values...); err != nil {
				t.Fatal(err)
			}
		}
	}
	service := newReportingService(e.Pool, clock)
	for _, tc := range []struct {
		owner    ids.UUID
		want     float64
		coverage crmcontracts.ReportingStatus
	}{{e.Rep1, 1, "ok"}, {e.Rep3, 2, "partial"}} {
		selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(tc.owner)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"sdr_outcomes"}}
		result, err := service.Evaluate(human, selection)
		if err != nil {
			t.Fatal(err)
		}
		metric := result.Metrics[0]
		if metric.Value == nil || *metric.Value != tc.want || metric.Coverage.Status != tc.coverage {
			t.Fatalf("owner %s: %+v", tc.owner, metric)
		}
	}
}

// A captured meeting is credited by who took part, not by whose calendar it
// came from. Rep3 imported it and never answered the invitation; Rep1
// organized it and Rep2 accepted. Each of the two counts it once, and their
// team counts it once. An organizer who declined their own meeting is not
// credited with it.
func TestReportingMeetingsHeldCreditsEveryAttendeeAndCountsATeamOnce(t *testing.T) {
	e := setupForecast(t)
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	human := reportingActor(e)
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return at }
	rep2 := ids.NewV7()
	exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep2@forecast.test', 'Rep Two')`, rep2)
	exec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, rep2)
	customer, err := contacts.NewStore(InstallationDB(e.Pool)).WithClock(clock).CreateContact(human, contacts.CreateContactInput{FullName: "Customer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	store := activities.NewStore(InstallationDB(e.Pool)).WithClock(clock)
	subject, start, held := "Rollout review", at.Add(-time.Hour), "held"
	meeting, _, err := store.LogActivity(human, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The calendar connector's shape: the importer is the host column, and the
	// provider's guest list says who organized and who answered.
	exec(t, `UPDATE activity SET captured_by = 'connector:google_calendar', host_user_id = $2 WHERE id = $1`, meeting.Id, e.Rep3)
	exec(t, `UPDATE activity_meeting_history SET host_id_at_change = $2 WHERE activity_id = $1`, meeting.Id, e.Rep3)
	for _, guest := range []struct {
		seat     ids.UUID
		address  string
		role     string
		response string
	}{
		{e.Rep1, "rep1@forecast.test", "organizer", "accepted"},
		{rep2, "rep2@forecast.test", "attendee", "accepted"},
		{e.Rep3, "rep3@forecast.test", "attendee", "needs_action"},
	} {
		exec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response) VALUES ($1, $2, $3, $4, $5)`,
			meeting.Id, guest.role, guest.address, guest.seat, guest.response)
	}
	declined := "Declined by its organizer"
	skipped, _, err := store.LogActivity(human, activities.LogActivityInput{
		Kind: "meeting", Subject: &declined, OccurredAt: &start, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE activity SET captured_by = 'connector:google_calendar', host_user_id = $2 WHERE id = $1`, skipped.Id, e.Rep3)
	exec(t, `UPDATE activity_meeting_history SET host_id_at_change = $2 WHERE activity_id = $1`, skipped.Id, e.Rep3)
	exec(t, `INSERT INTO activity_participant (activity_id, role, address, user_id, response) VALUES ($1, 'organizer', 'rep1@forecast.test', $2, 'declined')`,
		skipped.Id, e.Rep1)
	service := newReportingService(e.Pool, clock)
	for _, tc := range []struct {
		name  string
		scope crmcontracts.ReportingScope
		want  float64
	}{
		{"the organizer", crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, 1},
		{"the guest who accepted", crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(rep2)}, 1},
		{"the importer who never answered", crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}, 0},
		{"the two attendees' team", crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, 1},
		{"the importer's team", crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team2)}, 0},
	} {
		selection := crmcontracts.ReportingSelection{Scope: tc.scope, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"sdr_outcomes"}}
		result, err := service.Evaluate(human, selection)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		metric := result.Metrics[0]
		if metric.Value == nil || *metric.Value != tc.want {
			t.Errorf("%s counts %v meetings held, want %v (%+v)", tc.name, metric.Value, tc.want, metric.Coverage)
		}
	}
}
