// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
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
