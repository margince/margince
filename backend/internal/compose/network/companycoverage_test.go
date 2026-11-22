// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import "testing"

// The account verdict follows the published threshold. Enough visible contacts
// is multi-threaded whatever is hidden; too few is single-threaded only when
// nothing is hidden.
func TestTheAccountVerdictFollowsThePublishedThreshold(t *testing.T) {
	for maxEngaged := range 3 {
		for visible := range 4 {
			for _, incomplete := range []bool{false, true} {
				var want ThreadingVerdict
				switch {
				case visible > maxEngaged:
					want = ThreadingMultiple
				case incomplete:
					want = ThreadingUnknown
				case visible == 0:
					want = ThreadingNoContacts
				default:
					want = ThreadingSingle
				}
				if got := threadingVerdict(visible, incomplete, maxEngaged); got != want {
					t.Errorf("%d visible, incomplete %v, threshold %d: %q, want %q", visible, incomplete, maxEngaged, got, want)
				}
			}
		}
	}
}
