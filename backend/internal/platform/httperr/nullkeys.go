// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"sort"
)

// NullKeys names the keys of an object whose value is an explicit null, sorted.
// A request body and a patch held as raw JSON read their nulls through it.
// The REST door and the tool door therefore apply one rule.
func NullKeys(fields map[string]json.RawMessage) []string {
	var cleared []string
	for name, raw := range fields {
		if string(raw) == "null" {
			cleared = append(cleared, name)
		}
	}
	sort.Strings(cleared)
	return cleared
}

// NullKeysOf is NullKeys for a JSON object held as raw bytes.
func NullKeysOf(object json.RawMessage) ([]string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(object, &fields); err != nil {
		return nil, err
	}
	return NullKeys(fields), nil
}
