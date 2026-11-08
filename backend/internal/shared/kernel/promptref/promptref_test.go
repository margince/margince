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

func TestAPayloadShowsEachIDAsARefOfItsKind(t *testing.T) {
	t.Parallel()
	const third = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e09"
	payload := promptref.Rewrite(
		`{"deal":"`+firstID+`","acts":["`+secondID+`","`+firstID+`"],"other":"`+third+`"}`,
		map[string]string{firstID: "deal", secondID: "activity"},
		map[string]string{"deal": "d", "activity": "a", "": "r"})
	if want := `{"deal":"d1","acts":["a1","d1"],"other":"r1"}`; payload.Text() != want {
		t.Fatalf("Text() = %s, want %s", payload.Text(), want)
	}
	if id, ok := payload.ID("deal", "d1"); !ok || id != firstID {
		t.Errorf("ID(deal, d1) = %q, %v; want the deal", id, ok)
	}
	for _, foreign := range [][2]string{{"activity", "d1"}, {"deal", "d2"}, {"deal", firstID}} {
		if id, ok := payload.ID(foreign[0], foreign[1]); ok {
			t.Errorf("ID(%s, %s) = %q; a ref the payload never showed as that kind must be refused", foreign[0], foreign[1], id)
		}
	}
	if !payload.Names("the deal d1 stalled") || payload.Names("a C1 speaker, d2 and a10") {
		t.Error("Names must find exactly the refs this payload showed")
	}
	if got := payload.Refer(`{"entity_id":"` + secondID + `"}`); got != `{"entity_id":"a1"}` {
		t.Errorf("Refer = %s, want the activity's ref", got)
	}
}
