// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package currencylock

import (
	"maps"
	"strings"
	"testing"
)

func TestTheProseStatesTheFactsItWasWordedFrom(t *testing.T) {
	for name, tc := range map[string]struct {
		lock Lock
		want []string
	}{
		"frozen rates name what was counted, and only that": {
			Lock{Reason: RatesFrozen, Count: 3, Frozen: []Counted{{"deal", 0}, {"finance_invoice", 2}, {"contract", 1}}},
			[]string{"3 record(s) (mirrored invoices: 2, contracts: 1) have already frozen"},
		},
		"frozen rates counted by no table still name the count": {
			Lock{Reason: RatesFrozen, Count: 3},
			[]string{"3 record(s) have already frozen"},
		},
		"a rate sheet names its count, its base and the repair": {
			Lock{Reason: RateSheet, Count: 2, BaseCurrency: "EUR"},
			[]string{"2 rate(s)", "convert into EUR", "clear the rate sheet first"},
		},
		"a rate sheet with no stored base says so instead of a code": {
			Lock{Reason: RateSheet, Count: 1},
			[]string{"1 rate(s)", "convert into the base it was set to", "clear the rate sheet first"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			prose := tc.lock.Prose()
			if refusal := tc.lock.Refusal(); refusal != prose {
				t.Errorf("Refusal() = %q, want the prose %q", refusal, prose)
			}
			for _, want := range tc.want {
				if !strings.Contains(prose, want) {
					t.Errorf("Prose() = %q, want it to contain %q", prose, want)
				}
			}
		})
	}
}

func TestTheProseNamesNoRecordKindItDidNotCount(t *testing.T) {
	prose := Lock{Reason: RatesFrozen, Count: 1, Frozen: []Counted{{"deal", 0}, {"contract", 1}}}.Prose()
	if strings.Contains(prose, "closed deals") {
		t.Errorf("Prose() = %q names closed deals, which counted none", prose)
	}
}

func TestALockWithNoKnownReasonHasNoProse(t *testing.T) {
	for name, lock := range map[string]Lock{
		"the zero lock":  {},
		"a stray reason": {Reason: "ledger_sealed", Count: 2, BaseCurrency: "EUR"},
	} {
		if prose := lock.Prose(); prose != "" {
			t.Errorf("%s: Prose() = %q, want none rather than another reason's sentence", name, prose)
		}
		if lock.Refusal() == "" {
			t.Errorf("%s: Refusal() is empty, want a sentence a refused write can carry", name)
		}
	}
}

func TestTheParamsStateTheFactsAndLeaveAnUnnamedBaseOut(t *testing.T) {
	for name, tc := range map[string]struct {
		lock Lock
		want map[string]string
	}{
		"a rate sheet names its base": {
			Lock{Reason: RateSheet, Count: 2, BaseCurrency: "EUR"},
			map[string]string{"reason": "rate_sheet", "count": "2", "base_currency": "EUR"},
		},
		"frozen rates name no table and no base": {
			Lock{Reason: RatesFrozen, Count: 12, Frozen: []Counted{{"deal", 12}}},
			map[string]string{"reason": "rates_frozen", "count": "12"},
		},
	} {
		if got := tc.lock.Params(); !maps.Equal(got, tc.want) {
			t.Errorf("%s: Params() = %v, want %v", name, got, tc.want)
		}
	}
}
