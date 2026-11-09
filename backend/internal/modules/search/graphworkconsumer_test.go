// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

func TestOnlyAssociationUpdatesCanWaitForAHistoryDrain(t *testing.T) {
	const target = `{"entity_type":"contact","entity_id":"00000000-0000-0000-0000-000000000001"}`
	for _, test := range []struct {
		name, payload string
		want          bool
	}{
		{"workspace opening", `{"changed_fields":{"audience":"workspace"}}`, true},
		{"workspace with unknown change", `{"changed_fields":{"audience":"workspace","unknown":true}}`, false},
		{"association only", `{"changed_fields":{"relinked":` + target + `}}`, true},
		{"privacy change", `{"changed_fields":{"relinked":` + target + `,"audience":"participants"}}`, false},
		{"unknown change", `{"changed_fields":{"relinked":` + target + `,"future_privacy_rule":true}}`, false},
		{"no target", `{"changed_fields":{"relinked":null}}`, false},
		{"no identifier", `{"changed_fields":{"relinked":{"entity_type":"contact"}}}`, false},
		{"malformed", `{`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := graphArrivalCanWait(events.Envelope{Type: "activity.updated", Payload: json.RawMessage(test.payload)})
			if got != test.want {
				t.Fatalf("can wait=%t want=%t", got, test.want)
			}
		})
	}
}
