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
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// meetingWeeks is a workspace in Auckland whose reports read at a fixed clock,
// with one customer a meeting can be held with.
type meetingWeeks struct {
	e        *forecastEnv
	human    context.Context
	clock    func() time.Time
	customer ids.UUID
}

func setupMeetingWeeks(t *testing.T) meetingWeeks {
	t.Helper()
	e := setupForecast(t)
	human := reportingActor(e)
	zone := "Pacific/Auckland"
	settings := identity.NewInstallationSettings(InstallationDB(e.Pool), NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	if _, err := settings.UpdateInstallation(human, identity.InstallationPatch{Timezone: &zone}); err != nil {
		t.Fatalf("saving the installation zone: %v", err)
	}
	at := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return at }
	customer, err := contacts.NewStore(InstallationDB(e.Pool)).WithClock(clock).CreateContact(human, contacts.CreateContactInput{FullName: "Harbour Buyer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return meetingWeeks{e: e, human: human, clock: clock, customer: ids.UUID(customer.Id)}
}

// held logs a meeting held with the customer at the given time and precision.
func (m meetingWeeks) held(t *testing.T, at time.Time, precision string) ids.ActivityID {
	t.Helper()
	subject, held, host := "Trade fair", "held", ids.From[ids.UserKind](m.e.Rep1)
	logged, _, err := activities.NewStore(InstallationDB(m.e.Pool)).WithClock(m.clock).LogActivity(m.human, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, OccurredPrecision: precision,
		MeetingStatus: &held, HostUserID: &host, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: m.customer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

// weeks evaluates meetings held this month, by week, and answers the rows of
// the week starting on week.
func (m meetingWeeks) weeks(t *testing.T, week string) (map[string]float64, []crmcontracts.ReportingEvidenceRow) {
	t.Helper()
	service := newReportingService(m.e.Pool, m.clock)
	selection := crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"sdr_outcomes"},
	}
	result, err := service.Evaluate(m.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	weeks := map[string]float64{}
	for _, point := range result.Charts[0].Points {
		if point.Value != nil {
			weeks[point.Key] = *point.Value
		}
	}
	evidence, err := service.Evidence(m.human, selection, "meetings_held", "interval", "week:"+week, nil, nil, 50,
		reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	return weeks, evidence.Rows
}

// A day meeting held on a Sunday in Auckland counts in that Sunday's week. Its
// stored noon is Monday there, which put it in the next week. The evidence row
// keeps the stored date and says it is only a date.
func TestADayMeetingCountsInTheWeekOfItsDate(t *testing.T) {
	m := setupMeetingWeeks(t)
	sunday := occurredday.Anchor(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	m.held(t, sunday, activities.PrecisionDay)

	weeks, rows := m.weeks(t, "2026-10-05")
	if weeks["2026-10-05"] != 1 || weeks["2026-10-12"] != 0 {
		t.Errorf("weeks %v, want the meeting in the week of 5 October and none in the next", weeks)
	}
	if len(rows) != 1 {
		t.Fatalf("the week of 5 October holds %d evidence rows, want the day meeting", len(rows))
	}
	row := rows[0]
	if row.OccurredAt == nil || !row.OccurredAt.Equal(sunday) || row.OccurredPrecision == nil || !occurredday.IsDay(*row.OccurredPrecision) {
		t.Errorf("evidence row time %v precision %v, want the stored date %v as a day", row.OccurredAt, row.OccurredPrecision, sunday)
	}
}

// A meeting held at an instant keeps that instant's week once it is later
// made a day entry. The history keeps the instant, which is Monday in
// Auckland and Sunday by its UTC date.
func TestAHeldInstantKeepsItsWeekWhenTheMeetingBecomesADay(t *testing.T) {
	m := setupMeetingWeeks(t)
	mondayInAuckland := time.Date(2026, 10, 11, 11, 30, 0, 0, time.UTC)
	meeting := m.held(t, mondayInAuckland, activities.PrecisionInstant)
	day, date := activities.PrecisionDay, occurredday.Anchor(time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC))
	if _, err := activities.NewStore(InstallationDB(m.e.Pool)).WithClock(m.clock).UpdateActivity(m.human, meeting,
		activities.UpdateActivityInput{OccurredAt: &date, OccurredPrecision: &day}); err != nil {
		t.Fatalf("making the meeting a day entry: %v", err)
	}

	weeks, rows := m.weeks(t, "2026-10-12")
	if weeks["2026-10-12"] != 1 || weeks["2026-10-05"] != 0 {
		t.Errorf("weeks %v, want the held instant in the week of 12 October and none in the one before", weeks)
	}
	if len(rows) != 1 || rows[0].OccurredPrecision == nil || occurredday.IsDay(*rows[0].OccurredPrecision) {
		t.Fatalf("evidence rows %+v, want the held instant read as an instant", rows)
	}
}
