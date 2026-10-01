// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
)

func TestWeeklyDoesNotCompareDifferentAnalyticsDefinitions(t *testing.T) {
	e := setupForecast(t)
	human := reportingActor(e)
	engine := weekly.NewEngine(e.Pool, newTeammatesSeam(e.Pool)).WithNumeric(weeklyNumericEvaluator{})
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	prior, _, err := engine.AssembleFor(human, at)
	if err != nil {
		t.Fatal(err)
	}
	if prior.NumericSummary == nil || prior.NumericSummary.Version != "analytics:bookings_won@2:meetings_held@2" {
		t.Fatalf("metric versions missing: %+v", prior.NumericSummary)
	}
	current, _, err := engine.AssembleFor(human, at.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	same, err := engine.LatestReview(human, &current.LocalWeekStart)
	if err != nil || same.Prior == nil {
		t.Fatalf("same definitions must compare: %+v %v", same.Prior, err)
	}
	var b reportingBindings
	if _, err := e.owner.Exec(human, "UPDATE weekly_review SET numeric_summary=jsonb_set(numeric_summary,'{version}','\"analytics-1\"') WHERE id="+b.add(prior.ID), b.values...); err != nil {
		t.Fatal(err)
	}
	changed, err := engine.LatestReview(human, &current.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Prior != nil {
		t.Fatal("different attribution definitions produced a weekly comparison")
	}
}
