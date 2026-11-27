// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"testing"
	"time"
)

func TestRankNudgeReadsPriorityFreshnessAndChangeFrequency(t *testing.T) {
	newest := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		entry SitemapEntry
		want  int
	}{
		{"no metadata shifts nothing", SitemapEntry{Priority: 0.5}, 0},
		{"priority 1.0 and a recent change", SitemapEntry{Priority: 1, LastMod: newest.AddDate(0, -1, 0)}, 5},
		{"priority 0 ranks lowest", SitemapEntry{Priority: 0}, -3},
		{"untouched for over two years before the newest page", SitemapEntry{Priority: 0.5, LastMod: newest.AddDate(-4, 0, 0)}, -2},
		{"a page between one and two years old is neutral", SitemapEntry{Priority: 0.5, LastMod: newest.AddDate(-1, -6, 0)}, 0},
		{"daily change marks a listing or feed", SitemapEntry{Priority: 0.5, ChangeFreq: "daily"}, -2},
		{"monthly change is no signal", SitemapEntry{Priority: 0.5, ChangeFreq: "monthly"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.entry.RankNudge(newest); got != tc.want {
				t.Fatalf("RankNudge = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNewestLastModIsTheLatestEntryAndZeroWithoutAny(t *testing.T) {
	older, newer := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := NewestLastMod([]SitemapEntry{{LastMod: older}, {}, {LastMod: newer}}); !got.Equal(newer) {
		t.Fatalf("NewestLastMod = %v, want %v", got, newer)
	}
	if got := NewestLastMod([]SitemapEntry{{Loc: "https://acme.example/"}}); !got.IsZero() {
		t.Fatalf("NewestLastMod without lastmod = %v, want zero", got)
	}
}
