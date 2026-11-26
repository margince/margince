// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// SentenceStart fails a phrase that starts in lower case in any shipped
// language, for a table whose every phrase is shown as a whole message.
//
//craft:ignore naked-any the table is the caller's own copy struct or map, as for Census
func SentenceStart(t reporter, table any) {
	t.Helper()
	walk(t, table, func(t reporter, name string, p langcopy.Phrase) {
		t.Helper()
		for _, lang := range textlang.Shipped {
			if r, _ := utf8.DecodeRuneInString(p.In(lang)); !unicode.IsUpper(r) {
				t.Errorf("%s %s starts in lower case: %q", lang, name, p.In(lang))
			}
		}
	})
}
