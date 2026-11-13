// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func linesOfLength(n int) []crmcontracts.MagicLine {
	return make([]crmcontracts.MagicLine, n)
}

func TestALaneReadToItsBoundIsNamedCut(t *testing.T) {
	floor, whole := true, false
	cases := []struct {
		name    string
		receipt crmcontracts.MagicReceipt
		want    []crmcontracts.MagicReceiptCutAtLimit
	}{
		{"every lane under its bound", crmcontracts.MagicReceipt{
			Done: linesOfLength(2), NeedsYou: linesOfLength(4), CouldNotComplete: linesOfLength(0),
			Watching: linesOfLength(9), DoneIsFloor: &whole,
		}, []crmcontracts.MagicReceiptCutAtLimit{}},
		{"lanes at their bound", crmcontracts.MagicReceipt{
			Done: linesOfLength(5), NeedsYou: linesOfLength(5), CouldNotComplete: linesOfLength(5),
			DoneIsFloor: &whole,
		}, []crmcontracts.MagicReceiptCutAtLimit{
			crmcontracts.MagicReceiptCutAtLimitDone, crmcontracts.MagicReceiptCutAtLimitNeedsYou,
			crmcontracts.MagicReceiptCutAtLimitCouldNotComplete,
		}},
		{"a done floor under its bound", crmcontracts.MagicReceipt{
			Done: linesOfLength(1), DoneIsFloor: &floor,
		}, []crmcontracts.MagicReceiptCutAtLimit{crmcontracts.MagicReceiptCutAtLimitDone}},
	}
	for _, c := range cases {
		if got := cutLanes(c.receipt, 5); !slices.Equal(got, c.want) {
			t.Errorf("%s: cut = %v, want %v", c.name, got, c.want)
		}
	}
}
