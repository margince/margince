// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// DecodedValueRefusal holds a decoded body to the rules a JSON decode cannot
// see: the date-time range, number bounds and lists that refuse repeats. Every
// decoder of a contract body calls it with the raw body, so each door answers
// with one code.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func DecodedValueRefusal(into any, raw json.RawMessage) error {
	if outOfRange := datasource.RejectOutOfRangeTimes(into); outOfRange != nil {
		return Validation(outOfRange.Field, "out_of_range", outOfRange.Error())
	}
	if outOfBounds := datasource.RejectOutOfBoundsNumbers(into, raw); outOfBounds != nil {
		return Validation(outOfBounds.Field, "out_of_range", outOfBounds.Error())
	}
	if repeated := datasource.RejectRepeatedItems(into); repeated != nil {
		return Validation(repeated.Field, "duplicate", repeated.Error())
	}
	return nil
}
