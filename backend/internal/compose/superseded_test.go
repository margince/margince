// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
)

// A supersession question about either half of the money pair must ask about
// both. Restoring a minor-unit count under a currency it was never denominated
// in produces a number wrong by the scale difference, and it is wrong silently:
// the figure is plausible in both denominations.
func TestSupersessionAsksAboutBothHalvesOfTheMoneyPair(t *testing.T) {
	asked := strings.Join(coupledKeys([]string{"amount_minor"}), ",")
	if asked != "amount_minor,currency" {
		t.Errorf("coupledKeys([amount_minor]) = %q, want the pair", asked)
	}
	asked = strings.Join(coupledKeys([]string{"currency"}), ",")
	if asked != "amount_minor,currency" {
		t.Errorf("coupledKeys([currency]) = %q, want the pair", asked)
	}
}

// A field with no sibling is asked about alone. Coupling every key would refuse
// restores nothing superseded.
func TestSupersessionDoesNotCoupleAnOrdinaryField(t *testing.T) {
	asked := strings.Join(coupledKeys([]string{"full_name"}), ",")
	if asked != "full_name" {
		t.Errorf("coupledKeys([full_name]) = %q, want it alone", asked)
	}
}

// A later change to the CURRENCY supersedes a restore of the AMOUNT, and the
// refusal names the field the caller asked to put back rather than the sibling
// that moved.
func TestALaterCurrencyChangeSupersedesAnAmountRestore(t *testing.T) {
	got := reportedAs([]string{"currency"}, []string{"amount_minor"})
	if len(got) != 1 || got[0] != "amount_minor" {
		t.Errorf("reportedAs(currency moved, amount asked) = %v, want [amount_minor]", got)
	}
}

// A later write of an unrelated key supersedes nothing the caller asked about.
// Reporting it would refuse a restore the trail permits.
func TestAnUnrelatedLaterWriteSupersedesNothing(t *testing.T) {
	if got := reportedAs([]string{"title"}, []string{"full_name"}); len(got) != 0 {
		t.Errorf("reportedAs(title moved, full_name asked) = %v, want none", got)
	}
}
