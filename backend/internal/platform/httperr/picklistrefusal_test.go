// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func checkRefusal(table, constraint string) error {
	return fmt.Errorf("writing: %w", &pgconn.PgError{Code: "23514", TableName: table, ConstraintName: constraint})
}

func TestAPicklistRefusalNamesTheField(t *testing.T) {
	picklists := map[string]string{"deal_priority_check": "priority"}
	for constraint, field := range map[string]string{
		"deal_priority_check":                  "priority",
		storekit.PicklistConstraint("cf_tier"): "cf_tier",
	} {
		rec := httptest.NewRecorder()
		if !WritePicklistRefusal(rec, httptest.NewRequest(http.MethodPatch, "/v1/deals/x", nil), checkRefusal("deal", constraint), "deal", picklists) {
			t.Fatalf("%s was not answered as a picklist refusal", constraint)
		}
		var body struct {
			Detail  string `json:"detail"`
			Details struct {
				Fields []FieldError `json:"errors"`
			} `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusUnprocessableEntity || len(body.Details.Fields) != 1 || body.Details.Fields[0].Field != field {
			t.Errorf("%s answered %d %s, want 422 naming %q", constraint, rec.Code, rec.Body.String(), field)
		}
		if !strings.Contains(body.Detail, "check the picklist options") || strings.Contains(rec.Body.String(), "_check") {
			t.Errorf("%s answered %q: want the picklist sentence and no constraint name", constraint, body.Detail)
		}
	}
}

func TestEveryOtherCheckIsLeftToTheSharedNet(t *testing.T) {
	picklists := map[string]string{"deal_priority_check": "priority"}
	for _, tc := range []struct{ table, constraint string }{
		{"deal", "deal_description_bounded"},
		{"deal", storekit.TextMaxConstraint("cf_notes", 20000)},
		{"deal", "member_work_reference_retired"},
		{storekit.TableAudit, "audit_log_action_check"},
		{"activity", activityRestrictedImmutable},
		{"project", "deal_priority_check"},
		{storekit.TableOutbox, storekit.PicklistConstraint("cf_secret")},
	} {
		rec := httptest.NewRecorder()
		if WritePicklistRefusal(rec, httptest.NewRequest(http.MethodPatch, "/v1/deals/x", nil), checkRefusal(tc.table, tc.constraint), "deal", picklists) {
			t.Errorf("%s was answered as a picklist refusal; httperr.Write must answer it", tc.constraint)
		}
	}
}

// A constraint the caller's input broke is a refusal: its cause is a warning.
// A constraint on a row the caller never wrote stays an error.
func TestAWithheldCauseIsLoggedAtTheLevelOfWhoseFaultItIs(t *testing.T) {
	for _, tc := range []struct {
		name, table, constraint, level string
	}{
		{"a value outside its field", "deal", "deal_description_bounded", "level=WARN"},
		{"a retired member", "deal", "member_work_reference_retired", "level=WARN"},
		{"an audit row", storekit.TableAudit, "audit_log_action_check", "level=ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(restore) })

			Write(httptest.NewRecorder(), httptest.NewRequest(http.MethodPatch, "/v1/deals/x", nil), checkRefusal(tc.table, tc.constraint))
			if !strings.Contains(logged.String(), tc.level) {
				t.Errorf("logged %q, want %s", logged.String(), tc.level)
			}
		})
	}
}
