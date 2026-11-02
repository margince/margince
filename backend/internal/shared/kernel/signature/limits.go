// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package signature holds the field limits shared by saved and previewed sign-offs.
package signature

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxRunes bounds stored markup and every preview draft.
const MaxRunes = 2000

// ValidateHTML applies the saved-field limit to a draft as well.
func ValidateHTML(markup string) error {
	count := utf8.RuneCountInString(strings.TrimSpace(markup))
	if count > MaxRunes {
		return &TooLongError{Field: "html", Max: MaxRunes, Runes: count}
	}
	return nil
}

// TooLongError names the field and its character limit.
type TooLongError struct {
	Field      string
	Max, Runes int
}

func (e *TooLongError) Error() string {
	return fmt.Sprintf("a signature's %s is at most %d characters, this one is %d", e.Field, e.Max, e.Runes)
}

// FieldFault renders the same refusal on every transport.
func (e *TooLongError) FieldFault() (field, code, message string) {
	return e.Field, "too_long", e.Error()
}
