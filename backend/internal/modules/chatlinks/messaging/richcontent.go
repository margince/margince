// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"encoding/json"
	"strings"
)

// RichContent reads visible card and block text without treating actions or file contents as prose.
func RichContent(raw json.RawMessage) (string, bool) {
	remaining := 1024
	text, complete := readCardContent(raw, 0, &remaining)
	return strings.TrimSpace(text), complete
}

func readCardContent(raw json.RawMessage, depth int, remaining *int) (string, bool) {
	*remaining--
	if depth > 12 || *remaining < 0 {
		return "", false
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil && list != nil {
		parts := []string{}
		complete := true
		for _, item := range list {
			text, read := readCardContent(item, depth+1, remaining)
			parts = append(parts, text)
			complete = complete && read
		}
		return strings.Join(parts, "\n"), complete
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return "", false
	}
	parts := []string{}
	complete := true
	for _, key := range []string{"pretext", "title", "subtitle", "text", "value", "body", "blocks", "sections", "columns", "items", "elements", "facts", "fields"} {
		if item, found := object[key]; found {
			text, read := readCardContent(item, depth+1, remaining)
			parts = append(parts, text)
			complete = complete && read
		}
	}
	return strings.Join(parts, "\n"), complete && len(parts) > 0
}
