// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A seat on a deal says when the contact last wrote to us. It is read through
// the contact page's strength fold, and only for a caller who may read activities.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADealSeatCarriesWhenTheContactLastWroteAsADay(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	contactID := ids.From[ids.ContactKind](buyer)

	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Harbour renewal", PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	role := "champion"
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contactID, DealID: &dealID,
		Role: &role, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the contact: %v", err)
	}

	// A message they sent, logged by its date alone: the server keeps it at noon UTC.
	subject, sentOn := "Re: renewal terms", time.Date(2026, 5, 20, 16, 30, 0, 0, time.UTC)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: new("inbound"), Source: "manual",
		OccurredAt: &sentOn, OccurredPrecision: activities.PrecisionDay,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	}); err != nil {
		t.Fatalf("logging the message they sent: %v", err)
	}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	granted := coverageFor(t, e, coverageReaderPerms(true), dealID, now)
	got, ok := granted.SeatsInbound[buyer]
	if !ok {
		t.Fatalf("the seat carries no last message, though the contact wrote on 20 May: %+v",
			granted.SeatsInbound)
	}
	if want := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC); !got.At.Equal(want) || got.Precision != "day" {
		t.Errorf("last wrote = %v (%q), want %v as a day", got.At, got.Precision, want)
	}

	blind := coverageReaderPerms(true)
	delete(blind.Objects, "activity")
	refused := coverageFor(t, e, blind, dealID, now)
	if len(refused.Stakeholders) != 1 {
		t.Fatalf("a caller without the activity grant lost the seat itself: %d seats", len(refused.Stakeholders))
	}
	if len(refused.SeatsInbound) != 0 {
		t.Errorf("a caller without the activity grant was told when the contact wrote: %+v",
			refused.SeatsInbound)
	}
}
