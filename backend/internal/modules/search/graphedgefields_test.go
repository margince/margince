// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

func TestTheFoldsReadTheColumnsTheirSQLNames(t *testing.T) {
	for _, column := range []string{"archived_at", "audience", "occurred_at", "kind", "thread_key", "call_outcome", "meeting_status", "direction", "captured_by", participantRows} {
		if !foldReadsActivityColumns[column] {
			t.Errorf("column %q is missing from what the folds read: %v", column, foldReadsActivityColumns)
		}
	}
	for _, column := range []string{"subject", "body", "assignee_id", "is_done"} {
		if foldReadsActivityColumns[column] {
			t.Errorf("column %q is read by a fold, so the skip rules for it need a second look", column)
		}
	}
}

func TestEveryChangedFieldOfAnActivityUpdateIsPlaced(t *testing.T) {
	fields := reflect.TypeOf(crmcontracts.PublicEventActivityChangedFields{})
	seen := map[string]bool{}
	for i := range fields.NumField() {
		key, _, _ := strings.Cut(fields.Field(i).Tag.Get("json"), ",")
		seen[key] = true
		if _, placed := updatedFieldWrites[key]; !placed {
			t.Errorf("changed field %q is not in updatedFieldWrites; say which columns it writes", key)
		}
	}
	for key := range updatedFieldWrites {
		if !seen[key] {
			t.Errorf("updatedFieldWrites names %q, which the event no longer carries", key)
		}
	}
}

func TestOnlyAnUpdateTheFoldsCannotSeeSkipsTheRefold(t *testing.T) {
	for _, test := range []struct {
		name, payload string
		want          bool
	}{
		{"no payload", ``, true},
		{"no field list", `{}`, true},
		{"empty field list", `{"changed_fields":{}}`, true},
		{"malformed", `{`, true},
		{"unknown field", `{"changed_fields":{"future_field":true}}`, true},
		{"body only", `{"changed_fields":{"body":true}}`, false},
		{"reassigned", `{"changed_fields":{"assignee_id":"x","automatic_task_repaired":true}}`, false},
		{"calendar method", `{"changed_fields":{"calendar_method":"REPLY"}}`, true},
		{"agenda and recap", `{"changed_fields":{"agenda_changed":true,"meeting_recap_became_ready":true}}`, false},
		{"title and due date", `{"changed_fields":{"subject":"x","due_at":"2026-01-01T00:00:00Z","is_done":true}}`, false},
		{"harmless beside a relevant one", `{"changed_fields":{"body":true,"occurred_at":"2026-01-01T00:00:00Z"}}`, true},
		{"audience", `{"changed_fields":{"audience":"participants"}}`, true},
		{"relink", `{"changed_fields":{"relinked":{"entity_type":"deal","entity_id":"x"}}}`, true},
		{"direction", `{"changed_fields":{"direction":"inbound"}}`, true},
		{"meeting cancelled", `{"changed_fields":{"meeting_status":"cancelled"}}`, true},
		{"restored", `{"changed_fields":{"restored":true}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := events.Envelope{Type: "activity.updated", Payload: json.RawMessage(test.payload)}
			if got := refoldsActivity(env); got != test.want {
				t.Fatalf("refolds=%t want=%t", got, test.want)
			}
		})
	}
	for _, verb := range []string{"activity.captured", "activity.archived", "retention.applied"} {
		if !refoldsActivity(events.Envelope{Type: verb, Payload: json.RawMessage(`{"changed_fields":{"body":true}}`)}) {
			t.Errorf("%s must always refold", verb)
		}
	}
}

func TestAnUnchangedGuardComparesEveryFoldedFact(t *testing.T) {
	if unchangedEdgeGuard(true, interactionEdgeFacts) != "" {
		t.Fatal("a fold that advances the snapshot must write")
	}
	guard := unchangedEdgeGuard(false, contactEdgeFacts)
	for _, want := range []string{"(e.last_at, e.count_90d, e.count_total)", "IS DISTINCT FROM (EXCLUDED.last_at, EXCLUDED.count_90d, EXCLUDED.count_total)"} {
		if !strings.Contains(guard, want) {
			t.Errorf("guard %q lacks %q", guard, want)
		}
	}
}
