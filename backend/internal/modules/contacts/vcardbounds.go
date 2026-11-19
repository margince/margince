// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"
	"slices"
	"unicode/utf8"
)

// withTitleNote adds to a written card's reason that its title was cut to fit,
// when the record took that title. A reader who sees only "imported" would
// believe the whole title landed.
func withTitleNote(reason string, entry VCardEntry, fields []VCardField) string {
	kept := utf8.RuneCountInString(entry.Title)
	if entry.titleRunes <= kept || !slices.ContainsFunc(fields, titleWritten) {
		return reason
	}
	note := fmt.Sprintf("The title is %d characters, so only its first %d were imported.", entry.titleRunes, kept)
	if reason == "" {
		return note
	}
	return reason + " " + note
}

func titleWritten(field VCardField) bool {
	return field.Field == fieldTitle && (field.Action == VCardFieldImported || field.Action == VCardFieldReplaced)
}
