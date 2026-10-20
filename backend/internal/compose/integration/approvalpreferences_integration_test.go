// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestPersonalApprovalPreferencesThroughAuthenticatedRoutes(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	const path = "/v1/me/approval-preferences"
	for _, muted := range []bool{false, true, false} {
		var result crmcontracts.ApprovalPreferences
		body := crmcontracts.ApprovalPreference{Kind: "close_date_correction", Muted: muted}
		if status := e.Call(t, http.MethodPatch, path, body, nil, &result); status != http.StatusOK {
			t.Fatalf("update preference: status=%d", status)
		}
		if status := e.Call(t, http.MethodGet, path, nil, nil, &result); status != http.StatusOK {
			t.Fatalf("read preference: status=%d", status)
		}
		found := false
		for _, preference := range result.Data {
			if preference.Kind == "close_date_correction" {
				found = true
				if preference.Muted != muted {
					t.Fatalf("muted=%t want=%t", preference.Muted, muted)
				}
			}
		}
		if !found {
			t.Fatal("personal preference missing from authenticated read")
		}
	}
	bearer := apptest.PassportBearer(t, e, "preference-reader", "read", "write")
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		body := crmcontracts.ApprovalPreference{Kind: "close_date_correction", Muted: true}
		var problem struct {
			Code string `json:"code"`
		}
		if status := e.Call(t, method, path, body, bearer, &problem); status != http.StatusForbidden {
			t.Fatalf("passport %s preference: status=%d", method, status)
		}
		if problem.Code != "permission_denied" {
			t.Fatalf("passport denial=%q", problem.Code)
		}
	}
}
