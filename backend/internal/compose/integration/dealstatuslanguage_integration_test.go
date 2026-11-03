// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestChangingDealCardLanguagePreservesTheReadersDismissal(t *testing.T) {
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Original deal name", pipeline, stage, &e.Rep1))
	now := clocktest.Now(t)
	svc := dealstatus.NewService(e.Pool, e.Deals, e.Activities, dealrooms.NewStore(e.DB()), func() time.Time { return now })
	SetBaseLanguage(e.Admin(), t, e.Pool, "en")
	before, err := svc.ReadFacts(e.Admin(), deal)
	if err != nil {
		t.Fatal(err)
	}
	if before.Next == nil || before.Next.Action != dealstatus.ActionCreateTask || before.Next.Fingerprint == nil {
		t.Fatalf("expected a task proposal, got %+v", before.Next)
	}
	fingerprint := *before.Next.Fingerprint
	for _, language := range []string{"de", "vi"} {
		if err := svc.DismissMove(e.Admin(), deal, fingerprint); err != nil {
			t.Fatal(err)
		}
		SetBaseLanguage(e.Admin(), t, e.Pool, language)
		card, err := svc.ReadFacts(e.Admin(), deal)
		if err != nil {
			t.Fatal(err)
		}
		if card.Next != nil {
			t.Fatalf("%s resurrected the dismissed work", language)
		}
		queued, err := svc.CurrentMoves(e.Admin(), []ids.UUID{deal.UUID})
		if err != nil {
			t.Fatal(err)
		}
		if _, found := queued[deal.UUID]; found {
			t.Fatal("language change returned dismissed work to the queue")
		}
		if err := svc.RestoreMove(e.Admin(), deal, fingerprint); err != nil {
			t.Fatal(err)
		}
		card, err = svc.ReadFacts(e.Admin(), deal)
		if err != nil {
			t.Fatal(err)
		}
		if card.Next == nil || card.Next.Fingerprint == nil || *card.Next.Fingerprint != fingerprint {
			t.Fatalf("%s changed the restored work identity", language)
		}
		queued, err = svc.CurrentMoves(e.Admin(), []ids.UUID{deal.UUID})
		if err != nil {
			t.Fatal(err)
		}
		if _, found := queued[deal.UUID]; !found {
			t.Fatal("restoring translated work did not restore its queue entry")
		}
		if card.Next.Reason == before.Next.Reason || card.Story.Sentences[0].Text == before.Story.Sentences[0].Text {
			t.Fatalf("%s did not translate the complete card", language)
		}
	}
}
