// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// A contact proposal is staged under who wrote, so its caption is a person.
func TestAContactProposalIsStagedUnderWhoWrote(t *testing.T) {
	for _, tc := range []struct {
		row  capture.PendingCounterparty
		want string
	}{
		{capture.PendingCounterparty{Email: "boris@customer.example", DisplayName: "Boris Sambil"}, "Boris Sambil <boris@customer.example>"},
		{capture.PendingCounterparty{Email: "boris@customer.example", DisplayName: "  "}, "boris@customer.example"},
	} {
		if got := *counterpartyLabel(tc.row); got != tc.want {
			t.Errorf("label for %+v = %q, want %q", tc.row, got, tc.want)
		}
	}
}
