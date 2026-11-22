// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxCompanyNameRunes bounds a name the installation already carries. It stays
// above TextLabelCharacters because a boot setting or stored row past that must
// still load. The company record refuses a new or changed name past
// TextLabelCharacters.
const MaxCompanyNameRunes = 200

// AnchorRenameRefusal is why only a human administrator renames the anchor
// company, in every place that refuses anyone else.
const AnchorRenameRefusal = "its name is the installation's name, so only an administrator renames it, on the company profile"

// CompanyNameFault refuses name under field, or answers nil when the
// installation may carry it. Counted in runes.
func CompanyNameFault(field, name string) *ParseError {
	if missing := CompanyNameMissing(field, name); missing != nil {
		return missing
	}
	trimmed := strings.TrimSpace(name)
	if n := utf8.RuneCountInString(trimmed); n > MaxCompanyNameRunes {
		return &ParseError{
			Field: field, Code: "too_long", Max: MaxCompanyNameRunes,
			Message: fmt.Sprintf("a company name is at most %d characters; this one is %d", MaxCompanyNameRunes, n),
		}
	}
	return nil
}

// CompanyNameMissing refuses a name with no visible text under field, or
// answers nil.
func CompanyNameMissing(field, name string) *ParseError {
	if !HasVisibleText(strings.TrimSpace(name)) {
		return &ParseError{Field: field, Code: "required", Message: "the company needs a name"}
	}
	return nil
}
