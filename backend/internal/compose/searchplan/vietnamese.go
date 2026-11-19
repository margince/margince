// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"slices"
	"strings"
	"unicode"
)

// vietnameseCounts are the phrases that ask "how many". "mấy" also means "a
// few" ("mấy tháng gần đây"), so it asks a count only before a counted noun.
// The search box reads the same phrases (frontend/src/lib/question.ts).
var vietnameseCounts = [][]string{
	{"bao nhiêu"},
	{"bao nhieu"},
	{"mấy deal"},
	{"mấy khách hàng", "mấy khach hang"},
	{"mấy công ty", "mấy cong ty"},
	{"mấy lead"},
	{"mấy dự án", "mấy du an"},
	{"mấy hợp đồng", "mấy hop dong"},
}

// Unmarked "thang" is also "tháng" (month), which is "thang nay", "thang
// truoc" or "thang sau".
var vietnameseMonthWords = []string{"nay", "truoc", "sau"}

func vietnameseWords(question string) []string {
	return strings.FieldsFunc(strings.ToLower(question), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r)
	})
}

func holdsPhrase(words []string, phrase string) bool {
	parts := strings.Fields(phrase)
	for at := 0; at+len(parts) <= len(words); at++ {
		if slices.Equal(words[at:at+len(parts)], parts) {
			return true
		}
	}
	return false
}

func asksVietnameseCount(words []string) bool {
	for _, spellings := range vietnameseCounts {
		if slices.ContainsFunc(spellings, func(phrase string) bool { return holdsPhrase(words, phrase) }) {
			return true
		}
	}
	return false
}

func saysVietnameseWon(words []string) bool {
	for at, word := range words {
		if word == "thắng" || (word == "thang" && (at+1 == len(words) || !slices.Contains(vietnameseMonthWords, words[at+1]))) {
			return true
		}
	}
	return false
}

// AsksVietnameseWonCount reports whether phrase alone carries a Vietnamese
// count or won word the won-count admission reads.
func AsksVietnameseWonCount(phrase string) bool {
	words := vietnameseWords(phrase)
	return asksVietnameseCount(words) || saysVietnameseWon(words)
}
