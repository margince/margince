// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A merge lineage can end at one live seat per domain spelling. The answer is
// the root the typed spelling's own seat leads to, whatever its case.
func TestTheTypedSpellingChoosesItsOwnRoot(t *testing.T) {
	unicode, punycode := ids.New[ids.UserKind](), ids.New[ids.UserKind]()
	roots := []lineageRoot{
		{ID: unicode, ReachedFrom: "ana@bücher.example"},
		{ID: punycode, ReachedFrom: "Ana@xn--bcher-kva.example"},
	}
	if got, ok := rootReachedFrom(roots, "ana@xn--bcher-kva.example"); !ok || got != punycode {
		t.Errorf("the punycode spelling answered %v, want the root its own seat leads to", got)
	}
	if got, ok := rootReachedFrom(roots, "ana@bücher.example"); !ok || got != unicode {
		t.Errorf("the unicode spelling answered %v, want the root its own seat leads to", got)
	}
}

// A walk that reached no live seat is a removed login. One whose typed
// spelling owns no seat answers the first root in id order.
func TestALineageWithoutItsTypedSeatStillAnswersOneRoot(t *testing.T) {
	if _, ok := rootReachedFrom(nil, "ana@example.com"); ok {
		t.Error("a lineage with no live root answered a seat")
	}
	first := ids.New[ids.UserKind]()
	roots := []lineageRoot{{ID: first, ReachedFrom: "ana@one.example"}, {ID: ids.New[ids.UserKind](), ReachedFrom: "ana@two.example"}}
	if got, ok := rootReachedFrom(roots, "ana@three.example"); !ok || got != first {
		t.Errorf("an unmatched spelling answered %v, want the first root", got)
	}
}
