// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var (
	sentenceEnd = regexp.MustCompile(`[.!?]+(?:\s+|$)`)
	initial     = regexp.MustCompile(`(^|[\s(])\p{L}\.`)
	// The month of "tháng 11", of "20.11." or "20/11", and of "2026-11-20".
	monthPosition = regexp.MustCompile(`tháng\s+(\d{1,2})\b|\b\d{1,2}[./](\d{1,2})\b|\b\d{4}-(\d{2})-\d{2}\b`)
	// "may" and "jan" name a month only beside a day: "3 May", "May 3".
	datedNameLike = regexp.MustCompile(`\d\.?\s+(may|jan)\b|\b(may|jan)\.?\s+\d`)
	// proseMonths are the month words that name nothing else.
	proseMonths = func() map[string]bool {
		words := map[string]bool{}
		for _, word := range textlang.MonthWords() {
			words[word] = true
		}
		return words
	}()
)

// CheckSection refuses a rewrite of one section that lost a number, an email
// address, a link or an open question. It also refuses one that grew or shrank
// past a rewording.
func CheckSection(source, rendered string) error {
	source, rendered = strings.TrimSpace(source), strings.TrimSpace(rendered)
	if (source == "") != (rendered == "") {
		return errors.New("keep each section present or absent as in the original")
	}
	lost := slices.DeleteFunc(missingSpans(digitRun, source, rendered), func(run string) bool {
		return spelledAsMonth(run, source, rendered)
	})
	lost = append(lost, missingSpans(emailRun, source, rendered)...)
	if lost = append(lost, missingSpans(linkRun, source, rendered)...); len(lost) > 0 {
		return fmt.Errorf("the rendering lost %s; keep every number, email address and link exactly",
			strings.Join(lost, ", "))
	}
	// Native phrasing varies across languages; allow threefold expansion or
	// contraction and a short greeting's fixed overhead, but reject summaries.
	sourceLength, renderedLength := utf8.RuneCountInString(source), utf8.RuneCountInString(rendered)
	if sourceLength > 3*renderedLength+12 || renderedLength > 3*sourceLength+12 {
		return errors.New("the length differs too much; render the whole section without adding text")
	}
	sourceSentences, renderedSentences := sentenceCount(source), sentenceCount(rendered)
	if sourceSentences > 2*renderedSentences || renderedSentences > 2*sourceSentences {
		return errors.New("the sentence count differs too much; keep every statement and request")
	}
	if strings.Contains(withoutProtectedSpans(source), "?") && !strings.Contains(withoutProtectedSpans(rendered), "?") {
		return errors.New("the open questions disappeared; keep them as questions")
	}
	return nil
}

// spelledAsMonth reports whether a digit run the rendering dropped is a month
// it wrote by name. Every place the source holds the run is a month position,
// and the rendering names that month. A count that shares the digits still
// has to survive as digits.
func spelledAsMonth(run, source, rendered string) bool {
	number, err := strconv.Atoi(run)
	if err != nil || number < 1 || number > 12 {
		return false
	}
	asMonth := 0
	for _, match := range monthPosition.FindAllStringSubmatch(source, -1) {
		if slices.Contains(match[1:], run) {
			asMonth++
		}
	}
	if asMonth < countRuns(source, run) {
		return false
	}
	lowered := strings.ToLower(rendered)
	words := slices.DeleteFunc(strings.FieldsFunc(lowered, func(r rune) bool { return !unicode.IsLetter(r) }),
		func(word string) bool { return !proseMonths[word] })
	for _, match := range datedNameLike.FindAllStringSubmatch(lowered, -1) {
		words = append(words, match[1]+match[2])
	}
	return slices.ContainsFunc(words, func(word string) bool {
		month, ok := textlang.MonthWord(word)
		return ok && int(month) == number
	})
}

func countRuns(text, run string) int {
	count := 0
	for _, found := range digitRun.FindAllString(text, -1) {
		if found == run {
			count++
		}
	}
	return count
}

func sentenceCount(text string) int {
	text = withoutProtectedSpans(text)
	// Single-letter abbreviations and initials are common in German prose.
	text = initial.ReplaceAllStringFunc(text, func(span string) string {
		return strings.TrimSuffix(span, ".")
	})
	count, start := 0, 0
	for _, end := range sentenceEnd.FindAllStringIndex(text, -1) {
		if strings.TrimSpace(text[start:end[0]]) != "" {
			count++
		}
		start = end[1]
	}
	if strings.TrimSpace(text[start:]) != "" {
		count++
	}
	return count
}

func withoutProtectedSpans(text string) string {
	// Dots inside links, addresses and numbers do not end a sentence.
	for _, pattern := range []*regexp.Regexp{linkRun, emailRun, digitRun} {
		text = pattern.ReplaceAllStringFunc(text, func(span string) string {
			return "x" + span[len(strings.TrimRight(span, ".!?")):]
		})
	}
	return text
}
