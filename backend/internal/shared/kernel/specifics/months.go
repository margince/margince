// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

// The month words this product's readers are written to in.
//
// English and German only: Vietnamese writes its months as "tháng 6", which the
// numeric form in dates.go reads directly, so there is no vocabulary to keep
// for it. A word list that pretended otherwise would be a list nothing consults.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// months maps every accepted spelling to its number. Abbreviations are the
// three-letter forms a model actually writes; the German full names differ from
// the English in four of twelve, which is exactly why this is a table rather
// than a prefix rule.
var months = map[string]int{
	"january": 1, "jan": 1, "januar": 1,
	"february": 2, "feb": 2, "februar": 2,
	"march": 3, "mar": 3, "märz": 3, "maerz": 3,
	"april": 4, "apr": 4,
	"may": 5, "mai": 5,
	"june": 6, "jun": 6, "juni": 6,
	"july": 7, "jul": 7, "juli": 7,
	"august": 8, "aug": 8,
	"september": 9, "sep": 9, "sept": 9,
	"october": 10, "oct": 10, "oktober": 10, "okt": 10,
	"november": 11, "nov": 11,
	"december": 12, "dec": 12, "dezember": 12, "dez": 12,
}

// monthAlternatives is the alternation the date patterns embed, longest first
// so "juni" is not matched as "jun" with a stray "i" left behind.
var monthAlternatives = alternation(months)

func alternation(vocabulary map[string]int) string {
	words := make([]string, 0, len(vocabulary))
	for word := range vocabulary {
		words = append(words, word)
	}
	sort.Slice(words, func(i, j int) bool {
		if len(words[i]) != len(words[j]) {
			return len(words[i]) > len(words[j])
		}
		return words[i] < words[j]
	})
	for i, word := range words {
		words[i] = regexp.QuoteMeta(word)
	}
	return strings.Join(words, "|")
}

// monthNumber is a month word as its number, as a string the date readers
// parse like any other — empty when the word is not one, which their own guard
// then refuses.
func monthNumber(word string) string {
	number, ok := months[strings.ToLower(word)]
	if !ok {
		return ""
	}
	return strconv.Itoa(number)
}
