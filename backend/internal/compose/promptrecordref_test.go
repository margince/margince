// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// promptRecordRef is the ref a request's JSON payload shows for the record
// whose field holds value, so a fake can cite a record it seeded by what the
// test knows of it. It fails the test when no shown record matches.
func promptRecordRef(t *testing.T, req model.Request, field, value string) string {
	t.Helper()
	for _, message := range req.Messages {
		start, end := strings.Index(message.Content, "{"), strings.LastIndex(message.Content, "}")
		if start < 0 || end < start {
			continue
		}
		var payload any
		if err := json.Unmarshal([]byte(message.Content[start:end+1]), &payload); err != nil {
			continue
		}
		if ref, ok := refOfRecord(payload, field, value); ok {
			return ref
		}
	}
	t.Fatalf("the request shows no record whose %s is %q", field, value)
	return ""
}

func refOfRecord(node any, field, value string) (string, bool) {
	switch v := node.(type) {
	case map[string]any:
		if v[field] == value {
			ref, ok := v["id"].(string)
			return ref, ok
		}
		for _, child := range v {
			if ref, ok := refOfRecord(child, field, value); ok {
				return ref, true
			}
		}
	case []any:
		for _, child := range v {
			if ref, ok := refOfRecord(child, field, value); ok {
				return ref, true
			}
		}
	}
	return "", false
}

func TestPromptRecordRefFindsTheRecordByAField(t *testing.T) {
	req := model.Request{Messages: []model.Message{{
		Content: `<span>{"deal":{"id":"d1"},"timeline":[{"id":"a1","subject":"Kickoff"},{"id":"a2","subject":"Scope"}]}</span>`,
	}}}
	if got := promptRecordRef(t, req, "subject", "Scope"); got != "a2" {
		t.Errorf("promptRecordRef = %q, want a2", got)
	}
}
