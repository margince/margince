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
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// A day meeting held on a Sunday in Auckland counts in that Sunday's week. Its
// stored noon is Monday there, which put it in the next week. The evidence row
// keeps the stored date and says it is only a date.
func TestADayMeetingCountsInTheWeekOfItsDate(t *testing.T) {
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
	sunday := occurredday.Anchor(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	subject, held, host := "Trade fair", "held", ids.From[ids.UserKind](e.Rep1)
	if _, _, err := activities.NewStore(InstallationDB(e.Pool)).WithClock(clock).LogActivity(human, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &sunday, OccurredPrecision: activities.PrecisionDay,
		MeetingStatus: &held, HostUserID: &host, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
	}); err != nil {
		t.Fatal(err)
	}

	service := newReportingService(e.Pool, clock)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"sdr_outcomes"}}
	result, err := service.Evaluate(human, selection)
	if err != nil {
		t.Fatal(err)
	}
	weeks := map[string]float64{}
	for _, point := range result.Charts[0].Points {
		if point.Value != nil {
			weeks[point.Key] = *point.Value
		}
	}
	if weeks["2026-10-05"] != 1 || weeks["2026-10-12"] != 0 {
		t.Errorf("weeks %v, want the meeting in the week of 5 October and none in the next", weeks)
	}
	evidence, err := service.Evidence(human, selection, "meetings_held", "interval", "week:2026-10-05", nil, nil, 50,
		reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 {
		t.Fatalf("the week of 5 October holds %d evidence rows, want the day meeting", len(evidence.Rows))
	}
	row := evidence.Rows[0]
	if row.OccurredAt == nil || !row.OccurredAt.Equal(sunday) || row.OccurredPrecision == nil || !occurredday.IsDay(*row.OccurredPrecision) {
		t.Errorf("evidence row time %v precision %v, want the stored date %v as a day", row.OccurredAt, row.OccurredPrecision, sunday)
	}
}
