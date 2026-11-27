// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The Progress work list gives a deal's next task its picked date, so a reader
// in any zone prints the chosen day.
func TestTheProgressWorkListCarriesATasksPickedDate(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Open with a dated task", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 50_000, "currency": "EUR",
	})
	ctx := e.DealWriterContext(t)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
	actor.UserID = ids.MustParse(signedInUser(t, e))
	actor.ID = "human:" + actor.UserID.String()
	intent := &crmcontracts.IntentTime{WallClock: "2026-11-16T23:59:59", Timezone: "America/Los_Angeles"}
	if _, _, err := activities.NewStore(e.DB()).LogActivity(principal.WithActor(ctx, actor), activities.LogActivityInput{
		Kind: "task", Subject: new("Send the contract"), Source: "manual", DueIntent: intent,
		AssigneeID: new(ids.From[ids.UserKind](actor.UserID)),
		Links:      []activities.ActivityLinkInput{{EntityType: "deal", EntityID: ids.MustParse(deal)}},
	}); err != nil {
		t.Fatal(err)
	}
	var work crmcontracts.SalesProgressDealPage
	if status := e.Call(t, "GET", "/v1/analytics/progress/deals?limit=10", nil, nil, &work); status != http.StatusOK {
		t.Fatalf("reading the work list → %d", status)
	}
	row := dealWorkOf(work, deal)
	if row == nil || row.NextTask == nil {
		t.Fatalf("the deal has no next task: %+v", row)
	}
	task := row.NextTask
	if task.DueHasTime == nil || *task.DueHasTime {
		t.Errorf("due_has_time = %v, want false for a date with no time", task.DueHasTime)
	}
	if task.DueIntent == nil || task.DueIntent.Timezone != intent.Timezone || !strings.HasPrefix(task.DueIntent.WallClock, intent.WallClock) {
		t.Errorf("due_intent = %+v, want %+v", task.DueIntent, intent)
	}
}
