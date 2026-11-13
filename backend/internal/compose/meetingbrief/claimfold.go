// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// One promise, one line. The claim store files a claim per contact, so a
// promise made on a thread with three attendees reaches the brief three times.
// Every mailbox that received the same invitation is also read on its own, so
// one promise can come back a second time in other words. Both are folded here,
// before anything ranks or quotes a claim, into one claim that names everyone
// it was made to.

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// nearClaimOverlap is the share of the shorter claim's long words the other
// must repeat for two claims due the same day to be one promise. Two readings
// of one invitation share the names, the subject and the date, and differ in
// the verb the extractor chose.
const nearClaimOverlap = 0.6

// longWord is the fewest letters a word needs to count toward the overlap.
// Two different promises share their articles and prepositions; the object of
// the promise is what tells them apart.
const longWord = 4

var claimWords = struct{ And langcopy.Phrase }{
	And: langcopy.Phrase{En: "%s and %s", De: "%s und %s", Vi: "%s và %s"},
}

// foldSameClaims keeps the first of each promise in input order and adds the
// contacts of its repeats to it. A repeat's due date fills one the kept claim
// lacks, so the folded promise still reads as overdue.
func foldSameClaims(claims []ClaimIn) []ClaimIn {
	out := make([]ClaimIn, 0, len(claims))
	for _, claim := range claims {
		kept := slices.IndexFunc(out, func(earlier ClaimIn) bool { return sameClaim(earlier, claim) })
		if kept < 0 {
			out = append(out, claim)
			continue
		}
		if !slices.Contains(out[kept].contactNames(), claim.ContactName) {
			out[kept].AlsoTo = append(out[kept].AlsoTo, claim.ContactName)
		}
		if out[kept].DueAt == nil {
			out[kept].DueAt = claim.DueAt
		}
	}
	return out
}

// sameClaim needs the same kind and status, because a kept promise and an open
// one are two facts even when they read alike. The same words in the same
// order are one promise unless the two are due on different days. Similar
// wording is one promise only when both are due the same day.
func sameClaim(a, b ClaimIn) bool {
	if a.Kind != b.Kind || a.Status != b.Status {
		return false
	}
	left, right := bodyWords(a.Body), bodyWords(b.Body)
	if slices.Equal(left, right) {
		return a.DueAt == nil || b.DueAt == nil || sameDueDay(a.DueAt, b.DueAt)
	}
	if !sameDueDay(a.DueAt, b.DueAt) {
		return false
	}
	longLeft, longRight := longWords(left), longWords(right)
	shared := 0
	for word := range longLeft {
		if _, ok := longRight[word]; ok {
			shared++
		}
	}
	shorter := min(len(longLeft), len(longRight))
	return shorter > 0 && float64(shared) >= nearClaimOverlap*float64(shorter)
}

func bodyWords(body string) []string {
	return strings.FieldsFunc(strings.ToLower(body), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func longWords(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		if utf8.RuneCountInString(word) >= longWord {
			set[word] = struct{}{}
		}
	}
	return set
}

func sameDueDay(a, b *time.Time) bool {
	return a != nil && b != nil && a.UTC().Format(time.DateOnly) == b.UTC().Format(time.DateOnly)
}

func (claim ClaimIn) contactNames() []string {
	return append([]string{claim.ContactName}, claim.AlsoTo...)
}

// whom names every contact a claim was made to or by, as one phrase.
func (said briefCopy) whom(claim ClaimIn) string {
	names := claim.contactNames()
	if len(names) == 1 {
		return names[0]
	}
	last := len(names) - 1
	return fmt.Sprintf(said.Say(claimWords.And), strings.Join(names[:last], ", "), names[last])
}
