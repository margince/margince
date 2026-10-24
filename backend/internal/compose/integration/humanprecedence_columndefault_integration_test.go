// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A column default is not something a human typed. A company created by a
// human without a lifecycle holds 'unknown' only because the column says so.
// Automatic work moving it on overwrites nobody, so the write applies directly.
// A value a human did type holds automatic work back: it may not stage an
// approval, so the change is refused and the value stays.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAutomaticWorkChangesAFieldThatStillHoldsItsColumnDefault(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Defaulted Lifecycle GmbH"})
	if lifecycle, held := agentPatchCompany(t, e, companyID, "lifecycle", "prospect"); held || lifecycle != "prospect" {
		t.Fatalf("automatic work moved a defaulted lifecycle → lifecycle %q, held back %v; want it applied",
			lifecycle, held)
	}

	// Once a human sets the field, it is theirs.
	if status := e.Call(t, "PATCH", "/v1/companies/"+companyID, AnyMap{"lifecycle": "customer"}, nil, nil); status != http.StatusOK {
		t.Fatalf("human sets lifecycle → %d", status)
	}
	if lifecycle, held := agentPatchCompany(t, e, companyID, "lifecycle", "former_customer"); !held || lifecycle != "customer" {
		t.Errorf("automatic work overwrote a lifecycle a human set → lifecycle %q, held back %v; want it refused and unchanged",
			lifecycle, held)
	}
}

func TestAutomaticWorkIsRefusedAValueTypedAtCreate(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createdID(t, e, "/v1/companies", AnyMap{
		"source":       "manual",
		"display_name": "Typed Industry GmbH", "industry": "Logistics",
	})
	if industry, held := agentPatchCompany(t, e, companyID, "industry", "Retail"); !held || industry != "logistics_transport" {
		t.Errorf("automatic work overwrote the industry the human typed at create → industry %q, held back %v", industry, held)
	}
}

// Who can see a record is never a default automatic work moves: narrowing a
// human's company to its owner hides it from every colleague.
func TestAutomaticWorkIsRefusedNarrowingWhoSeesAHumanCreatedRecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Defaulted Visibility GmbH"})
	if visibility, held := agentPatchCompany(t, e, companyID, "visibility", "owner"); !held || visibility != "workspace" {
		t.Errorf("automatic work narrowed a defaulted visibility → visibility %q, held back %v; want it refused",
			visibility, held)
	}
}

// A boolean default is not nobody's edit. A product a human added is on sale
// because it is active, so automatic work may not take it off sale.
func TestAutomaticWorkIsRefusedMovingABooleanDefaultOnAHumanCreatedRecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	productID := createdID(t, e, "/v1/products", AnyMap{
		"name": "Defaulted Active Day", "unit_price_minor": 1000, "currency": "EUR", "source": "manual",
	})
	if held := automaticPatch(t, e, "product", productID, AnyMap{"active": false}); !held {
		t.Error("automatic work took a human's product off sale")
	}
	var product struct {
		Active bool `json:"active"`
	}
	if status := e.Call(t, "GET", "/v1/products/"+productID, nil, nil, &product); status != http.StatusOK || !product.Active {
		t.Errorf("after the refusal the product reads %d active=%v; want it still on sale", status, product.Active)
	}
}

// agentPatchCompany sends automatic work's one-field change and returns the
// value the record holds afterwards, and whether the change was held back.
func agentPatchCompany(t *testing.T, e *apptest.AppEnv, companyID, field, value string) (current string, held bool) {
	t.Helper()
	held = automaticPatch(t, e, "company", companyID, AnyMap{field: value})
	var company map[string]any
	if status := e.Call(t, "GET", "/v1/companies/"+companyID, nil, nil, &company); status != http.StatusOK {
		t.Fatalf("read back → %d", status)
	}
	current, ok := company[field].(string)
	if !ok {
		t.Fatalf("read back carries no %s string: %v", field, company[field])
	}
	return current, held
}

// automaticPatch sends update_record as automatic work and reports whether it
// was held back: refused because a field it touches is human-owned. Nothing is
// ever staged for it.
func automaticPatch(t *testing.T, e *apptest.AppEnv, recordType, id string, fields AnyMap) bool {
	t.Helper()
	call, err := json.Marshal(AnyMap{"record_type": recordType, "id": id, "fields": fields})
	if err != nil {
		t.Fatal(err)
	}
	_, err = automaticInvoker(t, e)("update_record", string(call))
	if isPassportlessStagingRefusal(err) {
		return true
	}
	if err != nil {
		t.Fatalf("automatic %s patch → %v", recordType, err)
	}
	return false
}
