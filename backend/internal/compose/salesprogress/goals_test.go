// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func okAmount(minor int64) crmcontracts.SalesProgressAmount {
	return crmcontracts.SalesProgressAmount{Status: "ok", AmountMinor: &minor}
}

func TestComparedReadsCompletionAndASignedDelta(t *testing.T) {
	goal := int64(10000000)
	completion, delta := compared(okAmount(6000000), crmcontracts.SalesProgressGoal{Status: "set", AmountMinor: &goal})
	if completion == nil || *completion != 0.6 || delta == nil || *delta != -4000000 {
		t.Fatalf("compared = %v %v, want 0.6 and -4000000", completion, delta)
	}
	zero := int64(0)
	completion, delta = compared(okAmount(500), crmcontracts.SalesProgressGoal{Status: "zero", AmountMinor: &zero})
	if completion != nil || delta == nil || *delta != 500 {
		t.Fatalf("a zero goal compares as %v %v, want no completion and delta 500", completion, delta)
	}
	subtotal := int64(11000000)
	if completion, delta = compared(okAmount(9000000), crmcontracts.SalesProgressGoal{Status: "incomplete", AmountMinor: &subtotal}); completion != nil || delta != nil {
		t.Fatal("an incomplete goal supports no completion or delta")
	}
	partial := crmcontracts.SalesProgressAmount{Status: "partial", AmountMinor: &goal}
	if completion, delta = compared(partial, crmcontracts.SalesProgressGoal{Status: "set", AmountMinor: &goal}); completion != nil || delta != nil {
		t.Fatal("a partial amount supports no comparison")
	}
}
