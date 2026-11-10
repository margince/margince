// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Steering is what a caller says about one draft on every drafting route.
// It holds the ask they typed, and the draft already on their screen when the
// call is a rewrite. Both are trimmed, and empty means absent. A route therefore cannot
// read "   " as a first draft on one surface and as a rewrite on another.
type Steering struct {
	Subject   string
	Intent    string
	RewriteOf string
	Language  textlang.Lang
}

// NewSteering reads the optional request fields of crm.yaml's DraftSteering.
// A language the product does not write is refused, not ignored: ignoring it
// would draft in a language the rep did not pick.
func NewSteering(intent, rewriteOf, subject, language *string) (Steering, error) {
	var out Steering
	if intent != nil {
		out.Intent = strings.TrimSpace(*intent)
	}
	if rewriteOf != nil {
		out.RewriteOf = strings.TrimSpace(*rewriteOf)
	}
	if subject != nil {
		out.Subject = strings.TrimSpace(*subject)
	}
	if language != nil {
		if !textlang.Known(*language) {
			return Steering{}, &UnknownLanguageError{Code: *language}
		}
		out.Language = textlang.Lang(*language)
	}
	return out, nil
}

const languageField = "language"

// UnknownLanguageError is a FieldFault, so every draft route answers it with a
// 422 naming `language`.
type UnknownLanguageError struct {
	Code string
}

var _ apperrors.FieldFault = (*UnknownLanguageError)(nil)

func (e *UnknownLanguageError) Error() string {
	return fmt.Sprintf("%s %q is not a language a draft is written in", languageField, e.Code)
}

// FieldFault names the language field, the invalid_enum code, and the codes a request may send instead.
func (e *UnknownLanguageError) FieldFault() (field, code, message string) {
	shipped := make([]string, 0, len(textlang.Shipped))
	for _, lang := range textlang.Shipped {
		shipped = append(shipped, string(lang))
	}
	return languageField, "invalid_enum", fmt.Sprintf("%s must be one of %s; leave it out to write in the language of the correspondence",
		languageField, strings.Join(shipped, ", "))
}
