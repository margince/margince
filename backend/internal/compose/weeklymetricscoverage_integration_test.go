// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// coverageReviewAt is a Monday, so the week under review is 21–28 September in
// the fixture's UTC installation.
var (
	coverageReviewAt  = time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	coverageWeekStart = time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	coverageWeekEnd   = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
)

func TestAWeekBeforeTheRepsFirstRecordIsNotRecordedRatherThanZero(t *testing.T) {
	e := setupForecast(t)
	writer := reportingActor(e)
	// Both written now, which is after the reviewed week closed.
	coverageDeal(writer, t, e, nil, nil)
	subject := "Send the revised offer"
	due := time.Now().Add(24 * time.Hour)
	assignee := ids.From[ids.UserKind](e.Rep1)
	if _, _, err := activities.NewStore(InstallationDB(e.Pool)).LogActivity(writer, activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &assignee, Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}

	review := assembleCoverageWeek(t, e)
	coverage := review.NumericSummary.FigureCoverage
	for family, figure := range map[string]*crmcontracts.WeeklyFigureCoverage{"deals": coverage.Deals, "tasks": coverage.Tasks} {
		if figure == nil || figure.Status != crmcontracts.WeeklyFigureCoverageStatusNotRecorded {
			t.Fatalf("%s for a week before the rep's first record = %+v, want not_recorded", family, figure)
		}
		if figure.RecordedSince == nil || figure.RecordedSince.Before(coverageWeekEnd) || figure.Reason == nil {
			t.Errorf("%s names no later first record or no reason: %+v", family, figure)
		}
	}
	if coverage.Leads != nil {
		t.Errorf("a seat without lead read was told about leads: %+v, want no statement", coverage.Leads)
	}
}

func TestASeatThatReadsLeadsIsToldItsLeadsWereNotRecorded(t *testing.T) {
	e := setupForecast(t)

	review := assembleCoverageWeekAs(leadReadingActor(t, e), t, e)

	leads := review.NumericSummary.FigureCoverage.Leads
	if leads == nil || leads.Status != crmcontracts.WeeklyFigureCoverageStatusNotRecorded || leads.RecordedSince != nil {
		t.Errorf("leads for a rep who holds none = %+v, want not_recorded with no first record", leads)
	}
}

// leadReadingActor is reportingActor with lead read added.
func leadReadingActor(t *testing.T, e *forecastEnv) context.Context {
	t.Helper()
	ctx := reportingActor(e)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("reportingActor bound no principal")
	}
	grants := maps.Clone(actor.Permissions.Objects)
	grants["lead"] = principal.ObjectGrant{Read: true}
	actor.Permissions.Objects = grants
	return principal.WithActor(ctx, actor)
}

func TestAFigureCountsFromItsFirstRecordAndAnImportCanMeasureAWholeWeek(t *testing.T) {
	e := setupForecast(t)
	writer := reportingActor(e)
	// An imported deal, closed in the source system before the reviewed week.
	imported := "hubspot"
	coverageDeal(writer, t, e, &imported, func() time.Time { return coverageWeekStart.AddDate(0, 0, -11) })

	contact := e.seedID(t, `INSERT INTO contact (id, first_name, last_name, full_name, source, captured_by)
		VALUES ($1, 'Jamie', 'Buyer', 'Jamie Buyer', 'manual', 'test')`)
	held, subject := "held", "First customer review"
	occurred := coverageWeekStart.Add(2*24*time.Hour + 15*time.Hour)
	if _, _, err := activities.NewStore(InstallationDB(e.Pool)).LogActivity(writer, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &occurred, MeetingStatus: &held, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatal(err)
	}

	review := assembleCoverageWeek(t, e)
	coverage := review.NumericSummary.FigureCoverage
	if coverage.Deals == nil || coverage.Deals.Status != crmcontracts.WeeklyFigureCoverageStatusRecorded {
		t.Fatalf("a deal imported as closed before the week left deals at %+v, want recorded", coverage.Deals)
	}
	meetings := coverage.Meetings
	if meetings == nil || meetings.Status != crmcontracts.WeeklyFigureCoverageStatusPartial ||
		meetings.RecordedSince == nil || !meetings.RecordedSince.Equal(occurred) {
		t.Fatalf("meetings whose first record falls mid-week = %+v, want partial from %s", meetings, occurred)
	}

	reread, err := coverageEngine(e).LatestReview(reportingActor(e), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reread.NumericSummary == nil || reread.NumericSummary.FigureCoverage == nil {
		t.Fatal("the frozen review lost its figure coverage on re-read")
	}
	again := reread.NumericSummary.FigureCoverage
	if again.Deals.Status != coverage.Deals.Status || again.Meetings.Status != meetings.Status ||
		again.Meetings.RecordedSince == nil || !again.Meetings.RecordedSince.Equal(*meetings.RecordedSince) {
		t.Errorf("the frozen coverage changed on re-read: %+v, assembled %+v", again, coverage)
	}
}

func coverageEngine(e *forecastEnv) *weekly.Engine {
	return weekly.NewEngine(e.Pool, newTeammatesSeam(e.Pool)).WithNumeric(weeklyNumericEvaluator{})
}

func assembleCoverageWeek(t *testing.T, e *forecastEnv) weekly.Review {
	t.Helper()
	return assembleCoverageWeekAs(reportingActor(e), t, e)
}

func assembleCoverageWeekAs(reader context.Context, t *testing.T, e *forecastEnv) weekly.Review {
	t.Helper()
	review, created, err := coverageEngine(e).AssembleFor(reader, coverageReviewAt)
	if err != nil {
		t.Fatal(err)
	}
	if !created || review.NumericSummary == nil || review.NumericSummary.FigureCoverage == nil {
		t.Fatalf("the assembled week carries no figure coverage: %+v", review)
	}
	return review
}

// coverageDeal creates one of Rep1's deals through the real writer and, when a
// close clock is given, wins it at that instant.
func coverageDeal(writer context.Context, t *testing.T, e *forecastEnv, source *string, closedAt func() time.Time) {
	t.Helper()
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := store.CreateDeal(writer, deals.CreateDealInput{
		Name: "Weber Rahmenvertrag", PipelineID: ids.From[ids.PipelineKind](e.pipeline),
		StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &owner, Source: "manual", SourceSystem: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if closedAt == nil {
		return
	}
	won := e.seedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Won', 10, 'won', 100)`, e.pipeline)
	reason := "imported"
	if _, err := store.WithClock(closedAt).AdvanceDeal(writer, ids.From[ids.DealKind](ids.UUID(created.Id)), deals.AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](won), WonWithoutContractReason: &reason,
	}); err != nil {
		t.Fatal(err)
	}
}
