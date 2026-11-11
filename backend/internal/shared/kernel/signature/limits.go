// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package signature holds the field limits shared by saved and previewed sign-offs.
package signature

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxHTMLRunes bounds stored markup and every preview draft, counted on the raw
// input: the size a mail client's signature editor produces, as Gmail caps it.
const MaxHTMLRunes = 10000

// ValidateHTML applies the saved-field limit to a draft as well.
func ValidateHTML(markup string) error {
	count := utf8.RuneCountInString(strings.TrimSpace(markup))
	if count > MaxHTMLRunes {
		return &TooLongError{Field: "html", Max: MaxHTMLRunes, Runes: count}
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

// FieldMax is the limit the field was refused at.
func (e *TooLongError) FieldMax() int { return e.Max }

// PreviewImages draws a sanitized signature's https images for the app's own
// preview, whose page may not load a remote image. A send never asks it.
type PreviewImages interface {
	Inline(ctx context.Context, markup string) string
}
