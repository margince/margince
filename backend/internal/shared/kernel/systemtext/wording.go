// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Says reports whether text is code's phrase in language, worded as now or as
// a stored row may still hold it.
func Says(code string, language textlang.Lang, text string) bool {
	definition, ok := definitionOf(code)
	return ok && (text == definition.Words.In(language) || slices.Contains(definition.Retired[language], text))
}

// SaysInAnyLanguage reports whether text is code's phrase in some shipped
// language, worded as now or as a stored row may still hold it.
func SaysInAnyLanguage(code, text string) bool {
	return slices.ContainsFunc(textlang.Shipped, func(language textlang.Lang) bool { return Says(code, language, text) })
}

// Reworded is code's phrase as worded now, in the language text says it in.
// Text that is not the phrase comes back unchanged.
func Reworded(code, text string) string {
	for _, language := range textlang.Shipped {
		if Says(code, language, text) {
			return Copy(code, language)
		}
	}
	return text
}
