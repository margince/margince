// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"regexp"
	"strings"
)

// EmailTextPattern matches an address without matching a longer address.
// The result is a PostgreSQL regular expression for case-insensitive matching.
func EmailTextPattern(email string) string {
	return `(?<![[:alnum:]._%+-])` + regexp.QuoteMeta(strings.TrimSpace(email)) + `(?=$|[^[:alnum:].-]|\.(?:$|[^[:alnum:]]))`
}

// FullNameTextPattern matches whole words with flexible spacing. Callers choose
// the minimum word count because automatic erasure policies differ by record.
func FullNameTextPattern(fullName string, minimumWords int) (string, bool) {
	words := strings.Fields(fullName)
	if len(words) == 0 || len(words) < minimumWords {
		return "", false
	}
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		quoted = append(quoted, regexp.QuoteMeta(word))
	}
	return `\m` + strings.Join(quoted, `\s+`) + `\M`, true
}
