// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestRepairDigestUsesTheExecutedTypedScope(t *testing.T) {
	base := datatask.ReviewRequest{RuleVersion: "test", Limit: 1, AsOf: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	scopes := []string{
		`{"tasks":[{"id":"00000000-0000-0000-0000-000000000001","version":1,"source":"verdict","action":"reclassify_message","reason":"classification_changed","before_verdict":"asks_us","after_verdict":"informs_us"}]}`,
		` { "tasks" : [{"after_verdict":"informs_us","before_verdict":"asks_us","reason":"classification_changed","action":"reclassify_message","source":"verdict","version":1,"id":"00000000-0000-0000-0000-000000000001"}] } `,
		`{"tasks":[{"id":"00000000-0000-0000-0000-000000000001","version":1,"source":"verdict","action":"reclassify_message","reason":"classification_changed","before_verdict":"asks_us","after_verdict":"asks_us","after_verdict":"informs_us"}]}`,
	}
	var expected string
	for _, scope := range scopes {
		base.Scope = json.RawMessage(scope)
		got, err := homeRepairDigest(base)
		if err != nil {
			t.Fatal(err)
		}
		if expected == "" {
			expected = got
		}
		if got != expected {
			t.Fatal("equivalent executed scopes produce different digests")
		}
	}
	base.Scope = json.RawMessage(`{"tasks":[{"id":"00000000-0000-0000-0000-000000000001","version":1,"source":"verdict","action":"reclassify_message","reason":"classification_changed","before_verdict":"asks_us","after_verdict":"informs_us","after_verdict":"asks_us"}]}`)
	changed, err := homeRepairDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	if changed == expected {
		t.Fatal("different executed outcome retained the digest")
	}
}
