// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Specs for which seat a former member's address already lives on. The seed
// can hold a seat per domain spelling, and the spelling the caller typed
// decides between them.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	typedPunycode = "ana@xn--bcher-kva.example"
	otherUnicode  = "ana@bücher.example"
)

func TestTheTypedSpellingsMergedSeatAnswersItsOwnRoot(t *testing.T) {
	typedRoot, otherRoot := ids.New[ids.UserKind](), ids.New[ids.UserKind]()
	roots := []lineageRoot{
		{ID: otherRoot, ReachedFrom: otherUnicode, FromMerged: true, Live: true},
		{ID: typedRoot, ReachedFrom: "Ana@xn--bcher-kva.example", FromMerged: true, Live: true},
	}
	got, found, err := survivorFor(roots, typedPunycode)
	if err != nil || !found || got != typedRoot {
		t.Errorf("survivor = %v, %t, %v; want the typed seat's own root, whatever its case", got, found, err)
	}
}

func TestAnUnmergedTypedSeatIsNotASurvivor(t *testing.T) {
	roots := []lineageRoot{
		{ID: ids.New[ids.UserKind](), ReachedFrom: otherUnicode, FromMerged: true, Live: true},
		{ID: ids.New[ids.UserKind](), ReachedFrom: typedPunycode, FromMerged: false, Live: true},
	}
	if _, found, err := survivorFor(roots, typedPunycode); found || err != nil {
		t.Errorf("an unmerged typed seat answered found=%t, %v; want the caller to provision as usual", found, err)
	}
}

func TestATypedSeatWhoseRootIsGoneIsARemovedAddress(t *testing.T) {
	roots := []lineageRoot{
		{ID: ids.New[ids.UserKind](), ReachedFrom: otherUnicode, FromMerged: true, Live: true},
		{ID: ids.New[ids.UserKind](), ReachedFrom: typedPunycode, FromMerged: true, Live: false},
	}
	if _, _, err := survivorFor(roots, typedPunycode); !errors.Is(err, errRemovedEmail) {
		t.Errorf("a removed typed root answered %v, want the removed address, not the other spelling's survivor", err)
	}
}

func TestAnAddressNoSeatSpellsAsTypedFallsBackToAMergedRoot(t *testing.T) {
	live := ids.New[ids.UserKind]()
	roots := []lineageRoot{
		{ID: ids.New[ids.UserKind](), ReachedFrom: otherUnicode, FromMerged: false, Live: true},
		{ID: live, ReachedFrom: otherUnicode, FromMerged: true, Live: true},
	}
	if got, found, err := survivorFor(roots, typedPunycode); err != nil || !found || got != live {
		t.Errorf("survivor = %v, %t, %v; want the merged seat's live root", got, found, err)
	}
	gone := []lineageRoot{{ID: ids.New[ids.UserKind](), ReachedFrom: otherUnicode, FromMerged: true, Live: false}}
	if _, _, err := survivorFor(gone, typedPunycode); !errors.Is(err, errRemovedEmail) {
		t.Errorf("a merged lineage with no live root answered %v, want the removed address", err)
	}
	if _, found, err := survivorFor(nil, typedPunycode); found || err != nil {
		t.Errorf("no matched seat answered found=%t, %v; want nothing found", found, err)
	}
}

// Two logins name one mailbox when the local part matches and the domain is
// one in either spelling, whatever the case.
func TestTwoSpellingsOfOneLoginAreOneMailbox(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{otherUnicode, typedPunycode, true},
		{"Ana@XN--BCHER-KVA.example", otherUnicode, true},
		{"bea@xn--bcher-kva.example", otherUnicode, false},
		{"ana@other.example", otherUnicode, false},
		{"not-an-address", "not-an-address", false},
	} {
		if got := sameMailbox(c.a, c.b); got != c.same {
			t.Errorf("sameMailbox(%q, %q) = %t, want %t", c.a, c.b, got, c.same)
		}
	}
}
