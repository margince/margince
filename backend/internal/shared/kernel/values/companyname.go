// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxCompanyNameRunes bounds the name the installation and its own company
// share. Registered German names with the legal form spelled out run past 63.
const MaxCompanyNameRunes = 200

// CompanyNameFault answers the contract code and the sentence refusing name, or
// an empty code when the installation may carry it. Counted in runes.
func CompanyNameFault(name string) (code, message string) {
	trimmed := strings.TrimSpace(name)
	if !HasVisibleText(trimmed) {
		return "required", "the company needs a name"
	}
	if n := utf8.RuneCountInString(trimmed); n > MaxCompanyNameRunes {
		return "too_long", fmt.Sprintf("a company name is at most %d characters; this one is %d",
			MaxCompanyNameRunes, n)
	}
	return "", ""
}
