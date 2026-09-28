// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestMultiselectPreservesChoicesAndRefusesStrandedValues(t *testing.T) {
	e := schemaWiredEnv(t)
	status, field, problem := createCustomField(t, e, integration.AnyMap{
		"object": "company", "label": "Capabilities", "type": "multiselect", "options": []string{"Fit, scope", "C++", "Other"}, "source": "manual",
	})
	if status != http.StatusCreated {
		t.Fatalf("create field: %d %v", status, problem)
	}
	choices := []any{"Fit, scope", "C++"}
	created, id := createWithCF(t, e, "/v1/companies", integration.AnyMap{"display_name": "Multiple choices", "source": "manual", field.ColumnName: choices})
	if !reflect.DeepEqual(created[field.ColumnName], choices) {
		t.Fatalf("lost choices: %v", created)
	}
	var result integration.AnyMap
	if status := e.Call(t, "PATCH", "/v1/companies/"+id, integration.AnyMap{field.ColumnName: []string{"Unknown"}}, nil, &result); status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid option status %d: %v", status, result)
	}
	if status := e.Call(t, "PATCH", "/v1/custom-fields/"+field.ID+"/options", integration.AnyMap{"options": []string{"Other"}}, nil, &result); status != http.StatusConflict {
		t.Fatalf("stranding options status %d: %v", status, result)
	}
	if status := e.Call(t, "GET", "/v1/companies/"+id, nil, nil, &result); status != http.StatusOK {
		t.Fatal(status)
	}
	if !reflect.DeepEqual(result[field.ColumnName], choices) {
		t.Fatalf("refused write changed choices: %v", result)
	}
	if status := e.Call(t, "PATCH", "/v1/companies/"+id, integration.AnyMap{field.ColumnName: []string{}}, nil, &result); status != http.StatusOK {
		t.Fatalf("clear: %d %v", status, result)
	}
	if got, ok := result[field.ColumnName].([]any); !ok || len(got) != 0 {
		t.Fatalf("clear failed: %v", result)
	}
}
