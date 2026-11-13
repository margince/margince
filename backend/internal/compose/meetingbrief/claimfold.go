// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// One promise, one line. The claim store files a claim per contact, so a
// promise made on a thread with three attendees reaches the brief three times.
// The copies are folded here, before anything ranks or quotes a claim, into one
// claim that names everyone it was made to.

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

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
// one are two facts even when they read alike. It needs the same words in the
// same order. Two wordings can share most of their words and still name two
// deliverables, so close wording is never one promise. Known due days agree.
func sameClaim(a, b ClaimIn) bool {
	if a.Kind != b.Kind || a.Status != b.Status || !slices.Equal(bodyWords(a.Body), bodyWords(b.Body)) {
		return false
	}
	return a.DueAt == nil || b.DueAt == nil || sameDueDay(a.DueAt, b.DueAt)
}

func bodyWords(body string) []string {
	return strings.FieldsFunc(strings.ToLower(body), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
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
