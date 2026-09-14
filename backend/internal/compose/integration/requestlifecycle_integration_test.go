// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWonDealKeepsOldSchedulingRequestUntilItsTaskIsCompleted(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	deal := apptest.ExerciseDealToWon(t, e, apptest.DiscoverSeededPipeline(t, e))
	dealID, err := ids.Parse(deal)
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.DealWriterContext(t)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing writer")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	store := activities.NewStore(e.DB())
	at := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	subject, body, direction := "Re: A meeting?", "Yes, please send a few meeting slots.", "inbound"
	message, _, err := store.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, ThreadKey: "scheduling-request",
		Source: "test", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: dealID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(message.Id)
	if _, err := store.SetCaptureLabel(ctx, id, "meeting"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetOwedVerdict(ctx, id, activities.OwedVerdictAsksUs); err != nil {
		t.Fatal(err)
	}
	// Push the request outside the recent-history window with real activity writes.
	for i := range 26 {
		at = at.Add(time.Hour)
		direction = "outbound"
		thread := "unrelated-" + ids.NewV7().String()
		if i == 0 {
			thread = "scheduling-request"
			body = "Thanks, I will check."
		}
		if _, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, ThreadKey: thread, Source: "test", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: dealID}}}); err != nil {
			t.Fatal(err)
		}
	}
	var card crmcontracts.DealStatusCard
	if code := e.Call(t, "GET", "/v1/deals/"+deal+"/status", nil, nil, &card); code != http.StatusOK {
		t.Fatalf("read card: %d", code)
	}
	if card.Next == nil || card.Next.Action != "create_task" || card.ReplyTo == nil || *card.ReplyTo != message.Id {
		t.Fatalf("won deal lost its old request: %+v", card)
	}
	var task crmcontracts.Activity
	if code := e.Call(t, "POST", "/v1/tasks", card.Next.Arguments, nil, &task); code != http.StatusCreated {
		t.Fatalf("accept request: %d", code)
	}
	var replay crmcontracts.Activity
	if code := e.Call(t, "POST", "/v1/tasks", card.Next.Arguments, nil, &replay); code != http.StatusOK || replay.Id != task.Id {
		t.Fatalf("acceptance was not idempotent: %d %+v", code, replay)
	}
	if task.SourceActivityId == nil || *task.SourceActivityId != message.Id {
		t.Fatal("task lost its source")
	}
	if code := e.Call(t, "GET", "/v1/deals/"+deal+"/status", nil, nil, &card); code != http.StatusOK || card.Next == nil || card.Next.Action != "open_task" {
		t.Fatalf("won deal hid its open task: %d %+v", code, card)
	}
	if code := e.Call(t, "PATCH", "/v1/activities/"+task.Id.String(), map[string]bool{"is_done": true}, nil, nil); code != http.StatusOK {
		t.Fatalf("complete request task: %d", code)
	}
	card = crmcontracts.DealStatusCard{}
	if code := e.Call(t, "GET", "/v1/deals/"+deal+"/status", nil, nil, &card); code != http.StatusOK || card.Next != nil || card.ReplyTo != nil {
		t.Fatalf("completed request stayed open: %d %+v", code, card)
	}
}
