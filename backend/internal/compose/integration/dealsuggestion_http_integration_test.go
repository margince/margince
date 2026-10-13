// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Deal Scout routes over HTTP: an acceptance retried under its
// Idempotency-Key answers the first result and opens one deal, and a second
// decision under a new key is refused as a conflict.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedScoutedCompany builds a company with a held meeting over the API, runs
// one scout pass, and answers the suggestion's id.
func seedScoutedCompany(t *testing.T, e *apptest.AppEnv) (companyID, suggestionID string) {
	t.Helper()
	return seedScoutedNamed(t, e, "Acme GmbH", "Dana Buyer")
}

// seedScoutedNamed is seedScoutedCompany for a company and contact of the
// caller's naming, so one test can raise several suggestions.
func seedScoutedNamed(t *testing.T, e *apptest.AppEnv, companyName, contactName string) (companyID, suggestionID string) {
	t.Helper()
	var company, contact, meeting AnyMap
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": companyName}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company = %d %v", status, company)
	}
	companyID, _ = company["id"].(string)
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": contactName, "source": "manual"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact = %d %v", status, contact)
	}
	contactID, _ := contact["id"].(string)
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": contactID, "company_id": companyID, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create employment = %d", status)
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Scoping workshop", "meeting_status": "held",
		"occurred_at": time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339),
		"links":       []AnyMap{{"entity_type": "contact", "entity_id": contactID}},
	}, nil, &meeting); status != http.StatusCreated {
		t.Fatalf("log meeting = %d %v", status, meeting)
	}

	ws := apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool)
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), "agent:deal-scout")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := compose.RunDealScout(ctx, tx, time.Now())
		return err
	}); err != nil {
		t.Fatalf("the scout pass: %v", err)
	}
	var list struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-suggestions?company_id="+companyID, nil, nil, &list); status != http.StatusOK || len(list.Data) != 1 {
		t.Fatalf("list suggestions = %d with %d rows, want one", status, len(list.Data))
	}
	suggestionID, _ = list.Data[0]["id"].(string)
	return companyID, suggestionID
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
