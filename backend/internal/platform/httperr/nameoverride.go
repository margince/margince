// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"bytes"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// nameKeys are the body keys that carry the name or title of a record or a
// person, wherever they sit in the body.
var nameKeys = map[string]bool{
	"name": true, "full_name": true, "first_name": true, "last_name": true,
	"display_name": true, "title": true, "job_title": true,
}

// nameOverrideRefusal refuses a name or title that carries a bidirectional
// override, which would show it in another order than the one stored. Mail and
// imports reach the store without this door, so only what a person or an agent
// sends is refused. A body that is not JSON is left to the decode to refuse.
func nameOverrideRefusal(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	// inObject holds, per open container, whether it is an object; key is the
	// last key read in the innermost object, cleared once its value is read.
	var inObject []bool
	key, expectKey := "", false
	for {
		token, err := dec.Token()
		if err != nil {
			return nil
		}
		switch v := token.(type) {
		case json.Delim:
			switch v {
			case '{':
				inObject = append(inObject, true)
				expectKey = true
			case '[':
				inObject = append(inObject, false)
				expectKey = false
			default:
				inObject = inObject[:len(inObject)-1]
				expectKey = len(inObject) > 0 && inObject[len(inObject)-1]
			}
			continue
		case string:
			if expectKey {
				key, expectKey = v, false
				continue
			}
			if nameKeys[key] && values.HasDirectionOverride(v) {
				return Validation(key, "direction_override",
					"a name cannot hold a character that reverses the order it is shown in; remove it and send the name again")
			}
		}
		key = ""
		expectKey = len(inObject) > 0 && inObject[len(inObject)-1]
	}
}
