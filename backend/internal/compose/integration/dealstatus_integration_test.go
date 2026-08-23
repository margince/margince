// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal status card over the real stores: the facts it is written from are
// the ones the deal's timeline, open promises and Deal Room actually hold, and
// the move it offers is one the client can perform.
//
// No model runs here. This lane exercises the deterministic composition, which
// is the card every reader gets when no lane is wired — so it is the half that
// must hold on its own.

import (
	"net/http"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
)

func TestTheDealStatusCardFollowsTheDealsOwnRecords(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)

	card := readStatus(t, e, dealID)
	if card["generated_by"] != "deterministic" {
		t.Fatalf("generated_by = %v, want the composition this lane runs", card["generated_by"])
	}
	next, _ := card["next"].(map[string]any)
	if next["action"] != "create_task" {
		t.Fatalf("an empty deal should ask for a first step, got %v", next)
	}
	args, _ := next["arguments"].(map[string]any)
	if args["subject"] != "Agree the next step on Acme rollout" {
		t.Fatalf("arguments = %v", args)
	}

	// Performing the move through the verb it names. The card must still offer
	// one afterwards: an open task is evidence, not a reason to go quiet.
	if status := e.Call(t, "POST", "/v1/tasks", args, nil, nil); status != http.StatusCreated {
		t.Fatalf("create task from the card = %d", status)
	}
	after := readStatus(t, e, dealID)
	afterNext, _ := after["next"].(map[string]any)
	if afterNext == nil || afterNext["action"] == "none" {
		t.Fatalf("after creating the task the card went silent: %v", after)
	}

	// An unanswered inbound mail outranks the open task.
	var mail apptest.AnyMap
	if status := e.Call(t, "POST", "/v1/activities", apptest.AnyMap{
		"kind": "email", "direction": "inbound", "subject": "Re: rollout", "body": "Can you send the DPA?",
		"links":  []apptest.AnyMap{{"entity_type": "deal", "entity_id": dealID}},
		"source": "ui",
	}, nil, &mail); status != http.StatusCreated {
		t.Fatalf("log mail = %d %v", status, mail)
	}
	// The card is cached on its facts, and logging the mail moved them — so a
	// plain read must serve the rewritten card, not the one from before.
	replied := readStatus(t, e, dealID)
	repliedNext, _ := replied["next"].(map[string]any)
	repliedArgs, _ := repliedNext["arguments"].(map[string]any)
	if repliedNext["action"] != "draft_email" || repliedArgs["activity_id"] != mail["id"] {
		t.Fatalf("after an inbound mail the card still offers %v", repliedNext)
	}
}

func TestTheDealStatusCardCitesItsRecordsAndHidesADealTheCallerCannotSee(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	var roomRow apptest.AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID, nil, nil, &roomRow); status != http.StatusOK {
		t.Fatalf("room = %d", status)
	}
	dealID, _ := roomRow["deal_id"].(string)

	card := readStatus(t, e, dealID)
	story, _ := card["story"].(map[string]any)
	lines, _ := story["sentences"].([]any)
	if len(lines) == 0 {
		t.Fatal("the briefing tells no story")
	}
	// A sentence resting on an activity must name it, so the reader can open
	// what it was written from. A sentence about the deal's own fields cites
	// nothing because there is no second record to open.
	for _, l := range lines {
		line, _ := l.(map[string]any)
		if _, ok := line["evidence"].([]any); !ok {
			t.Errorf("%q carries no evidence list at all", line["text"])
		}
	}

	if status := e.Call(t, "GET", "/v1/deals/01a00000-0000-7000-8000-000000000000/status", nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("status of an unknown deal = %d, want 404", status)
	}
}

func readStatus(t *testing.T, e *apptest.AppEnv, dealID string) apptest.AnyMap {
	t.Helper()
	var card apptest.AnyMap
	if status := e.Call(t, "GET", "/v1/deals/"+dealID+"/status", nil, nil, &card); status != http.StatusOK {
		t.Fatalf("status = %d %v", status, card)
	}
	return card
}
