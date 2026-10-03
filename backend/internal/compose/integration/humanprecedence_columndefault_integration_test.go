// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A column default is not something a human typed. A company created by a
// human without a lifecycle holds 'unknown' only because the column says so;
// an agent moving it on overwrites nobody, so the write applies directly.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAgentChangesAFieldThatStillHoldsItsColumnDefault(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Defaulted Lifecycle GmbH"})
	bearer := companyAgentBearer(t, e)

	if lifecycle, approval := agentPatchCompany(t, e, bearer, companyID, "lifecycle", "prospect"); approval != "" || lifecycle != "prospect" {
		t.Fatalf("agent moved a defaulted lifecycle → lifecycle %q, approval %q; want it applied with no approval",
			lifecycle, approval)
	}

	// Once a human sets the field, it is theirs.
	if status := e.Call(t, "PATCH", "/v1/companies/"+companyID, AnyMap{"lifecycle": "customer"}, nil, nil); status != http.StatusOK {
		t.Fatalf("human sets lifecycle → %d", status)
	}
	if _, approval := agentPatchCompany(t, e, bearer, companyID, "lifecycle", "former_customer"); approval == "" {
		t.Error("agent overwrote a lifecycle a human set, with no approval staged")
	}
}

func TestAgentStillAsksBeforeOverwritingAValueTypedAtCreate(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createdID(t, e, "/v1/companies", AnyMap{
		"display_name": "Typed Industry GmbH", "industry": "Logistics",
	})
	bearer := companyAgentBearer(t, e)

	if industry, approval := agentPatchCompany(t, e, bearer, companyID, "industry", "Retail"); approval == "" || industry != "Logistics" {
		t.Errorf("agent overwrote the industry the human typed at create → industry %q, approval %q", industry, approval)
	}
}

// Who can see a record is never a default an agent moves unasked: narrowing a
// human's company to its owner hides it from every colleague.
func TestAgentAsksBeforeNarrowingWhoSeesAHumanCreatedRecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	companyID := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Defaulted Visibility GmbH"})
	bearer := companyAgentBearer(t, e)

	if visibility, approval := agentPatchCompany(t, e, bearer, companyID, "visibility", "owner"); approval == "" || visibility != "workspace" {
		t.Errorf("agent narrowed a defaulted visibility → visibility %q, approval %q; want it staged for approval",
			visibility, approval)
	}
}

// A boolean default is not nobody's edit: a product a human added is on sale
// because it is active, so taking it off sale is theirs to approve.
func TestAgentAsksBeforeMovingABooleanDefaultOnAHumanCreatedRecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	productID := createdID(t, e, "/v1/products", AnyMap{
		"name": "Defaulted Active Day", "unit_price_minor": 1000, "currency": "EUR", "source": "manual",
	})
	bearer := companyAgentBearer(t, e)

	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "PATCH", "/v1/products/"+productID, AnyMap{"active": false}, bearer, &problem)
	if status != http.StatusForbidden || problem.Code != "approval_required" {
		t.Errorf("agent took a human's product off sale → %d %q; want it staged for approval", status, problem.Code)
	}
}

func companyAgentBearer(t *testing.T, e *apptest.AppEnv) map[string]string {
	t.Helper()
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "column-default agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	return map[string]string{"Authorization": "Bearer " + minted.Token}
}

// agentPatchCompany sends the agent's one-field change and returns the value
// the record holds afterwards plus the approval it staged, if any.
func agentPatchCompany(t *testing.T, e *apptest.AppEnv, bearer map[string]string, companyID, field, value string) (current, approvalID string) {
	t.Helper()
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	switch status := e.Call(t, "PATCH", "/v1/companies/"+companyID, AnyMap{field: value}, bearer, &problem); {
	case status == http.StatusForbidden && problem.Code == "approval_required":
		approvalID = ExtractStagedApprovalID(t, problem.Detail)
	case status != http.StatusOK:
		t.Fatalf("agent %s patch → %d %q", field, status, problem.Code)
	}
	var company map[string]any
	if status := e.Call(t, "GET", "/v1/companies/"+companyID, nil, nil, &company); status != http.StatusOK {
		t.Fatalf("read back → %d", status)
	}
	held, ok := company[field].(string)
	if !ok {
		t.Fatalf("read back carries no %s string: %v", field, company[field])
	}
	return held, approvalID
}
