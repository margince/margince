// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	sentenceEnd = regexp.MustCompile(`[.!?]+(?:\s+|$)`)
	initial     = regexp.MustCompile(`(^|[\s(])\p{L}\.`)
)

func checkSection(source, rendered string) error {
	source, rendered = strings.TrimSpace(source), strings.TrimSpace(rendered)
	if (source == "") != (rendered == "") {
		return errors.New("keep each section present or absent as in the original")
	}
	for _, pattern := range []*regexp.Regexp{digitRun, emailRun, linkRun} {
		if lost := missingSpans(pattern, source, rendered); len(lost) > 0 {
			return fmt.Errorf("the rendering lost %s; keep every number, email address and link exactly",
				strings.Join(lost, ", "))
		}
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
