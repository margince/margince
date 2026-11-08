// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestContactCommercialContextExcludesEndedStakeholderSeats(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Open negotiation", pipeline, open, &e.Rep1))
	buyer := ids.From[ids.ContactKind](e.SeedContact(t, "Current buyer", &e.Rep1))
	former := ids.From[ids.ContactKind](e.SeedContact(t, "Former stakeholder", &e.Rep1))
	for _, contact := range []ids.ContactID{buyer, former} {
		edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
			Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Source: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		if contact == former {
			ended := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			if _, err := e.Contacts.UpdateRelationship(ctx, edge.ID, contacts.UpdateRelationshipInput{EndedAt: &ended}); err != nil {
				t.Fatal(err)
			}
		}
	}
	service := contactRoomService(e)
	current, err := service.Assemble(ctx, buyer)
	if err != nil {
		t.Fatal(err)
	}
	if current.Commercial == nil || current.Commercial.Deal == nil {
		t.Fatal("the current stakeholder has no open deal")
	}
	if len(current.Commercial.Committee) != 0 {
		t.Fatalf("ended stakeholders remain in the buying committee: %v", current.Commercial.Committee)
	}
	past, err := service.Assemble(ctx, former)
	if err != nil {
		t.Fatal(err)
	}
	if past.Commercial != nil && past.Commercial.Deal != nil {
		t.Fatal("an ended stakeholder seat still supplies an active deal")
	}
}
