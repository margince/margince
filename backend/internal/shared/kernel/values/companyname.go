// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxCompanyNameRunes bounds the name the installation and its own company
// share, with room for a registered name whose legal form is spelled out.
const MaxCompanyNameRunes = 200

// AnchorRenameRefusal is why only a human administrator renames the anchor
// company, in every place that refuses anyone else.
const AnchorRenameRefusal = "its name is the installation's name, so only an administrator renames it, on the company profile"

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
