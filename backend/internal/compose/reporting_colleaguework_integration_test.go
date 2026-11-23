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

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
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
	introduced := func(introducer ids.UUID) ids.UUID {
		ask, err := intros.Create(as(teammate), introductions.NewRequest{ContactID: m.customer, IntroducerUser: introducer, RouteType: "direct", InternalReason: "they run procurement", DueAt: m.clock().AddDate(0, 0, 7)})
		if err != nil {
			t.Fatalf("asking %s for the introduction: %v", introducer, err)
		}
		if err := intros.Decide(as(introducer), ask, introductions.StatusAccepted, "", nil, 1); err != nil {
			t.Fatalf("%s accepting the ask: %v", introducer, err)
		}
		if err := intros.Complete(as(introducer), ask, nil, 2); err != nil {
			t.Fatalf("%s completing the introduction: %v", introducer, err)
		}
		return ask
	}
	// Rep3's introduction is the two colleagues' alone: Rep1 never counts or sees it.
	mine, theirs := introduced(m.e.Rep1), introduced(m.e.Rep3)
	if err := InstallationDB(m.e.Pool).Tx(rep, func(tx pgx.Tx) error {
		seen, err := reportingVisibleSources(rep, tx, reportingVisibilityKey{table: introductions.Table, metric: reportingIntroductionsMade}, []ids.UUID{mine, theirs})
		if err == nil && (!seen[mine] || seen[theirs]) {
			t.Errorf("a saved edition shows Rep1 introductions %v, want Rep1's own alone", seen)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m.bookFor(rep, t, teammate, time.Date(2026, 9, 30, 10, 59, 0, 0, time.UTC))
	m.bookFor(rep, t, teammate, time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC))
	m.bookFor(rep, t, m.e.Rep1, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	m.bookFor(as(m.e.Rep3), t, m.e.Rep1, time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC))
	service := newReportingService(m.e.Pool, m.clock)
	metrics := []crmcontracts.ReportingMetricID{reportingIntroductionsMade, reportingMeetingsBookedForColleagues}
	for _, metric := range metrics {
		october := openapi_types.Date{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
		target := crmcontracts.ReportingTargetInput{Metric: metric, Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(m.e.Rep1)}, PeriodKind: "month", PeriodStart: october, Value: 4, Reason: "Partner plan"}
		if _, err := service.CreateTarget(rep, target); err != nil {
			t.Fatalf("setting the %s target: %v", metric, err)
		}
	}
	for _, scope := range []crmcontracts.ReportingScope{{Kind: "owner", Id: ptrUUID(m.e.Rep1)}, {Kind: "team", Id: ptrUUID(m.e.Team1)}} {
		result, err := service.Evaluate(rep, crmcontracts.ReportingSelection{Scope: scope, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: metrics, Blocks: []crmcontracts.ReportingBlockKind{}})
		if err != nil || len(result.Metrics) != 2 {
			t.Fatalf("%s evaluation: %v, %d metrics", scope.Kind, err, len(result.Metrics))
		}
		for _, metric := range result.Metrics {
			if metric.Value == nil || *metric.Value != 1 || metric.TargetActual == nil || *metric.TargetActual != 1 {
				t.Errorf("%s reads %s as %q, %q toward the target, want Rep1's one October introduction or booking", scope.Kind, metric.Id, csvCellPtr(metric.Value), csvCellPtr(metric.TargetActual))
			}
			if wantTarget := scope.Kind == "owner"; (metric.Target != nil && *metric.Target == 4) != wantTarget {
				t.Errorf("%s reads %s against target %q, want 4 only for the owner", scope.Kind, metric.Id, csvCellPtr(metric.Target))
			}
		}
	}
}
