// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrole

import (
	"fmt"
	"strings"
	"unicode"
)

// recordedTrimSQL is the set strings.TrimSpace trims, as a Postgres escape
// string. A bare trim() drops only spaces, so a tab-padded role read differently.
var recordedTrimSQL = func() string {
	var set strings.Builder
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if unicode.IsSpace(r) {
			fmt.Fprintf(&set, `\u%04x`, r)
		}
	}
	return "E'" + set.String() + "'"
}()

// RecordedSQL is Recorded's normalising step in SQL, for a role column that a
// statement compares with a vocabulary key. The column is a literal of the caller.
func RecordedSQL(column string) string {
	return "lower(btrim(" + column + ", " + recordedTrimSQL + "))"
}
