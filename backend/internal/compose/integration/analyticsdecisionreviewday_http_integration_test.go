// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A review day before today would make the decision's task overdue at once,
// so the route refuses it and records nothing.
func TestADecisionWithAPastReviewDayIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Review day", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
		"amount_minor": 70_000, "currency": "EUR",
	})
	today := clocktest.Now(t).UTC()
	decide := func(day time.Time) int {
		return e.Call(t, "POST", "/v1/analytics/decisions", AnyMap{
			"decision": "Call the buyer", "deal_id": deal, "owner_id": apptest.SessionUserID(t, e),
			"review_on": day.Format(time.DateOnly),
		}, nil, nil)
	}
	if status := decide(today.AddDate(0, 0, -3)); status != http.StatusUnprocessableEntity {
		t.Errorf("a review day three days ago answered %d, want 422", status)
	}
	if status := decide(today.AddDate(0, 0, 2)); status != http.StatusCreated {
		t.Errorf("a review day two days ahead answered %d, want 201", status)
	}
}
