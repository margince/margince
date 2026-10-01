// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// The stripper's rune windows used to be taken by converting the WHOLE page to
// []rune to keep its first 60 or 300 runes. Inside sharedOpening's
// pages × offsets × pages loop that was ~430,000 full-page conversions per
// round on a 60-page site, which is where a deep read's extraction step spent
// its time (160 s of CPU on congnghesovst.net with the model faked out) and
// most of the garbage a worker bursts to gigabytes on. The rewrite only reads
// the runes it keeps; these tests hold it to the old code's exact answers.

// chromeCorpusWords mixes scripts and widths on purpose: a window measured in
// runes and cut in bytes is only exercised by text whose runes are not one byte.
var chromeCorpusWords = []string{
	"Products", "Solutions", "Pricing", "About", "Contact", "Careers", "Blog",
	"Giải", "pháp", "công", "nghệ", "Liên", "hệ", "Über", "uns", "Größe",
	"製品", "会社", "概要", "🚀", "Überblick", "façade", "naïve", "a", "B2B",
}

// chromeCorpusBreakers are byte sequences a crawled page can carry that are not
// valid UTF-8 — a stray Latin-1 byte, a truncated euro sign, a lone
// continuation byte. The old code re-encoded them as U+FFFD inside its windows;
// the new code must too, or the two disagree on what a page "starts with".
var chromeCorpusBreakers = []string{"\xff", "\xe2\x82", "\x80", "\xc3"}

func chromeWords(r *rand.Rand, n int, breakers bool) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(chromeCorpusWords[r.Intn(len(chromeCorpusWords))])
		if breakers && r.Intn(40) == 0 {
			b.WriteString(chromeCorpusBreakers[r.Intn(len(chromeCorpusBreakers))])
		}
	}
	return b.String()
}

// chromeCorpus builds one site: some pages open with a per-page title and then
// one of a few shared menus (one per "language"), some carry no menu at all,
// some are near-duplicates, and bodies range from stub to long.
func chromeCorpus(r *rand.Rand, breakers bool) []crawlPage {
	menus := make([]string, 1+r.Intn(3))
	for i := range menus {
		menus[i] = chromeWords(r, 30+r.Intn(200), breakers)
	}
	pages := make([]crawlPage, 3+r.Intn(8))
	for i := range pages {
		var text strings.Builder
		if r.Intn(3) > 0 {
			text.WriteString(chromeWords(r, r.Intn(8), breakers) + " ")
		}
		if r.Intn(5) > 0 {
			text.WriteString(menus[r.Intn(len(menus))] + " ")
		}
		switch r.Intn(6) {
		case 0: // a stub
			text.WriteString(chromeWords(r, r.Intn(20), breakers))
		case 1: // a near-duplicate of an earlier page
			if i > 0 {
				text.WriteString(pages[r.Intn(i)].Text)
				continue
			}
			fallthrough
		default:
			text.WriteString(chromeWords(r, 50+r.Intn(250), breakers))
		}
		pages[i] = crawlPage{URL: fmt.Sprintf("https://example.test/%d", i), Text: text.String()}
	}
	return pages
}

// legacyPanicked runs f and reports whether it panicked. The old chromeStarts
// indexed past the end of a page whose first 300 runes held a broken byte —
// its window was measured on the re-encoded text, where each broken byte is
// three bytes — so on those corpora there is no old answer to match, only a
// crash the new code must not repeat.
func legacyPanicked(f func()) (panicked bool) {
	defer func() { panicked = recover() != nil }()
	f()
	return false
}

func TestTheChromeStripperAnswersExactlyAsItDidBefore(t *testing.T) {
	compared, crashed := 0, 0
	for seed := int64(0); seed < 160; seed++ {
		r := rand.New(rand.NewSource(seed))
		pages := chromeCorpus(r, seed%2 == 1)
		gotPages, gotBlocks := stripSharedPrefixBlocks(pages)
		var wantPages []crawlPage
		var wantBlocks []string
		if legacyPanicked(func() { wantPages, wantBlocks = legacyStripSharedPrefixBlocks(pages) }) {
			crashed++
			continue
		}
		compared++
		if !slices.Equal(gotBlocks, wantBlocks) {
			t.Fatalf("seed %d: blocks differ\n got %q\nwant %q", seed, gotBlocks, wantBlocks)
		}
		for i := range wantPages {
			if gotPages[i].Text != wantPages[i].Text {
				t.Fatalf("seed %d page %d: text differs\n got %q\nwant %q", seed, i, gotPages[i].Text, wantPages[i].Text)
			}
		}
	}
	// Guard the guard: a generator that only produced crashing corpora would
	// pass this test while comparing nothing.
	if compared < 120 {
		t.Fatalf("only %d of 160 corpora were comparable (%d crashed the old code)", compared, crashed)
	}
	t.Logf("%d corpora matched byte-for-byte; %d crashed the old code and not the new", compared, crashed)
}

