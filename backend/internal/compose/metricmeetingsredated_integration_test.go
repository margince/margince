// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Analytics keeps a redated meeting in the week it was held, as Weekly does.
// weekly.TestAMeetingRedatedAfterItWasHeldStaysInItsHeldWeek holds the review's
// side over the same fixture, so a regression in either reader fails.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnalyticsKeepsARedatedMeetingInItsHeldWeek(t *testing.T) {
	e := setupForecast(t)
	ctx := reportingActor(e)
	heldWeek := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	laterWeek := heldWeek.AddDate(0, 0, 7)
	store := activities.NewStore(InstallationDB(e.Pool))
	// Analytics counts customer meetings, so the meeting is filed on a contact.
	customer, err := contacts.NewStore(InstallationDB(e.Pool)).CreateContact(ctx,
		contacts.CreateContactInput{FullName: "Harbour Buyer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}

	at := heldWeek.AddDate(0, 0, 2).Add(10 * time.Hour)
	meeting, _, err := store.LogActivity(ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: new("Scope review"), Source: "manual", MeetingStatus: new("held"),
		OccurredAt: &at, OccurredPrecision: string(activities.PrecisionInstant),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(customer.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Redated through the verb a member uses, which is the defect's own path.
	moved := laterWeek.AddDate(0, 0, 2).Add(10 * time.Hour)
	if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)),
		activities.UpdateActivityInput{OccurredAt: &moved}); err != nil {
		t.Fatalf("redating the meeting: %v", err)
	}

	if got := analyticsMeetingsIn(t, e, heldWeek); got != 1 {
		t.Errorf("Analytics counts %d in the week the meeting was held, want 1", got)
	}
	if got := analyticsMeetingsIn(t, e, laterWeek); got != 0 {
		t.Errorf("Analytics counts %d in the week it was moved into, want 0", got)
	}
}

// analyticsMeetingsIn counts the held meetings Analytics places in one week.
func analyticsMeetingsIn(t *testing.T, e *forecastEnv, start time.Time) int {
	t.Helper()
	ctx := reportingActor(e)
	frame := crmcontracts.ReportingContext{
		EvaluatedAt: time.Now().UTC(), StateAt: time.Now().UTC(), PeriodKind: "week", Timezone: "UTC",
		Interval: crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 7)},
		Scope:    crmcontracts.ReportingScope{Kind: "workspace"},
	}
	counted := 0
	if err := InstallationDB(e.Pool).Tx(ctx, func(tx pgx.Tx) error {
		facts, _, err := readReportingMeetings(ctx, tx, frame, crmcontracts.ReportingFramework{}, metricEvaluator{})
		for _, fact := range facts {
			if fact.ContextID == reportingInterval {
				counted++
			}
		}
		return err
	}); err != nil {
		t.Fatalf("reading Analytics' meetings from %v: %v", start, err)
	}
	return counted
}
