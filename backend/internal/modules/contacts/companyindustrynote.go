// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const industryNotePrefix = "Industry as written: "

// industryNoteRunes bounds the raw text a note carries, so the note fits the
// description column beside the text already there.
const industryNoteRunes = 80

// industrySourceNote is a pure function of the raw text, so "already noted" is
// a substring check on the description.
func industrySourceNote(raw string) string {
	return industryNotePrefix + textcut.Runes(strings.TrimSpace(raw), industryNoteRunes)
}

// withIndustrySource appends the note for raw to the description after a space
// and never rewrites the text before it. It changes nothing when raw is empty,
// the note is already there, or the result would pass CompanyDescriptionMax.
func withIndustrySource(description *string, raw string) (next string, changed bool) {
	current := ""
	if description != nil {
		current = *description
	}
	if strings.TrimSpace(raw) == "" {
		return current, false
	}
	note := industrySourceNote(raw)
	if strings.Contains(current, note) {
		return current, false
	}
	if current != "" {
		note = current + " " + note
	}
	if utf8.RuneCountInString(note) > CompanyDescriptionMax {
		return current, false
	}
	return note, true
}
