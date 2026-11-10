// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptref_test

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

const (
	firstID  = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e07"
	secondID = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e08"
)

func TestRefsAreMintedInTheOrderTheRecordsAreShown(t *testing.T) {
	t.Parallel()
	refs := promptref.New("m", firstID, secondID)
	if got := refs.All(); !slices.Equal(got, []string{"m1", "m2"}) {
		t.Fatalf("All() = %v, want [m1 m2]", got)
	}
	if got := refs.For(secondID); got != "m2" {
		t.Errorf("a record shown twice keeps its first ref: For = %q, want m2", got)
	}
	if got := refs.For("a third record"); got != "m3" {
		t.Errorf("a new record takes the next ref: For = %q, want m3", got)
	}
}

func TestARefReadsBackToTheRecordItNames(t *testing.T) {
	t.Parallel()
	refs := promptref.New("d", firstID)
	if id, ok := refs.ID("d1"); !ok || id != firstID {
		t.Errorf("ID(d1) = %q, %v; want %q, true", id, ok, firstID)
	}
}

func TestRefOfLooksUpWithoutMinting(t *testing.T) {
	t.Parallel()
	refs := promptref.New("m", firstID)
	if ref, ok := refs.RefOf(firstID); !ok || ref != "m1" {
		t.Errorf("RefOf(first) = %q, %v; want m1, true", ref, ok)
	}
	if ref, ok := refs.RefOf(secondID); ok {
		t.Errorf("RefOf(unshown) = %q, true; want false", ref)
	}
	if got := refs.All(); !slices.Equal(got, []string{"m1"}) {
		t.Errorf("RefOf minted a ref: All() = %v", got)
	}
}

func TestARefTheTableNeverMintedIsRefused(t *testing.T) {
	t.Parallel()
	refs := promptref.New("d", firstID)
	for _, foreign := range []string{"d2", "m1", firstID, ""} {
		if id, ok := refs.ID(foreign); ok {
			t.Errorf("ID(%q) = %q, true; a ref this request never minted must be refused", foreign, id)
		}
	}
}

func TestAllIsACopyTheCallerCannotEdit(t *testing.T) {
	t.Parallel()
	refs := promptref.New("m", firstID)
	refs.All()[0] = "x9"
	if got := refs.All(); !slices.Equal(got, []string{"m1"}) {
		t.Errorf("All() = %v after editing a returned slice, want [m1]", got)
	}
}

func TestMintGivesEachIDARefOfItsKind(t *testing.T) {
	t.Parallel()
	const third = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e09"
	payload := `{"deal":"` + firstID + `","acts":["` + secondID + `","` + firstID + `"],"other":"` + third + `"}`
	tables := promptref.Mint(payload,
		map[string]string{firstID: "deal", secondID: "activity"},
		map[string]string{"deal": "d", "activity": "a", "": "r"})
	if len(tables) != 3 {
		t.Fatalf("Mint made %d tables, want one each for deal, activity and the unnamed kind", len(tables))
	}
	shown := promptref.Rewrite(payload, tables["deal"], tables["activity"], tables[""])
	if want := `{"deal":"d1","acts":["a1","d1"],"other":"r1"}`; shown != want {
		t.Fatalf("Rewrite over the minted tables = %s, want %s", shown, want)
	}
	if id, ok := tables["deal"].ID("d1"); !ok || id != firstID {
		t.Errorf("deal ID(d1) = %q, %v; want the deal", id, ok)
	}
	if id, ok := tables["activity"].ID("d1"); ok {
		t.Errorf("activity ID(d1) = %q; a ref minted for another kind must be refused", id)
	}
}

func TestMintUsesAKindsOwnNameWhenItHasNoPrefix(t *testing.T) {
	t.Parallel()
	tables := promptref.Mint(firstID, map[string]string{firstID: "lead"}, nil)
	if got := tables["lead"].All(); !slices.Equal(got, []string{"lead1"}) {
		t.Errorf("refs = %v, want [lead1]", got)
	}
}
