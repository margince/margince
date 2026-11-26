// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// A repair that core's own rule makes take a link back is shown, as a move is,
// though no hook touched the answer.
func TestACoreRetractionIsRecordedAsCoresFiling(t *testing.T) {
	got := filingOf(DealVerdict{Outcome: DealNone}, filingRetracted)
	if got == nil || got.Actor != FilingActorCore || got.Result != filingRetracted {
		t.Fatalf("filing = %+v, want core's retraction", got)
	}
	if got := filingOf(DealVerdict{Outcome: DealNone}, ""); got != nil {
		t.Errorf("an unlinked mail no one changed shows a filing: %+v", got)
	}
}
