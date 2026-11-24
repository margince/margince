// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A parent that already sits below the company is refused on the parent field,
// naming that company, before the database's cycle trigger can answer.
func TestAParentBelowTheCompanyIsRefusedByName(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	top := createCompanyRollupCompany(t, e, "Group Holding", "")
	middle := createCompanyRollupCompany(t, e, "Group Berlin", top)
	bottom := createCompanyRollupCompany(t, e, "Group Berlin Retail", middle)

	var problem struct {
		Code    string `json:"code"`
		Details struct {
			Errors []struct {
				Field  string            `json:"field"`
				Code   string            `json:"code"`
				Params map[string]string `json:"params"`
			} `json:"errors"`
		} `json:"details"`
	}
	status := e.Call(t, "PATCH", "/v1/companies/"+top, AnyMap{"parent_company_id": bottom},
		map[string]string{"If-Match": "1"}, &problem)
	if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) != 1 {
		t.Fatalf("PATCH a grandchild as parent → %d %+v, want one 422 field fault", status, problem)
	}
	fault := problem.Details.Errors[0]
	if fault.Field != "parent_company_id" || fault.Code != "parent_company_below" || fault.Params["name"] != "Group Berlin Retail" {
		t.Fatalf("the refusal names %+v, want parent_company_below on parent_company_id naming Group Berlin Retail", fault)
	}

	problem.Details.Errors = nil
	status = e.Call(t, "PATCH", "/v1/companies/"+top, AnyMap{"parent_company_id": top},
		map[string]string{"If-Match": "1"}, &problem)
	if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) != 1 ||
		problem.Details.Errors[0].Field != "parent_company_id" || problem.Details.Errors[0].Code != "parent_company_self" {
		t.Fatalf("PATCH a company as its own parent → %d %+v, want parent_company_self on parent_company_id", status, problem)
	}

	if status := e.Call(t, "PATCH", "/v1/companies/"+bottom, AnyMap{"parent_company_id": top},
		map[string]string{"If-Match": "1"}, nil); status != http.StatusOK {
		t.Fatalf("moving a company under an unrelated ancestor → %d, want 200", status)
	}
}
