// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSyncDirectionsAreIndependentAndEnablingReadsStartsProspectively(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep)
	state, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	e.enableOutbound(t)
	link := state.Link
	event := Event{EventID: ids.NewV7(), EventType: "deal.updated", Tier: TierImportant, Deals: []ids.UUID{e.deal}}
	if err := e.store.RecordEvent(e.system(), event); err != nil {
		t.Fatal(err)
	}
	enabled, disabled := true, false
	state, err = e.store.SetSyncOptions(ctx, RecordDeal, e.deal, SyncOptions{Read: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Link.ReadDisabled || state.Link.PostDisabled {
		t.Fatalf("read disable changed outbound: %+v", state.Link)
	}
	pending, err := e.store.PendingFor(e.system(), link.ID)
	if err != nil || len(pending.Items) != 1 {
		t.Fatalf("disabling reads discarded an outbound event: %+v %v", pending, err)
	}
	pulls, err := e.store.PullLinks(e.system(), link.ConnectionID)
	if err != nil || len(pulls) != 0 {
		t.Fatalf("read disabled link still polled: %+v %v", pulls, err)
	}
	state, err = e.store.SetSyncOptions(ctx, RecordDeal, e.deal, SyncOptions{Read: &enabled, Post: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if state.Link.ReadDisabled || !state.Link.PostDisabled || state.Link.ReadSince == nil {
		t.Fatalf("directions after switch: %+v", state.Link)
	}
	pending, err = e.store.PendingFor(e.system(), link.ID)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("queued send survived disabled outbound: %+v %v", pending, err)
	}
	event.EventID = ids.NewV7()
	if err := e.store.RecordEvent(e.system(), event); err != nil {
		t.Fatal(err)
	}
	pending, err = e.store.PendingFor(e.system(), link.ID)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("new outbound event queued while disabled: %+v %v", pending, err)
	}
	pulls, err = e.store.PullLinks(e.system(), link.ConnectionID)
	if err != nil || len(pulls) != 1 {
		t.Fatalf("inbound did not remain enabled: %+v %v", pulls, err)
	}
	state, err = e.store.SetSyncOptions(ctx, RecordDeal, e.deal, SyncOptions{Post: &enabled})
	if err != nil || state.Link.PostDisabled || state.Link.ReadDisabled {
		t.Fatalf("re-enable outbound: %+v %v", state, err)
	}
	pending, err = e.store.PendingFor(e.system(), link.ID)
	if err != nil || len(pending.Items) != 0 {
		t.Fatalf("re-enable replayed held posts: %+v %v", pending, err)
	}
}

func TestMessageDetailAndFrequencyChangeIndependentlyAndPostedItemsKeepTheirFormat(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep)
	state, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate, Verbosity: VerbosityEverything})
	if err != nil {
		t.Fatal(err)
	}
	e.enableOutbound(t)
	event := Event{EventID: ids.NewV7(), EventType: "deal.updated", Tier: TierImportant, Deals: []ids.UUID{e.deal}}
	if err := e.store.RecordEvent(e.system(), event); err != nil {
		t.Fatal(err)
	}
	full := MessageDetailFull
	state, err = e.store.SetSyncOptions(ctx, RecordDeal, e.deal, SyncOptions{MessageDetail: &full})
	if err != nil || state.Link.MessageDetail != MessageDetailFull || state.Link.Verbosity != VerbosityEverything {
		t.Fatalf("detail changed frequency: %+v %v", state, err)
	}
	pending, err := e.store.PendingFor(e.system(), state.Link.ID)
	if err != nil || len(pending.Items) != 1 || pending.Items[0].MessageDetail != MessageDetailFull {
		t.Fatalf("queued item kept obsolete detail: %+v %v", pending, err)
	}
	if err := e.store.MarkPosted(e.system(), state.Link.ID, pending.Items[0].ID, "M1", []ids.UUID{pending.Items[0].ID}, MessageDetailFull); err != nil {
		t.Fatal(err)
	}
	brief := MessageDetailBrief
	if _, err := e.store.SetSyncOptions(ctx, RecordDeal, e.deal, SyncOptions{MessageDetail: &brief}); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, "SELECT count(*) FROM chat_post_item WHERE message_detail='full'"); n != 1 {
		t.Fatal("changing detail expanded or rewrote an already delivered post")
	}
}

func TestAnOldFrequencyDefaultSurvivesTheFirstIndependentSettingsChange(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep)
	state, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate, Verbosity: VerbosityEverything})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, "UPDATE chat_link SET message_detail=NULL WHERE id=$1", state.Link.ID)
	state, err = e.store.GetState(ctx, RecordDeal, e.deal)
	if err != nil || state.Link.MessageDetail != MessageDetailFull {
		t.Fatalf("old Everything choice lost Full detail: %+v %v", state, err)
	}
	state, err = e.store.SetVerbosity(ctx, RecordDeal, e.deal, VerbosityImportant)
	if err != nil || state.Link.MessageDetail != MessageDetailFull || state.Link.Verbosity != VerbosityImportant {
		t.Fatalf("frequency changed effective message detail: %+v %v", state, err)
	}
}
