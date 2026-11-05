// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package editdistance_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/editdistance"
)

func TestLevenshteinCountsSingleEdits(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"workspce", "workspace", 1},
		{"dispaly_name", "display_name", 2},
		{"LOG_LVL", "LOG_LEVEL", 2},
	} {
		if got := editdistance.Levenshtein([]rune(c.a), []rune(c.b)); got != c.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLevenshteinWorksOnWholeTokens(t *testing.T) {
	got := editdistance.Levenshtein([]string{"send", "the", "quote"}, []string{"send", "a", "quote", "today"})
	if got != 2 {
		t.Fatalf("token distance = %d, want 2 (one substitution, one insertion)", got)
	}
}

func TestNearestOffersOnlyACloseName(t *testing.T) {
	names := []string{"log.level", "log.format", "workspace"}
	if got, ok := editdistance.Nearest("log.lvl", names, 2); !ok || got != "log.level" {
		t.Errorf("Nearest(log.lvl) = %q, %v; want log.level", got, ok)
	}
	if got, ok := editdistance.Nearest("redis", names, 2); ok {
		t.Errorf("Nearest(redis) = %q; nothing is within two edits", got)
	}
	if got, ok := editdistance.Nearest("workspace", names, 2); ok {
		t.Errorf("Nearest offered %q for a name that is itself a candidate", got)
	}
}

func TestNearestBreaksATieTheSameWay(t *testing.T) {
	for _, candidates := range [][]string{{"ac", "aa"}, {"aa", "ac"}} {
		if got, _ := editdistance.Nearest("ab", candidates, 2); got != "aa" {
			t.Fatalf("Nearest(ab, %v) = %q, want the first in sorted order, aa", candidates, got)
		}
	}
}
