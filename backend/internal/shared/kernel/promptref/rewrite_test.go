// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptref_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

func TestRewriteNamesEveryIDByItsRefLongestFirst(t *testing.T) {
	t.Parallel()
	sources := promptref.New("s", "activity:"+firstID)
	products := promptref.New("p", firstID, secondID)
	got := promptref.Rewrite(`{"source":"activity:`+firstID+`","product":"`+firstID+`","other":"`+secondID+`"}`,
		products, sources)
	if want := `{"source":"s1","product":"p1","other":"p2"}`; got != want {
		t.Errorf("Rewrite = %s, want %s", got, want)
	}
}

func TestNamedInFindsARefOnlyAsAWordOfItsOwn(t *testing.T) {
	t.Parallel()
	refs := promptref.New("m", firstID)
	for _, text := range []string{"see m1.", "m1", "(m1)", "per m10 and m1"} {
		if ref, ok := refs.NamedIn(text); !ok || ref != "m1" {
			t.Errorf("NamedIn(%q) = %q, %v; want m1, true", text, ref, ok)
		}
	}
	for _, text := range []string{"m10", "xm1", "the m-series", ""} {
		if ref, ok := refs.NamedIn(text); ok {
			t.Errorf("NamedIn(%q) = %q, true; want none", text, ref)
		}
	}
}
