// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"regexp"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

var textBoundField = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func textBoundFault(err error) (Fault, bool) {
	column, limit, ok := storekit.TextBoundViolation(err)
	if !ok || !textBoundField.MatchString(column) {
		return Fault{}, false
	}
	switch limit {
	case values.TextLabelCharacters, values.TextLineCharacters, values.TextNoteCharacters, values.TextDocumentCharacters:
	default:
		return Fault{}, false
	}
	fault, classified := Classify(TooLong(column, limit, 0))
	fault.InfraCause = err
	return fault, classified
}
