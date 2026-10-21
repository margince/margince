// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

// A record write stores every custom-field key it was sent or answers a 422
// naming the one it could not. A key no active field names, a retired field's
// key and a value the column cannot hold are each refused, on a create and on
// an update, where they used to be left out of a write that answered success.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func refusalNames(problem customFieldProblem, field, code string) bool {
	for _, refusal := range problem.Details.Errors {
		if refusal.Field == field && refusal.Code == code {
			return true
		}
	}
	return false
}

func TestARecordWriteNeverDropsACustomFieldKey(t *testing.T) {
	e := schemaWiredEnv(t)
	status, field, problem := createCustomField(t, e, integration.AnyMap{
		"object": "contact", "label": "Nickname", "type": "text", "source": "manual",
	})
	if status != http.StatusCreated {
		t.Fatalf("create contact field status = %d: %+v", status, problem)
	}
	status, retired, problem := createCustomField(t, e, integration.AnyMap{
		"object": "contact", "label": "Old tag", "type": "text", "source": "manual",
	})
	if status != http.StatusCreated {
		t.Fatalf("create contact field status = %d: %+v", status, problem)
	}
	if status := e.Call(t, "POST", "/v1/custom-fields/"+retired.ID+"/retire", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("retire status = %d", status)
	}

	_, contact := createWithCF(t, e, "/v1/contacts", integration.AnyMap{
		"full_name": "Ada Lovelace", "source": "manual", field.ColumnName: "Countess",
	})

	refused := []struct {
		name, key, code string
		value           json.RawMessage
	}{
		{"a key no field names", "cf_nmae", "unknown_field", json.RawMessage(`"x"`)},
		{"a retired field's key", retired.ColumnName, "unknown_field", json.RawMessage(`"x"`)},
		{"a value the column cannot hold", field.ColumnName, "invalid_value", json.RawMessage(`5`)},
	}
	for _, tc := range refused {
		t.Run("create "+tc.name, func(t *testing.T) {
			var problem customFieldProblem
			status := e.Call(t, "POST", "/v1/contacts", integration.AnyMap{
				"full_name": "Refused", "source": "manual", tc.key: tc.value,
			}, nil, &problem)
			if status != http.StatusUnprocessableEntity || !refusalNames(problem, tc.key, tc.code) {
				t.Fatalf("create → %d %+v, want 422 %s on %s", status, problem, tc.code, tc.key)
			}
		})
		t.Run("update "+tc.name, func(t *testing.T) {
			var problem customFieldProblem
			status := e.Call(t, "PATCH", "/v1/contacts/"+contact, integration.AnyMap{tc.key: tc.value},
				map[string]string{"If-Match": "1"}, &problem)
			if status != http.StatusUnprocessableEntity || !refusalNames(problem, tc.key, tc.code) {
				t.Fatalf("update → %d %+v, want 422 %s on %s", status, problem, tc.code, tc.key)
			}
		})
	}

	var read integration.AnyMap
	if status := e.Call(t, "GET", "/v1/contacts/"+contact, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("read status = %d", status)
	}
	assertWireCF(t, read, field.ColumnName, "Countess")
}
