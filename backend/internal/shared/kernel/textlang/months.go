// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textlang

import (
	"sort"
	"time"
)

// monthWords are the English and German month names and the abbreviations mail
// writes. Vietnamese names a month by its number ("tháng 11"), so it needs none.
var monthWords = map[string]time.Month{
	"january": 1, "januar": 1, "february": 2, "feb": 2, "februar": 2, "march": 3, "mar": 3, "märz": 3,
	"april": 4, "apr": 4, "mai": 5, "june": 6, "jun": 6, "juni": 6, "july": 7, "jul": 7, "juli": 7,
	"august": 8, "aug": 8, "september": 9, "sep": 9, "sept": 9, "october": 10, "oct": 10, "oktober": 10, "okt": 10,
	"november": 11, "nov": 11, "december": 12, "dec": 12, "dezember": 12, "dez": 12,
}

// nameLikeMonthWords are month words that are also a verb ("may") or a first
// name ("Jan"); a pattern that scans prose for a month leaves them out.
var nameLikeMonthWords = map[string]time.Month{"may": 5, "jan": 1}

// MonthWord is the month a lower-cased word names, "may" and "jan" included.
func MonthWord(word string) (time.Month, bool) {
	if month, ok := monthWords[word]; ok {
		return month, true
	}
	month, ok := nameLikeMonthWords[word]
	return month, ok
}

// MonthWords lists the month words that name nothing else, longest first so an
// alternation built from them reads "june" before "jun".
func MonthWords() []string {
	words := make([]string, 0, len(monthWords))
	for word := range monthWords {
		words = append(words, word)
	}
	sort.Slice(words, func(i, j int) bool {
		if len(words[i]) != len(words[j]) {
			return len(words[i]) > len(words[j])
		}
		return words[i] < words[j]
	})
	return words
}
