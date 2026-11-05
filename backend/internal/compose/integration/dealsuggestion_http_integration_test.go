// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Deal Scout routes over HTTP: an acceptance retried under its
// Idempotency-Key answers the first result and opens one deal, and a second
// decision under a new key is refused as a conflict.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// seedScoutedCompany builds a company with a buyer request over the API, runs
// one scout pass, and answers the suggestion's id.
func seedScoutedCompany(t *testing.T, e *apptest.AppEnv) (companyID, suggestionID string) {
	t.Helper()
	return apptest.SeedScoutedNamed(t, e, "Acme GmbH", "Dana Buyer")
}

func TestAnAcceptanceRetriedUnderItsKeyOpensOneDeal(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID, suggestionID := seedScoutedCompany(t, e)
	path := "/v1/deal-suggestions/" + suggestionID + "/accept"
	body := AnyMap{"name": "Acme platform"}

	var first, replay AnyMap
	if status := e.Call(t, "POST", path, body, map[string]string{"Idempotency-Key": "accept-1"}, &first); status != http.StatusOK {
		t.Fatalf("accept = %d %v", status, first)
	}
	if status := e.Call(t, "POST", path, body, map[string]string{"Idempotency-Key": "accept-1"}, &replay); status != http.StatusOK {
		t.Fatalf("replayed accept = %d %v", status, replay)
	}
	if first["deal_id"] == nil || replay["deal_id"] != first["deal_id"] {
		t.Fatalf("the replay answered deal %v, want the first answer's %v", replay["deal_id"], first["deal_id"])
	}
	var deals struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deals?company_id="+companyID, nil, nil, &deals); status != http.StatusOK || len(deals.Data) != 1 {
		t.Fatalf("the company holds %d deals after a retried accept, want exactly one", len(deals.Data))
	}

	var refused AnyMap
	if status := e.Call(t, "POST", path, body, map[string]string{"Idempotency-Key": "accept-2"}, &refused); status != http.StatusConflict {
		t.Fatalf("a second accept under a new key = %d %v, want 409", status, refused)
	}
	if refused["code"] != "suggestion_decided" {
		t.Fatalf("the conflict's code = %v, want suggestion_decided", refused["code"])
	}
	if status := e.Call(t, "POST", "/v1/deal-suggestions/"+suggestionID+"/dismiss", nil,
		map[string]string{"Idempotency-Key": "dismiss-1"}, nil); status != http.StatusConflict {
		t.Fatalf("dismissing an accepted suggestion = %d, want 409", status)
	}
}
