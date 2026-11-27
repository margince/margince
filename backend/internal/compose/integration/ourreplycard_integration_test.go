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
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The deal card follows the newest message on the request's thread. Our reply
// after the request turns "your move" into "you replied"; their next message
// turns it back. The request stays open the whole time.
func TestTheDealCardSaysWeRepliedUntilTheBuyerWritesAgain(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	deal := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))
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
	at := clocktest.Now(t).Add(-72 * time.Hour).Truncate(time.Second)
	mail := func(direction, body string, when time.Time) ids.UUID {
		t.Helper()
		subject := "Re: Pricing for phase two"
		row, _, err := store.LogActivity(ctx, activities.LogActivityInput{
			Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &when,
			ThreadKey: "pricing-phase-two", Source: "test", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: dealID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(row.Id)
	}
	request := mail("inbound", "Could you send the pricing for phase two?", at)
	if _, err := store.SetOwedVerdict(ctx, request, activities.OwedVerdictAsksUs, "prompts-test", at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	reminder := takeTheRequest(t, e, deal)

	card := readDealCard(t, e, deal)
	if card.ReplyTo == nil || *card.ReplyTo != crmcontracts.Id(request) || card.OurReply != nil {
		t.Fatalf("before our reply the card does not say the reply is owed: %+v", card)
	}

	replied := at.Add(5 * time.Hour)
	mail("outbound", "Thanks, I will check.", replied)
	card = readDealCard(t, e, deal)
	if card.ReplyTo != nil || card.RequestBasis != nil {
		t.Fatalf("after our reply the card still says the buyer waits: %+v", card)
	}
	if card.OurReply == nil || !card.OurReply.RepliedAt.Equal(replied) || card.OurReply.ReminderId == nil || ids.UUID(*card.OurReply.ReminderId) != reminder {
		t.Fatalf("the card does not say we replied with the reminder to mark done: %+v", card.OurReply)
	}

	mail("inbound", "Any news on the pricing?", replied.Add(time.Hour))
	card = readDealCard(t, e, deal)
	if card.OurReply != nil || card.ReplyTo == nil || *card.ReplyTo != crmcontracts.Id(request) {
		t.Fatalf("their newer message did not make it our move again: %+v", card)
	}
}

// takeTheRequest accepts the request through the card's own move, filing the
// reminder a human holds. That keeps the request current for its reader
// whatever the waiting horizon says.
func takeTheRequest(t *testing.T, e *apptest.AppEnv, deal string) ids.UUID {
	t.Helper()
	card := readDealCard(t, e, deal)
	if card.Next == nil || card.Next.Action != "create_task" {
		t.Fatalf("the card does not offer to take the request: %+v", card.Next)
	}
	var task crmcontracts.Activity
	if code := e.Call(t, "POST", "/v1/tasks", card.Next.Arguments, nil, &task); code != http.StatusCreated {
		t.Fatalf("taking the request: %d", code)
	}
	return ids.UUID(task.Id)
}

func readDealCard(t *testing.T, e *apptest.AppEnv, deal string) crmcontracts.DealStatusCard {
	t.Helper()
	var card crmcontracts.DealStatusCard
	if code := e.Call(t, "GET", "/v1/deals/"+deal+"/status", nil, nil, &card); code != http.StatusOK {
		t.Fatalf("reading the deal card: %d", code)
	}
	return card
}
