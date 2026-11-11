// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// Expected close dates are whole days, so a window that ends partway through
// a day still counts the deals closing on it.
func TestACustomCloseWindowCountsEveryDayItTouches(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	closeDay := clocktest.Now(t).UTC().Truncate(24*time.Hour).AddDate(0, 0, 10)
	createdID(t, e, "/v1/deals", AnyMap{
		"name": "Closes in ten days", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
		"amount_minor": 70_000, "currency": "EUR", "expected_close_date": closeDay.Format(time.DateOnly),
	})
	for name, end := range map[string]time.Time{
		"into the close day":          closeDay.Add(14 * time.Hour),
		"up to the close day's start": closeDay,
	} {
		query := url.Values{
			"period": {"this_month"}, "target_basis": {"month"}, "metrics": {"open_pipeline"}, "close_window": {"custom"},
			"close_start_at": {closeDay.AddDate(0, 0, -1).Format(time.RFC3339)}, "close_end_at": {end.Format(time.RFC3339)},
		}
		var evaluation crmcontracts.ReportingEvaluation
		mustCall(t, e, "GET", "/v1/analytics/evaluate?"+query.Encode(), nil, http.StatusOK, &evaluation)
		value := "none"
		if len(evaluation.Metrics) == 1 && evaluation.Metrics[0].Value != nil {
			value = fmt.Sprint(*evaluation.Metrics[0].Value)
		}
		if counted := value == "70000"; counted != end.After(closeDay) {
			t.Errorf("a window ending %s counts an open pipeline of %s", name, value)
		}
	}
}
