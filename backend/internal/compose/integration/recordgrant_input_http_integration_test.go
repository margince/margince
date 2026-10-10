// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type grantRefusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details struct {
		Errors []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"details"`
}

func (g grantRefusal) refusedField(field, code string) bool {
	for _, refused := range g.Details.Errors {
		if refused.Field == field && refused.Code == code {
			return true
		}
	}
	return false
}

type wireGrant struct {
	ID      string `json:"id"`
	Version *int64 `json:"version"`
}

func seedGrantFixture(t *testing.T, e *apptest.AppEnv) (contact, subject string) {
	t.Helper()
	contact = createAndID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Shared Contact"})
	var users struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/users", nil, nil, &users); status != http.StatusOK || len(users.Data) == 0 {
		t.Fatalf("list users → %d (%d users)", status, len(users.Data))
	}
	return contact, users.Data[0].ID
}

// Each bad field of the grant body answers 422 naming that field, in the
// contract's own vocabulary. It is never a server error or a sentence about
// agent scopes.
func TestARecordGrantRefusesEachBadFieldByItsOwnName(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Grant Input", "admin@grantinput.test", "Admin")
	contact, subject := seedGrantFixture(t, e)
	valid := func() AnyMap {
		return AnyMap{"record_type": "contact", "record_id": contact, "subject_type": "user", "subject_id": subject, "access": "read"}
	}

	cases := []struct {
		field, value, code string
	}{
		{"record_type", "task", "invalid"},
		{"subject_type", "robot", "invalid"},
		{"access", "admin", "invalid"},
		{"expires_at", "9999-12-31T23:59:59Z", "out_of_range"},
		{"expires_at", "9999-12-31T17:00:00Z", "out_of_range"},
	}
	for _, tc := range cases {
		t.Run(tc.field+"="+tc.value, func(t *testing.T) {
			body := valid()
			body[tc.field] = tc.value
			var problem grantRefusal
			status := e.Call(t, http.MethodPost, "/v1/record-grants", body, nil, &problem)
			if status != http.StatusUnprocessableEntity || !problem.refusedField(tc.field, tc.code) {
				t.Fatalf("POST %s=%s → %d %+v, want 422 %s on %s", tc.field, tc.value, status, problem, tc.code, tc.field)
			}
			for _, refused := range problem.Details.Errors {
				if strings.Contains(refused.Message, "draft") {
					t.Fatalf("refusal speaks of agent scopes: %q", refused.Message)
				}
			}
		})
	}

	body := valid()
	body["expires_at"] = "9999-12-30T23:59:59Z"
	if status := e.Call(t, http.MethodPost, "/v1/record-grants", body, nil, nil); status != http.StatusCreated {
		t.Fatalf("grant expiring at the last accepted instant → %d, want 201", status)
	}
	var page struct {
		Data []wireGrant `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/record-grants?record_type=contact&record_id="+contact, nil, nil, &page); status != http.StatusOK || len(page.Data) != 1 {
		t.Fatalf("list grants → %d with %d rows, want 200 with the grant just written", status, len(page.Data))
	}
}

// A grant carries its version, a re-assert moves it, and a revoke behind a
// stale If-Match changes nothing.
func TestARecordGrantRevokeHonoursIfMatch(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Grant Version", "admin@grantversion.test", "Admin")
	contact, subject := seedGrantFixture(t, e)
	body := AnyMap{"record_type": "contact", "record_id": contact, "subject_type": "user", "subject_id": subject, "access": "read"}

	var first, second wireGrant
	if status := e.Call(t, http.MethodPost, "/v1/record-grants", body, nil, &first); status != http.StatusCreated || first.Version == nil {
		t.Fatalf("first share → %d %+v, want 201 with a version", status, first)
	}
	body["reason"] = "covering the account"
	if status := e.Call(t, http.MethodPost, "/v1/record-grants", body, nil, &second); status != http.StatusCreated || second.Version == nil {
		t.Fatalf("re-assert → %d %+v, want 201 with a version", status, second)
	}
	if *second.Version != *first.Version+1 {
		t.Fatalf("re-assert version %d after %d, want one more", *second.Version, *first.Version)
	}

	stale := map[string]string{"If-Match": strconv.FormatInt(*first.Version, 10)}
	var problem grantRefusal
	if status := e.Call(t, http.MethodDelete, "/v1/record-grants/"+second.ID, nil, stale, &problem); status != http.StatusConflict || problem.Code != "version_skew" {
		t.Fatalf("revoke behind a stale If-Match → %d %+v, want 409 version_skew", status, problem)
	}
	var page struct {
		Data []wireGrant `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/record-grants?record_type=contact&record_id="+contact, nil, nil, &page); status != http.StatusOK || len(page.Data) != 1 {
		t.Fatalf("after a refused revoke → %d with %d grants, want the grant still there", status, len(page.Data))
	}

	current := map[string]string{"If-Match": strconv.FormatInt(*second.Version, 10)}
	if status := e.Call(t, http.MethodDelete, "/v1/record-grants/"+second.ID, nil, current, nil); status != http.StatusNoContent {
		t.Fatalf("revoke behind the current If-Match → %d, want 204", status)
	}
}
