// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "unicode"

// HasVisibleText reports whether s holds a character a reader can see.
// A zero-width space is a format character (Cf), not white space. A combining
// mark (M) draws nothing without a base. Trimming alone lets both through.
func HasVisibleText(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) && !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) && !unicode.IsMark(r) {
			return true
		}
	}
	return false
}
