// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package langcode answers whether a two-letter code names a language, for the
// writers that store one a document or a product text is written in.
package langcode

import "golang.org/x/text/language"

// Known reports whether code is a lowercase ISO 639-1 code the language
// registry assigns. "zz" has the right shape and names no language.
func Known(code string) bool {
	if len(code) != 2 || code[0] < 'a' || code[0] > 'z' || code[1] < 'a' || code[1] > 'z' {
		return false
	}
	base, err := language.ParseBase(code)
	return err == nil && base.String() == code
}
