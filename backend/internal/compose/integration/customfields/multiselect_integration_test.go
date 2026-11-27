// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestMultiselectPreservesChoicesAndRefusesUnknownOnes(t *testing.T) {
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

func TestMultiselectListFiltersCompareTheSelectedSetWithoutDependingOnOrder(t *testing.T) {
	for _, object := range []string{"company", "lead"} {
		t.Run(object, func(t *testing.T) {
			doors := apptest.SetupTwoDoors(t, "multiselect-"+object, []compose.Option{compose.WithSchemaPool(integration.SchemaPool(t))}, "read")
			e := doors.E
			status, field, problem := createCustomField(t, e, integration.AnyMap{"object": object, "label": "Capabilities", "type": "multiselect", "options": []string{"Fit, scope", "C++", "Other"}, "source": "manual"})
			if status != http.StatusCreated {
				t.Fatalf("create field: %d %v", status, problem)
			}
			path := "/v1/companies"
			body := integration.AnyMap{"display_name": "Both choices", "source": "manual", field.ColumnName: []string{"Fit, scope", "C++"}}
			if object == "lead" {
				path = "/v1/leads"
				delete(body, "display_name")
				body["full_name"] = "Both choices"
			}
			_, id := createWithCF(t, e, path, body)
			body[field.ColumnName] = []string{"C++"}
			if object == "lead" {
				body["full_name"] = "One choice"
			} else {
				body["display_name"] = "One choice"
			}
			createWithCF(t, e, path, body)
			for _, sort := range []string{"", "-created_at"} {
				query := url.Values{field.ColumnName: {`["C++","Fit, scope"]`}}
				if sort != "" {
					query.Set("sort", sort)
				}
				var result struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				if status := e.Call(t, "GET", path+"?"+query.Encode(), nil, nil, &result); status != http.StatusOK {
					t.Fatalf("filter: %d", status)
				}
				if len(result.Data) != 1 || result.Data[0].ID != id {
					t.Fatalf("filtered %s sort %q=%+v", object, sort, result.Data)
				}
			}
			answer := doors.MCP.CallOK(t, "list_records", integration.AnyMap{"record_type": object, "filters": integration.AnyMap{field.ColumnName: `["C++","Fit, scope"]`}})
			var listed struct {
				Records []struct {
					ID string `json:"id"`
				} `json:"records"`
			}
			answer.JSON(t, &listed)
			if len(listed.Records) != 1 || listed.Records[0].ID != id {
				t.Fatalf("MCP %s filter=%+v", object, listed)
			}
		})
	}
}
