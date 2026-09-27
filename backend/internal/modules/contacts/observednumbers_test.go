// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What a statement's listed numbers contribute before anything is written.

import (
	"slices"
	"testing"
)

// contact_phone.phone is E.164 by contract and no database constraint holds it,
// so the ONLY thing making that contract true for this writer is
// values.ParsePhone. A signature states a number however its author types it.
func TestAListedNumberIsNormalizedOrDeclined(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  string
		want []string
	}{
		"separators are formatting":        {"+49 (30) 1234-5678", []string{"+493012345678"}},
		"00 is the dialled form of +":      {"0049 30 12345678", []string{"+493012345678"}},
		"already normalized":               {"+493012345678", []string{"+493012345678"}},
		"no country prefix is unreachable": {"030 12345678", nil},
		"a word is not a number":           {"call me", nil},
		"empty contributes nothing":        {"   ", nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := listedPhones(parseListedNumbers([]observedNumber{{Phone: tc.raw}})); !slices.Equal(got, tc.want) {
				t.Errorf("numbers = %v, want %v", got, tc.want)
			}
		})
	}
}

// Four numbers are four numbers, one unreadable footer line does not cost the
// other three, and the same number spelled twice is one number.
func TestEveryReadableNumberOfAStatementIsKeptOnce(t *testing.T) {
	listed := parseListedNumbers([]observedNumber{
		{Phone: "+49 175 5550101", Evidence: "Germany: +49 175 5550101"},
		{Phone: "+84 35 5550102"},
		{Phone: "030 5550103"},
		{Phone: "+65 9555 0104"},
		{Phone: "0049 175 5550101", Evidence: "again"},
		{Phone: "+66 97 555 0105", PhoneType: "mobile"},
	})
	want := []string{"+491755550101", "+84355550102", "+6595550104", "+66975550105"}
	if got := listedPhones(listed); !slices.Equal(got, want) {
		t.Fatalf("numbers = %v, want %v", got, want)
	}
	if listed[0].Evidence != "Germany: +49 175 5550101" {
		t.Errorf("evidence = %q, want the first listing's line to stand", listed[0].Evidence)
	}
	if listed[0].PhoneType != emailTypeWork || listed[3].PhoneType != "mobile" {
		t.Errorf("types = %q/%q, want an unstated type read as work and a stated one kept",
			listed[0].PhoneType, listed[3].PhoneType)
	}
}

func listedPhones(listed []listedNumber) []string {
	var out []string
	for _, n := range listed {
		out = append(out, n.phone.String())
	}
	return out
}
