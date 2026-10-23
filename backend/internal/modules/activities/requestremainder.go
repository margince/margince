// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RequestRemainderGrounded binds a remaining ask to the original message, excluding quoted history.
func RequestRemainderGrounded(body, remaining string) bool {
	words := strings.FieldsFunc(remaining, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	return len(words) >= 2 && values.Quoted(textlang.MessageOpening(body, utf8.RuneCountInString(body)), remaining)
}
