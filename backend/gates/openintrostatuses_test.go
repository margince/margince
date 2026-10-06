// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/privacy"
)

// TestTheErasureClosesExactlyTheOpenIntroductions fails in both directions when the
// erasure's idea of an open ask stops matching the lifecycle's.
//
// The erasure closes an introduction naming an erased contact, because Decide and
// Cancel both write a fresh decision_reason and an ask left open is one whose prose can
// be rewritten after the scrub cleared it. Which statuses those are belongs to
// introductions.Open, and privacy may not import a sibling module — so it carries a
// declared mirror, and this is what makes the mirror fail rather than drift.
//
// Fewer in the erasure means an ask stays actionable and its prose returns. More means
// the erasure rewrites a completed outcome — introduced, name_dropped and replied all
// leave closed_at NULL — into "cancelled", destroying the record of what happened.
func TestTheErasureClosesExactlyTheOpenIntroductions(t *testing.T) {
	t.Parallel()
	var open []string
	for _, status := range introductions.EveryStatus() {
		if introductions.Open(status) {
			open = append(open, string(status))
		}
	}
	closed := privacy.OpenIntroStatuses()
	sort.Strings(open)
	sort.Strings(closed)

	if len(open) == 0 {
		t.Fatal("no status reads as open, so this comparison proves nothing: the lifecycle's own " +
			"census has stopped reaching introductions.Open")
	}
	if !equalStrings(open, closed) {
		t.Errorf("the lifecycle calls %v open and the erasure closes %v.\n\n"+
			"Fewer in the erasure leaves an ask actionable about an erased contact, and Cancel "+
			"writes a decision_reason from any open state — so the prose this scrub cleared comes "+
			"back. More rewrites a completed outcome into cancelled: introduced, name_dropped and "+
			"replied all leave closed_at NULL, so they look open to anything asking that column.",
			open, closed)
	}
}

// equalStrings compares two sorted lists, which is the whole comparison this gate is.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
