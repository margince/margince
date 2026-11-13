// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package projectkey is the one shape a project key may take. The projects
// module mints and refuses keys against it. Capture reads keys out of subject
// lines with it. The project_key_shape CHECK states the same shape, and this
// package's test holds the two together.
package projectkey

// CheckPattern is the regular expression of the project_key_shape CHECK.
const CheckPattern = `^[A-Za-z][A-Za-z0-9_-]{1,23}$`

// MinLen and MaxLen bound a key in characters; every allowed character is
// one byte.
const (
	MinLen = 2
	MaxLen = 24
)

// Shaped reports whether text could be a project key: an ASCII letter, then
// letters, digits, underscores or hyphens, 2 to 24 in all. Case does not
// matter, as keys match case-insensitively.
//
// A bare number fails the letter-led rule, which keeps `[2026]` and `[4711]`,
// a year and an order number, from ever reading as a key.
func Shaped(text string) bool {
	if len(text) < MinLen || len(text) > MaxLen || !letter(text[0]) {
		return false
	}
	for i := 1; i < len(text); i++ {
		b := text[i]
		if !letter(b) && (b < '0' || b > '9') && b != '_' && b != '-' {
			return false
		}
	}
	return true
}

func letter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
