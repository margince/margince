// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package faulttest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// AssertCheckViolationsReachTheSharedNet asserts that a module's error writer
// answers a custom picklist with the picklist refusal and leaves every other
// CHECK to httperr's constraint net. table is a table the module writes.
func AssertCheckViolationsReachTheSharedNet(t *testing.T, table string, write func(http.ResponseWriter, *http.Request, error)) {
	t.Helper()
	for _, tc := range []struct {
		name, table, constraint string
		status                  int
		code, field             string
	}{
		{"a custom picklist", table, storekit.PicklistConstraint("cf_tier"), http.StatusUnprocessableEntity, "validation_error", "cf_tier"},
		{"an overlong custom text", table, storekit.TextMaxConstraint("cf_briefing", 20000), http.StatusUnprocessableEntity, "validation_error", "cf_briefing"},
		{"a retired member", table, "member_work_reference_retired", http.StatusConflict, "conflict", ""},
		{"a retention hold", "activity", "activity_restricted_immutable", http.StatusLocked, "locked", ""},
		{"an audit row", storekit.TableAudit, "audit_log_action_check", http.StatusInternalServerError, "internal", ""},
		{"an outbox row", storekit.TableOutbox, "event_outbox_stream_check", http.StatusInternalServerError, "internal", ""},
		{"a custom picklist name on an outbox row", storekit.TableOutbox, storekit.PicklistConstraint("cf_secret"), http.StatusInternalServerError, "internal", ""},
		{"a custom text bound name on an outbox row", storekit.TableOutbox, storekit.TextMaxConstraint("cf_secret", 20000), http.StatusInternalServerError, "internal", ""},
		{"an unnamed rule", table, table + "_unnamed_rule", http.StatusUnprocessableEntity, "value_not_allowed", ""},
	} {
		rec := httptest.NewRecorder()
		cause := &pgconn.PgError{Code: storekit.CheckViolationCode, TableName: tc.table, ConstraintName: tc.constraint}
		write(rec, httptest.NewRequest(http.MethodPatch, "/v1/records/x", nil), fmt.Errorf("writing: %w", cause))
		var body struct {
			Code    string `json:"code"`
			Details struct {
				Errors []struct {
					Field string `json:"field"`
				} `json:"errors"`
			} `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decoding %q: %v", tc.name, rec.Body.String(), err)
		}
		if rec.Code != tc.status || body.Code != tc.code {
			t.Errorf("%s answered %d %s, want %d %s", tc.name, rec.Code, body.Code, tc.status, tc.code)
		}
		if tc.field != "" && (len(body.Details.Errors) != 1 || body.Details.Errors[0].Field != tc.field) {
			t.Errorf("%s answered %s, want it to name %s", tc.name, rec.Body.String(), tc.field)
		}
		if strings.Contains(rec.Body.String(), tc.constraint) {
			t.Errorf("%s put the constraint name in the body: %s", tc.name, rec.Body.String())
		}
	}
}
