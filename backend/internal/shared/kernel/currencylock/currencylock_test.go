// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package currencylock

import (
	"strings"
	"testing"
)

func TestTheProseStatesTheFactsItWasWordedFrom(t *testing.T) {
	for name, tc := range map[string]struct {
		lock Lock
		want []string
	}{
		"frozen rates name every kind of record that counts": {
			Lock{Reason: RatesFrozen, Count: 3},
			[]string{"3 record(s)", "closed deals", "sent offers", "mirrored invoices", "contracts", "commission entries"},
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
			for _, want := range tc.want {
				if !strings.Contains(prose, want) {
					t.Errorf("Prose() = %q, want it to contain %q", prose, want)
				}
			}
		})
	}
}
