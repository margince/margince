// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
)

// e164 is the wire form the schema documents for contact_phone: "+",
// a non-zero country digit, 8–15 digits total.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// Phone is an E.164-normalized number. Separators are formatting, so
// parsing strips them; a leading 00 is the international-dialing
// spelling of +. A number WITHOUT a country prefix is rejected rather
// than guessed — region inference needs carrier metadata this
// dependency-free kernel deliberately does not carry.
type Phone struct{ s string }

var phoneSeparators = strings.NewReplacer(" ", "", "(", "", ")", "", ".", "", "-", "", "/", "", "\t", "")

func ParsePhone(raw string) (Phone, error) {
	cleaned := phoneSeparators.Replace(strings.TrimSpace(raw))
	if cleaned == "" {
		return Phone{}, &ParseError{Field: "phone", Code: "phone_empty", Message: "a phone number is required"}
	}
	if strings.HasPrefix(cleaned, "00") {
		cleaned = "+" + cleaned[2:]
	}
	if !strings.HasPrefix(cleaned, "+") {
		return Phone{}, &ParseError{Field: "phone", Code: "phone_needs_country_code",
			Message: "the number needs its country prefix (+49…, 0049…)"}
	}
	if !e164.MatchString(cleaned) {
		return Phone{}, &ParseError{Field: "phone", Code: "phone_malformed",
			Message: "not an E.164 number (+ and 8–15 digits)"}
	}
	return Phone{s: cleaned}, nil
}

func (p Phone) String() string { return p.s }
func (p Phone) IsZero() bool   { return p.s == "" }

// CountryCode is the number's country calling code, without the "+".
//
// Readable from the digits alone because ITU calling codes are prefix-free: 1
// and 7 are the only one-digit codes, the two-digit ones are the fixed set
// below, and every other code is three digits. Two numbers with one code are
// one country's, which is how a newer number knows which older one it replaces.
func (p Phone) CountryCode() string {
	digits := strings.TrimPrefix(p.s, "+")
	if digits == "" {
		return ""
	}
	if digits[0] == '1' || digits[0] == '7' {
		return digits[:1]
	}
	if twoDigitCallingCodes[digits[:2]] {
		return digits[:2]
	}
	return digits[:3]
}

// twoDigitCallingCodes is ITU-T E.164's two-digit assignments.
var twoDigitCallingCodes = map[string]bool{
	"20": true, "27": true, "30": true, "31": true, "32": true, "33": true, "34": true,
	"36": true, "39": true, "40": true, "41": true, "43": true, "44": true, "45": true,
	"46": true, "47": true, "48": true, "49": true, "51": true, "52": true, "53": true,
	"54": true, "55": true, "56": true, "57": true, "58": true, "60": true, "61": true,
	"62": true, "63": true, "64": true, "65": true, "66": true, "81": true, "82": true,
	"84": true, "86": true, "90": true, "91": true, "92": true, "93": true, "94": true,
	"95": true, "98": true,
}

func (p Phone) Value() (driver.Value, error) { return p.s, nil }

//craft:ignore naked-any sql.Scanner mandates the any source parameter
func (p *Phone) Scan(src any) error {
	switch v := src.(type) {
	case string:
		p.s = v
	case []byte:
		p.s = string(v)
	default:
		return fmt.Errorf("values: cannot scan %T into Phone", src)
	}
	return nil
}
