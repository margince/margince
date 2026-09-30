// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailsubject_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

// The pattern strips every reply prefix, stacked and in any case, and leaves a
// forward prefix standing: a forward to the sender is not an answer to them.
func TestThePatternStripsRepliesAndKeepsForwards(t *testing.T) {
	strip := regexp.MustCompile(`(?i)` + mailsubject.ReplyPrefixPattern())
	cases := map[string]string{
		"Re: Invoice":          "Invoice",
		"AW: RE:  Invoice":     "Invoice",
		"antw:Invoice":         "Invoice",
		"Fwd: Invoice":         "Fwd: Invoice",
		"WG: Re: Invoice":      "WG: Re: Invoice",
		"Invoice":              "Invoice",
		"Rechnung Re: Invoice": "Rechnung Re: Invoice",
	}
	for subject, want := range cases {
		if got := strip.ReplaceAllString(subject, ""); got != want {
			t.Errorf("stripping %q gave %q, want %q", subject, got, want)
		}
	}
}

// The pattern is written into SQL as a string literal, so it must never carry
// the character that would end one.
func TestThePatternIsSafeInsideASQLLiteral(t *testing.T) {
	if strings.ContainsAny(mailsubject.ReplyPrefixPattern(), `'%`) {
		t.Fatalf("the pattern %q carries a quote or a percent sign, which breaks the SQL literal "+
			"or the format template it is written into", mailsubject.ReplyPrefixPattern())
	}
}

// No prefix is both a reply and a forward.
func TestNoPrefixIsBothAReplyAndAForward(t *testing.T) {
	for _, reply := range mailsubject.ReplyPrefixes() {
		for _, forward := range mailsubject.ForwardPrefixes() {
			if reply == forward {
				t.Errorf("%q is in both lists", reply)
			}
		}
	}
}
