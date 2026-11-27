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

// A decision's review day is a day of the installation's calendar. It reads
// back as the day that was sent, and no door moves it to a day already over.
func TestADecisionReviewDayIsAnInstallationDayOverEveryDoor(t *testing.T) {
	t.Parallel()
	for _, zone := range []string{"America/Los_Angeles", "Pacific/Auckland"} {
		t.Run(zone, func(t *testing.T) {
			e := apptest.SetupApp(t)
			e.BootstrapWorkspace(t)
			if _, err := e.Owner.Exec(t.Context(), `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone); err != nil {
				t.Fatalf("setting the installation zone: %v", err)
			}
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			stages := apptest.DiscoverSeededPipeline(t, e)
			deal := createdID(t, e, "/v1/deals", AnyMap{
				"name": "Review day", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
				"amount_minor": 70_000, "currency": "EUR",
			})
			today := clocktest.Now(t).In(loc)
			day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
			decide := func(reviewOn string, out any) int {
				return e.Call(t, "POST", "/v1/analytics/decisions", AnyMap{
					"decision": "Call the buyer", "deal_id": deal, "owner_id": apptest.SessionUserID(t, e), "review_on": reviewOn,
				}, nil, out)
			}
			if status := decide(day(-1), nil); status != http.StatusUnprocessableEntity {
				t.Fatalf("yesterday in %s answered %d, want 422", zone, status)
			}
			var made struct {
				ID       string `json:"id"`
				ReviewOn string `json:"review_on"`
			}
			if status := decide(day(2), &made); status != http.StatusCreated || made.ReviewOn != day(2) {
				t.Fatalf("a day two ahead answered %d with review_on %q, want 201 and %s", status, made.ReviewOn, day(2))
			}
			var listed struct {
				Data []struct {
					ReviewOn string `json:"review_on"`
				} `json:"data"`
			}
			mustCall(t, e, "GET", "/v1/analytics/decisions", nil, http.StatusOK, &listed)
			if len(listed.Data) != 1 || listed.Data[0].ReviewOn != day(2) {
				t.Fatalf("the list reads %+v, want one decision on %s", listed.Data, day(2))
			}
			back := today.AddDate(0, 0, -1)
			if status := e.Call(t, "PATCH", "/v1/activities/"+made.ID, AnyMap{"due_at": back.UTC().Format(time.RFC3339)}, nil, nil); status != http.StatusUnprocessableEntity {
				t.Errorf("moving the decision's task to yesterday answered %d, want 422", status)
			}
			if status := e.Call(t, "PATCH", "/v1/activities/"+made.ID, AnyMap{"due_intent": AnyMap{"wall_clock": day(-1) + "T23:59:59", "timezone": zone}}, nil, nil); status != http.StatusUnprocessableEntity {
				t.Errorf("moving the decision's task to a past chosen day answered %d, want 422", status)
			}
			if status := e.Call(t, "PATCH", "/v1/activities/"+made.ID, AnyMap{"due_intent": AnyMap{"wall_clock": day(3) + "T23:59:59", "timezone": zone}}, nil, nil); status != http.StatusOK {
				t.Errorf("moving the decision's task to a later day answered %d, want 200", status)
			}
		})
	}
}
