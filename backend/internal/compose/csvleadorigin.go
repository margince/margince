// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead origin columns of a CSV import.

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// leadTargetOrigin and leadTargetOriginDetail are where the lead came from, as
// the source system worded it. The value is free text. The store resolves it to
// a lead_origin key, or keeps the wording in the note.
const (
	leadTargetOrigin       = "origin"
	leadTargetOriginDetail = "origin_detail"
)

// originFrom reads a row's origin wording and note. The note is cut to what the
// column holds. A spreadsheet value can run to any length, and a note is not
// worth refusing a row over.
func originFrom(fields map[string]string) (hint string, note *string) {
	if wording := importString(fields, leadTargetOrigin); wording != nil {
		hint = *wording
	}
	if text := importString(fields, leadTargetOriginDetail); text != nil {
		cut := textcut.Runes(*text, values.MaxLeadOriginDetailRunes)
		note = &cut
	}
	return hint, note
}

// leadOriginKeptOnReimport is true when a file's origin value changes nothing.
// A lead that already has an origin or note keeps it, because a human may have
// set it and a later file knows no better. A wording the lead already holds as
// its note is not news either. The store applies the same rule under the row
// lock. This only keeps the run from counting a rewrite that cannot happen.
func leadOriginKeptOnReimport(current map[string]json.RawMessage, field, incoming string) bool {
	switch field {
	case leadTargetOrigin:
		return textOf(current[leadTargetOrigin]) != "" || textOf(current[leadTargetOriginDetail]) == incoming
	case leadTargetOriginDetail:
		return textOf(current[leadTargetOriginDetail]) != ""
	}
	return false
}
