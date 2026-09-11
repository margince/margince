// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Which names removal may resolve, against the real store.
//
// Retiring a word takes it out of the vocabulary and leaves it on every record
// already carrying it, and the record read hands those assignments back —
// flagged, sorted after the live ones. So removal is the one verb that must
// reach a retired name: a live-only lookup answers "the tagging is already
// absent" about a row that is still there, and nothing else can take it off,
// because retiring the word is what put it in that state.
//
// The store learned this first. The by-NAME door did not, and that is the door
// an assistant uses, because an assistant works from words.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRemovalResolvesARetiredNameAndApplyingStillDoesNot(t *testing.T) {
	e := integration.Setup(t)
	retired := seedTag(t, e, "Retired Conference", true)
	live := seedTag(t, e, "Live Conference", false)
	seam := tagSeam(e.Pool)
	ctx := e.Admin()

	for _, tc := range []struct {
		name   string
		lookup func(context.Context, string) (ids.UUID, bool, error)
		word   string
		want   bool
		why    string
	}{
		{
			"removal reaches the retired word", seam.FindTagToRemove, "Retired Conference", true,
			"a record still carries it and nothing else can take it off",
		},
		{"removal reaches a live word", seam.FindTagToRemove, "Live Conference", true, ""},
		{
			"removal invents nothing", seam.FindTagToRemove, "Never Coined", false,
			"a name nobody coined means the tagging really is absent",
		},
		{
			"applying refuses the retired word", seam.FindTag, "Retired Conference", false,
			"putting a retired word back on a record coins it again",
		},
		{"applying reaches a live word", seam.FindTag, "Live Conference", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, ok, err := tc.lookup(ctx, tc.word)
			if err != nil {
				t.Fatalf("looking up %q: %v", tc.word, err)
			}
			if ok != tc.want {
				t.Fatalf("resolving %q answered ok=%v, want %v — %s", tc.word, ok, tc.want, tc.why)
			}
			if !ok {
				return
			}
			want := retired
			if tc.word == "Live Conference" {
				want = live
			}
			if id != want {
				t.Errorf("resolving %q answered %s, want %s", tc.word, id, want)
			}
		})
	}
}
