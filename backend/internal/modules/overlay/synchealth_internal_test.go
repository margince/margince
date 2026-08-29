// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/overlaybudget"
)

// The two-window collapse in isolation, like aggregateState's own table: a
// shed on EITHER window throttles real reads, so it must win over the other
// window's calmer band — including the search-only arm, which is the one a
// swapped comparison would silently invert.
func TestWorstBudgetBandTakesTheWorseWindow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rest, srch string
		want       string
	}{
		{"both ok", overlaybudget.BandOK, overlaybudget.BandOK, overlaybudget.BandOK},
		{"rest warns", overlaybudget.BandWarn, overlaybudget.BandOK, overlaybudget.BandWarn},
		{"search alone sheds", overlaybudget.BandOK, overlaybudget.BandShed, overlaybudget.BandShed},
		{"shed outranks warn", overlaybudget.BandWarn, overlaybudget.BandShed, overlaybudget.BandShed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := worstBudgetBand(overlaybudget.Budget{Band: tc.rest, SearchBand: tc.srch})
			if got != tc.want {
				t.Fatalf("worstBudgetBand(%s, %s) = %s, want %s", tc.rest, tc.srch, got, tc.want)
			}
		})
	}
}
