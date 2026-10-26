// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package billingdemo

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestADeclarationIsValid(t *testing.T) {
	if err := New().Ingress[0].Validate(); err != nil {
		t.Fatalf("the ingress source: %v", err)
	}
}

func TestAPaidInvoiceSitsOnTheWonStage(t *testing.T) {
	conn := connection{member: "m", pipeline: "p", openStage: "open", wonStage: "won"}
	inv := invoice{erpID: "INV-1", name: "Acme", currency: "EUR", amountMinor: 5000}

	open := recordFor(conn, inv)
	inv.paid = true
	won := recordFor(conn, inv)

	if open.Deal.StageID != "open" || won.Deal.StageID != "won" {
		t.Errorf("stages = %q then %q, want open then won", open.Deal.StageID, won.Deal.StageID)
	}
	if won.Kind() != extension.KindDeal || won.Key != "INV-1" || won.System != "billing-demo" {
		t.Errorf("record = %+v, want a deal keyed by the invoice under billing-demo", won)
	}
	if got := *won.Deal.AmountMinor; got != 5000 {
		t.Errorf("amount = %d, want 5000", got)
	}
}

func TestAnIdIsCheckedBeforeTheDatabaseSeesIt(t *testing.T) {
	for id, want := range map[string]bool{
		"01a17608-0c4a-7086-b086-1caba78f6630": true,
		"not-an-id":                            false,
		"01a17608-0c4a-7086-b086-1caba78f663g": false,
		"":                                     false,
	} {
		if got := isUUID(id); got != want {
			t.Errorf("isUUID(%q) = %v, want %v", id, got, want)
		}
	}
}
