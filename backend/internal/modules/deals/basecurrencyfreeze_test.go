// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/currencylock"
)

// The refusal names each table the freeze counts in words. A table added to
// the count without words would reach an operator as its SQL name.
func TestTheRefusalHasWordsForEveryFrozenRateTable(t *testing.T) {
	for _, table := range frozenRateTables {
		if _, named := currencylock.FrozenKind(table); !named {
			t.Errorf("currencylock has no words for %s, which the base-currency freeze counts", table)
		}
	}
}
