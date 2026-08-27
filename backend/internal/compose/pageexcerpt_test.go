// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How much of a crawled page one fact call reads, and what it says about the
// rest.
//
// The lane used to index the page's whole stripped text, so the prompt was as
// long as whoever published the page chose to make it — up to webread's 1 MiB
// fetch cap. On a metered provider that is a stranger's decision about our
// tokens; on a local one it sizes the context window the adapter must
// allocate.

import (
	"strings"
	"testing"
)

func TestAPageIsReadOnlyAsFarAsItsBudget(t *testing.T) {
	long := strings.Repeat("ä", pageFactsExcerptRunes*3)

	excerpt, unread := pageFactsExcerpt(crawlPage{URL: "https://example.test/x", Text: long})

	if len(excerpt) != 1 {
		t.Fatalf("the excerpt holds %d pages, want the one it was given", len(excerpt))
	}
	// Runes, not bytes. The text is multi-byte on purpose: a byte slice would
	// cut mid-character here and the budget would mean something different for
	// a German page than for an English one.
	if got := len([]rune(excerpt[0].Text)); got != pageFactsExcerptRunes {
		t.Errorf("the excerpt kept %d runes, want %d", got, pageFactsExcerptRunes)
	}
	if want := pageFactsExcerptRunes * 2; unread != want {
		t.Errorf("it reported %d runes unread, want %d — the cap alone cannot say what was lost, "+
			"because a page one rune over and one a hundred times over are read identically", unread, want)
	}
}

// A page inside the budget must not be reported as truncated: a warning that
// fires on every ordinary page is one an operator learns to ignore, and then
// the real one is invisible too.
func TestAPageInsideTheBudgetIsReadWholeAndSaysSo(t *testing.T) {
	whole := strings.Repeat("a", pageFactsExcerptRunes)

	excerpt, unread := pageFactsExcerpt(crawlPage{URL: "https://example.test/x", Text: whole})

	if excerpt[0].Text != whole {
		t.Errorf("a page exactly at the budget was altered: kept %d of %d runes",
			len([]rune(excerpt[0].Text)), len([]rune(whole)))
	}
	if unread != 0 {
		t.Errorf("a page read in full reported %d runes unread", unread)
	}
}
