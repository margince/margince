// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Version is the extension's own version string, expected stable for an
// unchanged unit: the boot inventory records it and logs a change. It
// carries no authority (operator decisions bind to digests,
// never to a version string).
type Version string

// Validate requires a non-empty, single-line printable string. The
// inventory writes it into system_log verbatim, where control characters
// and surrounding whitespace cannot be read back.
func (v Version) Validate() error {
	if v == "" {
		return errors.New("extension version is empty — the boot inventory records it")
	}
	if strings.TrimSpace(string(v)) != string(v) {
		return fmt.Errorf("extension version %q carries surrounding whitespace", string(v))
	}
	for _, r := range v {
		if !unicode.IsPrint(r) {
			return fmt.Errorf("extension version %q carries a non-printable character", string(v))
		}
	}
	return nil
}

// Description is one operator-facing sentence saying what the unit is for.
//
// Required, because the screen that grants a unit's permissions lists every
// composed unit by name. A name alone leaves an admin deciding what to grant to
// something called "de". Only the unit's author knows the answer, so the
// declaration carries it.
type Description string

// Validate requires a non-empty, single-line printable sentence. The
// Extensions settings page shows it to an operator as written.
func (d Description) Validate() error {
	if strings.TrimSpace(string(d)) == "" {
		return errors.New("extension description is empty — the access screen lists the unit by it")
	}
	if strings.TrimSpace(string(d)) != string(d) {
		return fmt.Errorf("extension description %q carries surrounding whitespace", string(d))
	}
	for _, r := range d {
		if !unicode.IsPrint(r) {
			return fmt.Errorf("extension description %q carries a non-printable character", string(d))
		}
	}
	// Runes, not bytes: a German or Vietnamese sentence is well inside the
	// bound and well past it in UTF-8, and the message says "characters".
	if n := utf8.RuneCountInString(string(d)); n > maxDescriptionLength {
		return fmt.Errorf("extension description is %d characters — one sentence, capped at %d", n, maxDescriptionLength)
	}
	return nil
}

// One sentence, not a manual: the screen gives it one line under the unit name.
const maxDescriptionLength = 200
