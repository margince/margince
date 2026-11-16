// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturekind canonicalizes legacy labels at read boundaries.
package capturekind

const legacyOther = "noise"

// Other classifies neither a commitment nor a meeting, without judging importance.
const Other = "other"

// Canonical preserves immutable legacy storage while naming its public meaning.
func Canonical(label string) string {
	if label == legacyOther {
		return Other
	}
	return label
}

// SQL is Canonical for a compile-time SQL column expression.
func SQL(column string) string {
	return "CASE WHEN " + column + " = '" + legacyOther + "' THEN '" + Canonical(legacyOther) + "' ELSE " + column + " END"
}
