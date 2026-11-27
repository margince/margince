// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// The Progress work list names a deal's next task as History does: a task
// Margince wrote reads in the language the screen asks for.
func TestTheProgressWorkListNamesATaskMarginceWroteInTheReadersLanguage(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Open with a written task", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
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
	subject := systemtext.Literal("automation.new_lead")
	if _, _, err := activities.NewStore(e.DB()).LogActivity(principal.WithActor(ctx, actor), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: new(ids.From[ids.UserKind](actor.UserID)),
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("automation.new_lead")},
		Links:      []activities.ActivityLinkInput{{EntityType: "deal", EntityID: ids.MustParse(deal)}},
	}); err != nil {
		t.Fatal(err)
	}

	var work crmcontracts.SalesProgressDealPage
	if status := e.Call(t, "GET", "/v1/analytics/progress/deals?limit=10", nil,
		map[string]string{"X-Margince-Display-Locale": "de"}, &work); status != http.StatusOK {
		t.Fatalf("reading the work list → %d", status)
	}
	open := dealWorkOf(work, deal)
	if open == nil || open.NextTask == nil || open.NextTask.Subject != germanNewLeadTask {
		t.Errorf("the deal's next task = %+v, want %q", open, germanNewLeadTask)
	}
}
