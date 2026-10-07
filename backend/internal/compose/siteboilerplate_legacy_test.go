// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"unicode/utf8"
)

// The shared-chrome stripper exactly as it stood before its rune windows were
// taken without converting whole pages (siteboilerplate.go). Frozen here, and
// only here, so siteboilerplate_equivalence_test.go can hold the rewrite to
// byte-for-byte the same answers: the change is a cost change, and a test that
// compared against expectations rather than against the old code would let a
// behaviour change through under the name of a speed-up.
//
// Do not "fix" anything in this file. Its value is that it is wrong in exactly
// the ways the old code was.

func legacyStripSharedPrefixBlocks(pages []crawlPage) ([]crawlPage, []string) {
	out := make([]crawlPage, len(pages))
	copy(out, pages)
	if len(pages) < boilerplateMinPages {
		return out, nil
	}
	total := 0
	for _, page := range pages {
		total += len(page.Text)
	}
	if total > boilerplateMaxCorpusBytes {
		return out, nil
	}

	var blocks []string
	for round := 0; round < boilerplateMaxRounds; round++ {
		block := legacyStripOneSharedBlock(out)
		if block == "" {
			break
		}
		blocks = append(blocks, block)
	}
	return out, blocks
}

func legacyStripOneSharedBlock(out []crawlPage) string {
	prefix := legacySharedOpening(out)
	if utf8.RuneCountInString(prefix) < boilerplateMinRunes {
		return ""
	}

	cut := false
	for i, page := range out {
		at := strings.Index(page.Text, prefix)
		if at < 0 {
			continue
		}
		if utf8.RuneCountInString(page.Text[:at]) > chromeSearchRunes {
			continue
		}
		trimmed := strings.TrimSpace(page.Text[at+len(prefix):])
		if trimmed == "" {
			continue
		}
		out[i].Text = trimmed
		cut = true
	}
	if !cut {
		return ""
	}
	return prefix
}

func legacySharedOpening(pages []crawlPage) string {
	best := ""
	for i, candidate := range pages {
		if utf8.RuneCountInString(candidate.Text) < boilerplateMinRunes {
			continue
		}
		for _, start := range legacyChromeStarts(candidate.Text) {
			block := legacyBlockFrom(candidate, i, start, pages)
			if utf8.RuneCountInString(block) > utf8.RuneCountInString(best) {
				best = block
			}
		}
	}
	return best
}

func legacyBlockFrom(candidate crawlPage, self, start int, pages []crawlPage) string {
	common := candidate.Text[start:]
	agreeing := 0
	for j, other := range pages {
		if j == self {
			continue
		}
		shared := legacyLongestSharedRun(common, other.Text)
		if utf8.RuneCountInString(shared) < boilerplateMinRunes {
			continue
		}
		common = shared
		agreeing++
	}
	if agreeing == 0 {
		return ""
	}
	if float64(agreeing+1)/float64(len(pages)) < boilerplateMinShare {
		return ""
	}
	if legacyTooLargeAShare(common, pages) || legacyBlockDominatesPages(common, pages) {
		return ""
	}
	return common
}

func legacyChromeStarts(text string) []int {
	starts := []int{0}
	limit := len(text)
	if runes := []rune(text); len(runes) > chromeSearchRunes {
		limit = len(string(runes[:chromeSearchRunes]))
	}
	for i := 1; i < limit; i++ {
		if text[i-1] == ' ' && text[i] != ' ' {
			starts = append(starts, i)
			if len(starts) >= chromeMaxStarts {
				return starts
			}
		}
	}
	return starts
}

func legacyLongestSharedRun(head, other string) string {
	anchor := head
	if runes := []rune(anchor); len(runes) > chromeAnchorRunes {
		anchor = string(runes[:chromeAnchorRunes])
	}
	if strings.TrimSpace(anchor) == "" {
		return ""
	}
	limit := len(other)
	if runes := []rune(other); len(runes) > chromeSearchRunes {
		limit = len(string(runes[:chromeSearchRunes]))
	}
	at := strings.Index(other[:min(limit+len(anchor), len(other))], anchor)
	if at < 0 {
		return ""
	}
	shared := legacyCommonPrefix(head, other[at:])
	if utf8.RuneCountInString(shared) < boilerplateMinRunes {
		return ""
	}
	return shared
}

func legacyBlockDominatesPages(block string, pages []crawlPage) bool {
	dominated, carrying := 0, 0
	for _, page := range pages {
		at := strings.Index(page.Text, block)
		if at < 0 {
			continue
		}
		carrying++
		remainder := strings.TrimSpace(page.Text[at+len(block):])
		if utf8.RuneCountInString(remainder) < boilerplateMinSurvivingRunes {
			dominated++
		}
	}
	if carrying == 0 {
		return false
	}
	return float64(dominated)/float64(carrying) > 0.5
}

func legacyTooLargeAShare(prefix string, pages []crawlPage) bool {
	carrying, stubs := 0, 0
	for _, page := range pages {
		at := strings.Index(page.Text, prefix)
		if at < 0 {
			continue
		}
		carrying++
		remainder := strings.TrimSpace(page.Text[at+len(prefix):])
		if utf8.RuneCountInString(remainder) < boilerplateMinRemainderRunes {
			stubs++
		}
	}
	if carrying == 0 {
		return false
	}
	return float64(stubs)/float64(carrying) > boilerplateMaxStubShare
}

func legacyCommonPrefix(a, b string) string {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	if limit > boilerplateMaxBlockBytes {
		limit = boilerplateMaxBlockBytes
	}
	end := 0
	for end < limit && a[end] == b[end] {
		end++
	}
	shared := a[:end]
	if !utf8.ValidString(shared) {
		for len(shared) > 0 && !utf8.ValidString(shared) {
			shared = shared[:len(shared)-1]
		}
	}
	if space := strings.LastIndexAny(shared, " \t\n"); space > 0 {
		shared = shared[:space]
	}
	return shared
}
