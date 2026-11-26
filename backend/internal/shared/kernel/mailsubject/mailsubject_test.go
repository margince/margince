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

// Topic strips reply and forward prefixes alike, stacked and in any case, and
// nothing a sender wrote after them.
func TestTopicStripsRepliesAndForwards(t *testing.T) {
	cases := map[string]string{
		"Re: Termin und Angebot":      "termin und angebot",
		"WG: AW:  Termin und Angebot": "termin und angebot",
		"fwd:Re: Termin":              "termin",
		"Termin":                      "termin",
		"Re: Termin, unser Angebot":   "termin, unser angebot",
		"Angebot Re: Termin":          "angebot re: termin",
		"FW: Re: Termin":              "termin",
	}
	for subject, want := range cases {
		if got := mailsubject.Topic(subject); got != want {
			t.Errorf("Topic(%q) = %q, want %q", subject, got, want)
		}
	}
}

// A subject forwards when its outermost prefix does. An answer to a forward is
// a reply, and a word that only starts like a prefix is the subject.
func TestIsForwardReadsTheOutermostPrefix(t *testing.T) {
	cases := map[string]bool{
		"WG: Angebot":       true,
		"  fwd:Angebot":     true,
		"FW : Angebot":      true,
		"\u00a0WG: Angebot": true,
		"\u2003WG: Angebot": false,
		"Fwd: Re: Angebot":  true,
		"Re: Fwd: Angebot":  false,
		"AW: WG: Angebot":   false,
		"Forward planning":  false,
		"Fwdx: Angebot":     false,
		"Angebot WG: Lager": false,
		"":                  false,
	}
	for subject, want := range cases {
		if got := mailsubject.IsForward(subject); got != want {
			t.Errorf("IsForward(%q) = %t, want %t", subject, got, want)
		}
	}
}
