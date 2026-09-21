// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type requestReviewModel struct{ calls int }

func (m *requestReviewModel) Complete(context.Context, model.Request) (model.Response, error) {
	m.calls++
	return model.Response{}, errors.New("model unavailable")
}

func TestFactsOnlyRefreshNeverCallsTheModelEvenWhenTheRequestChanges(t *testing.T) {
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
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Read: true}
	actor.Permissions.Objects["installation_settings"] = principal.ObjectGrant{Read: true}
	var me crmcontracts.MeResponse
	if code := e.Call(t, "GET", "/v1/me", nil, nil, &me); code != http.StatusOK {
		t.Fatal("read current user")
	}
	actor.UserID = ids.UUID(me.User.Id)
	actor.ID = "human:" + actor.UserID.String()
	ctx = principal.WithActor(ctx, actor)
	store := activities.NewStore(e.DB())
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	now := at.Add(time.Hour)
	lane := &requestReviewModel{}
	service := dealstatus.NewService(e.Pool, deals.NewStore(e.DB(), deals.Installation{}), store, dealrooms.NewStore(e.DB()), func() time.Time { return now }).WithLane(lane, "test")
	subject, direction := "Please send slots", "inbound"
	source, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "email", Subject: &subject, Direction: &direction, ThreadKey: "facts-request", OccurredAt: &at, Source: "test", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: dealID}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(source.Id)
	if _, err := store.SetOwedVerdict(ctx, id, activities.OwedVerdictAsksUs, "prompts-test", time.Now()); err != nil {
		t.Fatal(err)
	}
	card, err := service.ReadFacts(ctx, ids.From[ids.DealKind](dealID))
	if err != nil {
		t.Fatal(err)
	}
	if card.Next == nil || card.Next.Action != "create_task" || lane.calls != 0 {
		t.Fatal("facts read missed work or called a model")
	}
	assertCachedRequestMove(ctx, t, service, dealID, "create_task")
	task, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "task", RequestActivityID: &id, Source: "ui"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	card, err = service.ReadFacts(ctx, ids.From[ids.DealKind](dealID))
	if err != nil {
		t.Fatal(err)
	}
	if card.Next == nil || card.Next.Action != "open_task" || lane.calls != 0 {
		t.Fatal("changed facts called a model or lost the reminder")
	}
	assertCachedRequestMove(ctx, t, service, dealID, "open_task")
	done := true
	if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	card, err = service.ReadFacts(ctx, ids.From[ids.DealKind](dealID))
	if err != nil {
		t.Fatal(err)
	}
	if card.Next != nil || card.ReplyTo != nil || lane.calls != 0 {
		t.Fatal("completion did not refresh without model work")
	}
	assertCachedRequestMove(ctx, t, service, dealID, "")
	if _, err := service.Get(ctx, ids.From[ids.DealKind](dealID), true); err != nil {
		t.Fatal(err)
	}
	if lane.calls == 0 {
		t.Fatal("test never connected the real model boundary")
	}
}

func assertCachedRequestMove(ctx context.Context, t *testing.T, service *dealstatus.Service, dealID ids.UUID, want string) {
	t.Helper()
	moves, err := service.CachedMoves(ctx, []ids.UUID{dealID})
	if err != nil {
		t.Fatal(err)
	}
	move, found := moves[dealID]
	if (want != "") != found || string(move.Action) != want {
		t.Fatalf("cached action = %q, present = %v, want %q", move.Action, found, want)
	}
}
