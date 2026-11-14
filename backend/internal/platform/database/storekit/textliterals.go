// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "strings"

// TextLiterals spells values as a comma-separated list of SQL text literals.
// A generic plan keeps a partial index only when its predicate's value is a
// literal. So a vocabulary value is spelled, not bound. Every value must be
// compile-time vocabulary or checked against one.
func TextLiterals[T ~string](values ...T) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(string(value), "'", "''")+"'")
	}
	return strings.Join(quoted, ", ")
}
