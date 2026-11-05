// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

import (
	"encoding"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ActivityArgument has the same meaning before and after a wire round trip.
func ActivityArgument(args map[string]any) (ids.UUID, bool) {
	raw, present := args["request_activity_id"]
	if !present {
		raw, present = args["activity_id"]
	}
	if !present {
		return ids.UUID{}, false
	}
	return RecordArgument(raw)
}

// RecordArgument reads a typed builder operand or its stored JSON value.
func RecordArgument(raw any) (ids.UUID, bool) { //craft:ignore naked-any reads the contract's open arguments object
	switch id := raw.(type) {
	case ids.UUID:
		return id, true
	case encoding.TextMarshaler:
		text, err := id.MarshalText()
		if err != nil {
			return ids.UUID{}, false
		}
		parsed, err := ids.Parse(string(text))
		return parsed, err == nil
	case string:
		parsed, err := ids.Parse(id)
		return parsed, err == nil
	default:
		return ids.UUID{}, false
	}
}
