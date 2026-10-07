// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import "testing"

func TestReindexIsRequiredWhenTheIdentityMovedOrABacklogRemains(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		configured, populated string
		pending               int
		want                  bool
	}{
		{"current store, nothing pending", "m@1", "m@1", 0, false},
		{"current store, a backlog", "m@1", "m@1", 3, true},
		{"identity moved, nothing pending", "m@2", "m@1", 0, true},
		{"identity moved and a backlog", "m@2", "m@1", 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ReindexRequired(tc.configured, tc.populated, tc.pending); got != tc.want {
				t.Errorf("ReindexRequired(%q, %q, %d) = %v, want %v", tc.configured, tc.populated, tc.pending, got, tc.want)
			}
		})
	}
}
