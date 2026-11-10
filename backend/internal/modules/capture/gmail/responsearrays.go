// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"encoding/json"
	"errors"
)

// Leave room above Gmail's user-label limit while bounding decoded slice overhead.
// https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.labels
const maxMessageLabels = 16384

var (
	errResponseArrayLimit = errors.New("gmail: response has too many array entries")
	errMetadataNeedsRaw   = errors.New("gmail: message metadata needs a full download")
)

type boundedJSONList[T any] []T

type messageLabels = boundedJSONList[string]

func (list *boundedJSONList[T]) UnmarshalJSON(raw []byte) error {
	quoted, escaped, depth, separators := false, false, 0, 0
	for _, ch := range raw {
		if escaped {
			escaped = false
			continue
		}
		if quoted && ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			quoted = !quoted
		}
		if quoted {
			continue
		}
		switch ch {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 1 {
				separators++
				if separators >= maxMessageLabels {
					return errResponseArrayLimit
				}
			}
		}
	}
	return json.Unmarshal(raw, (*[]T)(list))
}
