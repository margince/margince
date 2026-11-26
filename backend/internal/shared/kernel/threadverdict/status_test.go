// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package threadverdict

import (
	"strings"
	"testing"
)

func TestRepairArgumentsCannotChangeTheHoldingRule(t *testing.T) {
	before := HoldsSQL("a.status")
	statuses := HoldingStatuses()
	statuses[0] = Pending
	if !Holds(Held) || HoldsSQL("a.status") != before {
		t.Fatal("mutating repair arguments changed reader authorization")
	}
}

// The set-based rule names the statuses HoldsMessages answers true for, and no other.
func TestHoldsMessagesSQLNamesWhatHoldsMessagesHolds(t *testing.T) {
	got := HoldsMessagesSQL("v.status")
	for status := range statusRules {
		if named := strings.Contains(got, "'"+status+"'"); named != HoldsMessages(status) {
			t.Errorf("%s: named in SQL = %v, HoldsMessages = %v", status, named, HoldsMessages(status))
		}
	}
}
