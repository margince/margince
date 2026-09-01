// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "testing"

// Which of our kinds a card's TYPE parameters mean.
//
// The question a TYPE answers here is WHERE the number is — work, home, a
// pocket — and a vCard says several other things in the same position. Read as
// though the first value were the answer, a number the card plainly calls a
// work number comes back as `other`, and the card's own words are lost on the
// way in.
func TestAPhoneTakesItsKindFromWhicheverTypeNamesOne(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		params []string
		want   string
	}{
		"a plain work number":     {[]string{"TYPE=WORK"}, phoneTypeWork},
		"a home number":           {[]string{"TYPE=HOME"}, vcardTypeHome},
		"a cell is a mobile":      {[]string{"TYPE=CELL"}, vcardTypeMobile},
		"no type at all":          {nil, phoneTypeWork},
		"the 2.1 bare spelling":   {[]string{"WORK"}, phoneTypeWork},
		"a preferred work number": {[]string{"TYPE=PREF,WORK"}, phoneTypeWork},
		// RFC 2426 allows the types as a VALUE LIST or as repeated parameters,
		// and both spellings are in the wild. Reading the first value of the
		// first TYPE answers `voice` for each of these.
		"a value list led by voice":     {[]string{"TYPE=VOICE,WORK"}, phoneTypeWork},
		"repeated type parameters":      {[]string{"TYPE=voice", "TYPE=work"}, phoneTypeWork},
		"the 2.1 bare pair":             {[]string{"VOICE", "WORK"}, phoneTypeWork},
		"auxiliaries before a home one": {[]string{"TYPE=PREF", "TYPE=FAX,HOME"}, vcardTypeHome},
		// A property whose types are auxiliary all the way down still answers
		// with one: a fax is not a work phone.
		"a fax and nothing else": {[]string{"TYPE=FAX"}, channelKindOther},
		// PREF alone says which number is primary and nothing about where it
		// is, so the card has named no type and the caller's default applies.
		"pref and nothing else": {[]string{"TYPE=PREF"}, phoneTypeWork},
		// A kind this product does not have is an ANSWER, not an absence.
		// Dropped, it was indistinguishable from a card that named no type, and
		// this landed as a work number.
		"an unknown bare type":      {[]string{"X-custom"}, channelKindOther},
		"an unknown listed type":    {[]string{"TYPE=X-custom"}, channelKindOther},
		"a key=value is not a type": {[]string{"VALUE=uri"}, phoneTypeWork},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := phoneKindFrom(tc.params); got != tc.want {
				t.Errorf("phoneKindFrom(%v) = %q, want %q", tc.params, got, tc.want)
			}
		})
	}
}

// The same reading serves an email, which has fewer kinds and the same trap.
func TestAnEmailTakesItsKindFromWhicheverTypeNamesOne(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		params []string
		want   string
	}{
		"a work address":             {[]string{"TYPE=WORK"}, emailTypeWork},
		"a home address is personal": {[]string{"TYPE=HOME"}, "personal"},
		"no type at all":             {nil, emailTypeWork},
		// The ordinary vCard 2.1 spelling of a private address: INTERNET says
		// how it is delivered, not whose it is.
		"internet before home":      {[]string{"TYPE=INTERNET,HOME"}, "personal"},
		"internet and nothing else": {[]string{"TYPE=INTERNET"}, channelKindOther},
		"pref before home":          {[]string{"TYPE=PREF,HOME"}, "personal"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := emailKindFrom(tc.params); got != tc.want {
				t.Errorf("emailKindFrom(%v) = %q, want %q", tc.params, got, tc.want)
			}
		})
	}
}
