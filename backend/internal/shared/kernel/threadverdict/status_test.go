// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package threadverdict

import "testing"

func TestRepairArgumentsCannotChangeTheHoldingRule(t *testing.T) {
	before := HoldsSQL("a.status")
	statuses := HoldingStatuses()
	statuses[0] = Pending
	if !Holds(Held) || HoldsSQL("a.status") != before {
		t.Fatal("mutating repair arguments changed reader authorization")
	}
}
