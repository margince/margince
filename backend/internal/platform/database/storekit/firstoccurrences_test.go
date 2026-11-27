// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"slices"
	"testing"
)

func TestFirstOccurrencesKeepsOrderAndDropsRepeats(t *testing.T) {
	for _, c := range []struct {
		name     string
		in, want []string
	}{
		{name: "a repeat keeps its first place", in: []string{"a", "b", "a", "c", "b"}, want: []string{"a", "b", "c"}},
		{name: "no repeat is unchanged", in: []string{"c", "a", "b"}, want: []string{"c", "a", "b"}},
		{name: "nothing is an empty list, not nil", in: nil, want: []string{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := FirstOccurrences(c.in)
			if !slices.Equal(got, c.want) || got == nil {
				t.Fatalf("FirstOccurrences(%v) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}
