// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Introductions made and meetings booked for colleagues read through the
// evaluation My outcomes asks: the reader's own month, held against the target
// set for it. The installation is in Auckland, where October begins at 11:00
// UTC on 30 September.

import (
	"context"
	"maps"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bookFor logs a meeting rep books for host and dates its booking at.
func (m meetingWeeks) bookFor(rep context.Context, t *testing.T, host ids.UUID, at time.Time) {
	t.Helper()
	subject, booked, hostID, start := "Demo", "booked", ids.From[ids.UserKind](host), at.Add(14*24*time.Hour)
	logged, _, err := activities.NewStore(InstallationDB(m.e.Pool)).WithClock(m.clock).LogActivity(rep, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &start, MeetingStatus: &booked, HostUserID: &hostID, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: m.customer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.e.owner.Exec(context.Background(), `UPDATE activity SET created_at = $2 WHERE id = $1`, ids.UUID(logged.Id), at); err != nil {
		t.Fatal(err)
	}
}

func TestColleagueWorkReadsAgainstItsTargetInMyOutcomes(t *testing.T) {
	m := setupMeetingWeeks(t)
	actor, _ := principal.Actor(m.human)
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects[introductions.Object] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	as := func(user ids.UUID) context.Context {
		seat := actor
		seat.ID, seat.UserID = "human:"+user.String(), user
		return principal.WithActor(m.human, seat)
	}
	rep := as(m.e.Rep1)
	teammate := reportingTeammate(t, m.e)
	intros := introductions.NewStore(InstallationDB(m.e.Pool), func() time.Time { return m.clock().Add(-time.Hour) })
	// An ask between two others is theirs alone: it counts for neither scope
	// of Rep1's, and the company figure says it left something out.
	ask, err := intros.Create(as(teammate), introductions.NewRequest{ContactID: m.customer, IntroducerUser: m.e.Rep3, RouteType: "direct", InternalReason: "they run procurement", DueAt: m.clock().AddDate(0, 0, 7)})
	if err != nil || intros.Decide(as(m.e.Rep3), ask, introductions.StatusAccepted, "", nil, 1) != nil || intros.Complete(as(m.e.Rep3), ask, nil, 2) != nil {
		t.Fatalf("the ask between two colleagues: %v", err)
	}
	for _, on := range []time.Time{time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)} {
		contact, err := contacts.NewStore(InstallationDB(m.e.Pool)).SetClock(m.clock).CreateContact(rep, contacts.CreateContactInput{FullName: "Buyer " + on.Format(time.DateOnly), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := intros.Record(rep, introductions.NewRecorded{ContactID: ids.UUID(contact.Id), IntroducerUser: m.e.Rep1, IntroducedUser: teammate, On: on, Channel: "email"}); err != nil {
			t.Fatal(err)
		}
	}
	m.bookFor(rep, t, teammate, time.Date(2026, 9, 30, 10, 59, 0, 0, time.UTC))
	m.bookFor(rep, t, teammate, time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC))
	m.bookFor(rep, t, m.e.Rep1, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))

	service := newReportingService(m.e.Pool, m.clock)
	metrics := []crmcontracts.ReportingMetricID{reportingIntroductionsMade, reportingMeetingsBookedForColleagues}
	for _, metric := range metrics {
		october := openapi_types.Date{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
		target := crmcontracts.ReportingTargetInput{Metric: metric, Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(m.e.Rep1)}, PeriodKind: "month", PeriodStart: october, Value: 4, Reason: "Partner plan"}
		if _, err := service.CreateTarget(rep, target); err != nil {
			t.Fatalf("setting the %s target: %v", metric, err)
		}
	}
	for _, scope := range []crmcontracts.ReportingScope{{Kind: "owner", Id: ptrUUID(m.e.Rep1)}, {Kind: "team", Id: ptrUUID(m.e.Team1)}, {Kind: "workspace"}} {
		result, err := service.Evaluate(rep, crmcontracts.ReportingSelection{Scope: scope, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: metrics, Blocks: []crmcontracts.ReportingBlockKind{}})
		if err != nil || len(result.Metrics) != 2 {
			t.Fatalf("%s evaluation: %v, %d metrics", scope.Kind, err, len(result.Metrics))
		}
		for _, metric := range result.Metrics {
			if metric.Value == nil || *metric.Value != 1 || metric.TargetActual == nil || *metric.TargetActual != 1 {
				t.Errorf("%s reads %s as %q, %q toward the target, want the one October introduction or booking", scope.Kind, metric.Id, csvCellPtr(metric.Value), csvCellPtr(metric.TargetActual))
			}
			if withheld := metric.Id == reportingIntroductionsMade && scope.Kind == "workspace"; metric.Coverage.Withheld != withheld {
				t.Errorf("%s reads %s as withheld %t, want %t", scope.Kind, metric.Id, metric.Coverage.Withheld, withheld)
			}
			if wantTarget := scope.Kind == "owner"; (metric.Target != nil && *metric.Target == 4) != wantTarget {
				t.Errorf("%s reads %s against target %q, want 4 only for the owner", scope.Kind, metric.Id, csvCellPtr(metric.Target))
			}
		}
	}
}
