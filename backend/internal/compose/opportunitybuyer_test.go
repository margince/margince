// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOpportunityBuyerRequiresAQuotedNeedAndAVerifiedParticipant(t *testing.T) {
	contact, source := ids.NewV7(), ids.NewV7()
	ref := "contact:" + contact.String()
	for _, role := range []string{"from", "to", "cc"} {
		t.Run(role, func(t *testing.T) {
			thread := settledThread{Messages: []threadMessage{{ID: source, Body: "We need a platform migration.", Parties: []commitmentParty{{Ref: ref, Contact: &contact, Role: role}}}}}
			event := extractedEvent{Kind: "new_opportunity", MessageID: source.String(), Quote: "We need a platform migration.", BuyerRef: ref, CommercialRole: "buyer"}
			if reason := validateEventEvidence(event, thread); reason != "" {
				t.Fatalf("verified external buyer rejected: %s", reason)
			}
			buyer, direction := opportunityBuyer(event, thread)
			if buyer == nil || *buyer != contact || direction != "buyer" {
				t.Fatalf("buyer = %v, role = %s", buyer, direction)
			}
			event.BuyerRef = "contact:" + ids.NewV7().String()
			if buyer, role := opportunityBuyer(event, thread); buyer != nil || role != "unknown" {
				t.Fatalf("unverified buyer = %v/%s", buyer, role)
			}
			event.BuyerRef, event.Quote = ref, "Invented buying intent"
			if buyer, role := opportunityBuyer(event, thread); buyer != nil || role != "unknown" {
				t.Fatalf("unquoted buyer = %v/%s", buyer, role)
			}
		})
	}
}

func TestCommercialIntermediariesCannotBecomeTheBuyer(t *testing.T) {
	contact, source := ids.NewV7(), ids.NewV7()
	ref := "contact:" + contact.String()
	thread := settledThread{Messages: []threadMessage{{ID: source, Body: "We can supply the platform.", Parties: []commitmentParty{{Ref: ref, Contact: &contact, Role: "to"}}}}}
	for _, role := range []string{"supplier", "partner", "provider", "unknown"} {
		event := extractedEvent{Kind: "new_opportunity", MessageID: source.String(), Quote: "We can supply the platform.", BuyerRef: ref, CommercialRole: role}
		buyer, got := opportunityBuyer(event, thread)
		if buyer != nil || got != role {
			t.Fatalf("%s attribution = %v/%s", role, buyer, got)
		}
	}
}

func TestOwnPurchaseCannotNameOurSeatAsTheCustomer(t *testing.T) {
	seat, source := ids.NewV7(), ids.NewV7()
	ref := "seat:" + seat.String()
	body := "We need a migration platform for our own sales team. Your company would be our supplier for this project."
	thread := settledThread{Messages: []threadMessage{{ID: source, Body: body, Parties: []commitmentParty{{Ref: ref, Seat: &seat, Role: "from"}}}}}
	event := extractedEvent{Kind: extractKindOpportunity, MessageID: source.String(), Quote: body, BuyerRef: ref, CommercialRole: commercialRoleBuyer}
	if reason := validateEventEvidence(event, thread); reason == "" {
		t.Fatal("our own seat was accepted as an external buyer")
	}
	if buyer, role := opportunityBuyer(event, thread); buyer != nil || role != commercialRoleUnknown {
		t.Fatalf("our own purchase attributed to buyer %v/%s", buyer, role)
	}
	event.CommercialRole, event.BuyerRef = commercialRoleSupplier, ""
	if reason := validateEventEvidence(event, thread); reason != "" {
		t.Fatalf("supplier context rejected: %s", reason)
	}
}
