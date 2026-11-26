// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"regexp"
	"strings"
)

// languageGrammar is `language.code`'s own CHECK, restated here so a unit's
// declaration is refused at boot with a message that names the unit rather than
// at the first write with a constraint name.
var languageGrammar = regexp.MustCompile(`^[a-z]{2}$`)

// maxLanguageLabelRunes is the bound the language table puts on a label.
const maxLanguageLabelRunes = 120

// Language is one language a unit lets stored data carry, beside the three core
// ships. Declaring it does not translate the product: a language with no UI
// catalogue falls back to English on screen.
type Language struct {
	// Code is the lower-case ISO 639-1 code, for example `ar`.
	Code string
	// Label is the language's name in English.
	Label string
}

// Validate refuses a code the language table could not hold, and a missing or
// over-long label.
func (l Language) Validate() error {
	if !languageGrammar.MatchString(l.Code) {
		return fmt.Errorf("language %q is not a lower-case ISO 639-1 code, for example ar", l.Code)
	}
	label := strings.TrimSpace(l.Label)
	if label == "" {
		return fmt.Errorf("language %q has no label", l.Code)
	}
	if len([]rune(label)) > maxLanguageLabelRunes {
		return fmt.Errorf("language %q has a label over %d characters", l.Code, maxLanguageLabelRunes)
	}
	return nil
}
