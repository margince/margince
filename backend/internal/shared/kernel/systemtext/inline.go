// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// quoteLookalikes turns each double quotation mark into its single twin, so a
// value cannot close the marks a sentence put around it.
var quoteLookalikes = strings.NewReplacer("“", "‘", "”", "’", "„", "‚", `"`, "'")

// Inline is a value someone else typed, made safe to quote inside a sentence a
// person approves. Its own quotation marks become single ones and its line
// breaks spaces, so a reader always sees where it ends. The result is already
// in the shape an approval summary is stored in, so its descriptor still binds.
func Inline(value string) string {
	var b strings.Builder
	space := false
	for _, r := range value {
		switch {
		case unicode.IsSpace(r):
			space = b.Len() > 0
		case r == utf8.RuneError || values.IsInvisibleRune(r):
			continue
		default:
			if space {
				b.WriteByte(' ')
				space = false
			}
			b.WriteRune(r)
		}
	}
	return quoteLookalikes.Replace(b.String())
}