// The helpers are held to the old code one level down as well, over text the
// corpus test reaches only by chance: windows cut inside a multi-byte rune,
// texts shorter than the window, and a window landing on a broken byte.
func TestTheRuneWindowsMatchTheOldConversions(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 5000; i++ {
		breakers := i%2 == 1
		head := chromeWords(r, r.Intn(120), breakers)
		other := chromeWords(r, r.Intn(10), breakers) + " " + head[:r.Intn(len(head)+1)] + chromeWords(r, r.Intn(80), breakers)
		if got, want := longestSharedRun(head, other), legacyLongestSharedRun(head, other); got != want {
			t.Fatalf("longestSharedRun(%q, %q) = %q, the old code said %q", head, other, got, want)
		}
		got := chromeStarts(other)
		var want []int
		if !legacyPanicked(func() { want = legacyChromeStarts(other) }) && !slices.Equal(got, want) {
			t.Fatalf("chromeStarts(%q) = %v, the old code said %v", other, got, want)
		}
		if got, want := commonPrefix(head, other), legacyCommonPrefix(head, other); got != want {
			t.Fatalf("commonPrefix(%q, %q) = %q, the old code said %q", head, other, got, want)
		}
		// The fast path blockFrom takes for a page already validated must
		// give the same answer as the path that re-validates.
		if utf8.ValidString(head) {
			if got, want := commonPrefixOf(head, other, true), legacyCommonPrefix(head, other); got != want {
				t.Fatalf("commonPrefixOf(%q, %q, valid) = %q, the old code said %q", head, other, got, want)
			}
			if got, want := longestSharedRunOf(head, other, true), legacyLongestSharedRun(head, other); got != want {
				t.Fatalf("longestSharedRunOf(%q, %q, valid) = %q, the old code said %q", head, other, got, want)
			}
		}
	}
}

func TestRunesAtLeastCountsWhatRuneCountInStringCounts(t *testing.T) {
	for _, s := range []string{"", "a", "ü", "製品", "\xff\xff", "a\xe2\x82b", strings.Repeat("🚀", 7), strings.Repeat("\x80", 9)} {
		for n := 0; n <= 10; n++ {
			if got, want := runesAtLeast(s, n), len([]rune(s)) >= n; got != want {
				t.Errorf("runesAtLeast(%q, %d) = %v, want %v", s, n, got, want)
			}
		}
	}
}

// siteChromeBenchmarkPages is a congnghesovst.net-sized site: 60 pages of
// ~14k runes, each opening with a title and the same long menu, in a script
// whose runes are wider than a byte.
func siteChromeBenchmarkPages() []crawlPage {
	r := rand.New(rand.NewSource(7))
	menu := chromeWords(r, 400, false)
	pages := make([]crawlPage, 60)
	for i := range pages {
		pages[i] = crawlPage{
			URL:  fmt.Sprintf("https://example.test/%d", i),
			Text: chromeWords(r, 4, false) + " " + menu + " " + chromeWords(r, 2500, false),
		}
	}
	return pages
}

func BenchmarkStripSharedPrefixBlocks(b *testing.B) {
	pages := siteChromeBenchmarkPages()
	b.Run("current", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			stripSharedPrefixBlocks(pages)
		}
	})
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			legacyStripSharedPrefixBlocks(pages)
		}
	})
}

// The crash itself, pinned: a page longer than the search window whose opening
// carries broken bytes. The old code panicked here; a deep read of such a site
// failed its job instead of reading it.
func TestAPageWithBrokenBytesInItsOpeningDoesNotCrashTheStripper(t *testing.T) {
	// 310 broken bytes are 310 runes but only 310 bytes, while the old window
	// of 300 re-encoded runes was 900 bytes long.
	text := strings.Repeat("\xff", 310) + " own words of this page"
	if !legacyPanicked(func() { legacyChromeStarts(text) }) {
		t.Fatal("expected the old chromeStarts to panic on this page; the fixture no longer pins the bug")
	}
	chromeStarts(text)
	pages := []crawlPage{{Text: text}, {Text: text + " a"}, {Text: text + " b"}, {Text: text + " c"}}
	stripSharedPrefixBlocks(pages)
}

func TestFirstDifferenceFindsTheFirstMismatchingByte(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 3000; i++ {
		a := []byte(chromeWords(r, r.Intn(300), true))
		b := slices.Clone(a)
		if len(b) > 0 && r.Intn(4) > 0 {
			b[r.Intn(len(b))] ^= byte(1 + r.Intn(255))
		}
		want := 0
		for want < len(a) && a[want] == b[want] {
			want++
		}
		if got := firstDifference(string(a), string(b)); got != want {
			t.Fatalf("firstDifference = %d, want %d (len %d)", got, want, len(a))
		}
	}
}
