// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "unicode"

// HasVisibleText reports whether s holds a character a reader can see. A zero-width
// space or joiner is not white space to unicode.IsSpace, so trimming alone lets a
// name that renders as nothing through; those are format characters (Cf).
func HasVisibleText(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) && !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}
