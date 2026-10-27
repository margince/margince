// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"strconv"
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

// AssertSaveChangesNothing sends a save at the record's current version and
// checks the version, the audit rows and the outbox all stayed where they were.
func AssertSaveChangesNothing(
	t *testing.T, e *AppEnv, method, path string, body map[string]any, entityID string, version int64,
) {
	t.Helper()
	count := func(query string) int {
		var n int
		if err := e.Owner.QueryRow(t.Context(), query, entityID).Scan(&n); err != nil {
			t.Fatalf("counting for %s: %v", entityID, err)
		}
		return n
	}
	const audits = `SELECT count(*) FROM audit_log WHERE entity_id = $1`
	const events = `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%'`
	auditsBefore, eventsBefore := count(audits), count(events)

	var answer struct {
		Version int64 `json:"version"`
	}
	headers := map[string]string{"If-Match": strconv.FormatInt(version, 10)}
	if status := e.Call(t, method, path, body, headers, &answer); status != http.StatusOK {
		t.Fatalf("%s %s with the stored values -> %d, want 200", method, path, status)
	}
	if answer.Version != version {
		t.Errorf("%s %s with the stored values answered version %d, want the unchanged %d",
			method, path, answer.Version, version)
	}
	if got := count(audits); got != auditsBefore {
		t.Errorf("%s %s with the stored values wrote %d audit row(s), want none", method, path, got-auditsBefore)
	}
	if got := count(events); got != eventsBefore {
		t.Errorf("%s %s with the stored values emitted %d event(s), want none", method, path, got-eventsBefore)
	}
}
