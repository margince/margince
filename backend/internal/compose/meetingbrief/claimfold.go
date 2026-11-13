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

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// nearClaimOverlap is the share of the shorter claim's words the longer one
// must repeat for two claims due the same day to be one promise. Two readings
// of one invitation share the names, the subject and the date, and differ in
// the verb the extractor chose.
const nearClaimOverlap = 0.6

var claimWords = struct{ And langcopy.Phrase }{
	And: langcopy.Phrase{En: "%s and %s", De: "%s und %s", Vi: "%s và %s"},
}

// foldSameClaims keeps the first of each promise in input order and adds the
// contacts of its repeats to it.
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
	}
	return out
}

// sameClaim needs the same kind and status, because a kept promise and an open
// one are two facts even when they read alike. Identical wording is one promise
// on any date; similar wording only when both are due the same day.
func sameClaim(a, b ClaimIn) bool {
	if a.Kind != b.Kind || a.Status != b.Status {
		return false
	}
	left, right := claimWordSet(a.Body), claimWordSet(b.Body)
	shared := 0
	for word := range left {
		if _, ok := right[word]; ok {
			shared++
		}
	}
	if shared == len(left) && shared == len(right) {
		return true
	}
	shorter := min(len(left), len(right))
	return shorter > 0 && sameDueDay(a.DueAt, b.DueAt) &&
		float64(shared) >= nearClaimOverlap*float64(shorter)
}

func claimWordSet(body string) map[string]struct{} {
	words := strings.FieldsFunc(strings.ToLower(body), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		set[word] = struct{}{}
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
