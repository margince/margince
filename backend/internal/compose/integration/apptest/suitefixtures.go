// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"testing"
)

// Fixtures that internal/compose/integration and a suite package split out of it
// both call. They sit here because they take an AppEnv, and a split suite can
// import nothing from that package's _test.go files.

// CreateCustomField defines a field and answers its cf_* key.
func CreateCustomField(t *testing.T, e *AppEnv, body map[string]any) string {
	t.Helper()
	var field struct {
		ColumnName string `json:"column_name"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/custom-fields", body, nil, &field); status != http.StatusCreated {
		t.Fatalf("POST /v1/custom-fields → %d, want 201", status)
	}
	if field.ColumnName == "" {
		t.Fatal("the created field carries no column_name")
	}
	return field.ColumnName
}
