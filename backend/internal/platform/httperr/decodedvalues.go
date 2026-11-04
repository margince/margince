// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import "github.com/margince/margince/backend/internal/shared/ports/datasource"

// decodedValueRefusal holds a decoded body to the contract rules that a JSON
// decode cannot see: the stored date-time range and lists that refuse repeats.
//
//craft:ignore naked-any the JSON deserialization seam: the decode target is whichever contract request struct the handler owns
func decodedValueRefusal(into any) error {
	if outOfRange := datasource.RejectOutOfRangeTimes(into); outOfRange != nil {
		return Validation(outOfRange.Field, "out_of_range", outOfRange.Error())
	}
	if repeated := datasource.RejectRepeatedItems(into); repeated != nil {
		return Validation(repeated.Field, "duplicate", repeated.Error())
	}
	return nil
}
