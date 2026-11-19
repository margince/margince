// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The password rule every door shares: change, reset, the first admin and the
// operator reset.

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The password bound, shared by every path that accepts one. Counted in runes:
// a four-emoji password is sixteen bytes and would clear a byte floor of twelve.
const (
	minPasswordLen = 12
	maxPasswordLen = 256
)

// passwordRuleError is the password rule for every Go caller that accepts a
// password: 12 to 256 runes, not blank, and not one character repeated.
//
// It returns the field-shaped refusal the surfaces render, so a caller does not
// restate the numbers in prose that can drift from the check. The browser
// states the same rule in frontend/src/features/sign-in/passwordrule.ts, and
// TestThePasswordRuleMirrorsTheServer holds the two together.
func passwordRuleError(field, pw string) error {
	n := utf8.RuneCountInString(pw)
	switch {
	case n < minPasswordLen || n > maxPasswordLen:
		return &values.ParseError{
			Field:   field,
			Code:    "length",
			Message: fmt.Sprintf("the password must be %d–%d characters", minPasswordLen, maxPasswordLen),
		}
	case isBlank(pw):
		return &values.ParseError{Field: field, Code: "blank", Message: "the password cannot be only spaces"}
	case isOneRepeatedRune(pw):
		return &values.ParseError{Field: field, Code: "repeated", Message: "the password cannot be one character repeated"}
	}
	return nil
}

// passwordEmailError refuses a password that is the account's own address, the
// first guess anyone makes. The address is compared the way a login is, by
// sameLoginIdentity.
func passwordEmailError(field, pw, email string) error {
	if sameLoginIdentity(pw, email) {
		return &values.ParseError{Field: field, Code: "email", Message: "the password cannot be your email address"}
	}
	return nil
}

// isBlank is true when every code point is white space by unicode.IsSpace. The
// browser spells the same set in passwordrule.ts; the golden corpus in
// testdata/passwordrule.json holds both to it.
func isBlank(pw string) bool {
	return strings.IndexFunc(pw, func(r rune) bool { return !unicode.IsSpace(r) }) < 0
}

func isOneRepeatedRune(pw string) bool {
	first, _ := utf8.DecodeRuneInString(pw)
	for _, r := range pw {
		if r != first {
			return false
		}
	}
	return true
}
