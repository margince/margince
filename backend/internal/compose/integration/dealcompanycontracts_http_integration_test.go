// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal company move refused over the wire: the REST update answers the
// 422 field error the form shows, and a CSV re-import skips the row.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// acmeDealWithContract files an Acme deal and Acme's contract against it over
// REST, and answers the deal and Contoso.
func acmeDealWithContract(t *testing.T, e *apptest.AppEnv) (deal, acme, contoso string) {
	t.Helper()
	acme = createdID(t, e, "/v1/companies", AnyMap{"display_name": "Acme", "source": "manual"})
	contoso = createdID(t, e, "/v1/companies", AnyMap{"display_name": "Contoso", "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal = createdID(t, e, "/v1/deals", AnyMap{
		"name": "Acme expansion", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"company_id": acme, "source": "manual",
	})
	createdID(t, e, "/v1/contracts", AnyMap{
		"company_id": acme, "deal_id": deal, "title": "Acme MSA", "contract_number": "MSA-2026", "value_basis": "total",
	})
	return deal, acme, contoso
}

func dealCompanyOverREST(t *testing.T, e *apptest.AppEnv, deal string) string {
	t.Helper()
	var read struct {
		CompanyID string `json:"company_id"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/deals/"+deal, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("reading the deal → %d", status)
	}
	return read.CompanyID
}

func TestTheRESTDealUpdateRefusesACompanyMoveWithAFieldError(t *testing.T) {
	e := setupImportApp(t)
	deal, acme, contoso := acmeDealWithContract(t, e)

	var problem struct {
		Details struct {
			Errors []struct{ Field, Code, Message string } `json:"errors"`
		} `json:"details"`
	}
	status := e.Call(t, http.MethodPatch, "/v1/deals/"+deal, AnyMap{"company_id": contoso}, nil, &problem)

	if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) != 1 {
		t.Fatalf("PATCH → %d %+v, want one 422 field error", status, problem)
	}
	refusal := problem.Details.Errors[0]
	if refusal.Field != "company_id" || refusal.Code != "deal_contracts_cross_company" ||
		!strings.Contains(refusal.Message, "MSA-2026") {
		t.Errorf("field error = %+v, want company_id naming the contract that blocks", refusal)
	}
	if got := dealCompanyOverREST(t, e, deal); got != acme {
		t.Errorf("the deal names %s after the refusal, want Acme", got)
	}
}

// A file that re-points a deal at another company meets the same refusal: the
// row is skipped and the deal keeps its company.
func TestACSVReimportDoesNotMoveADealAwayFromItsContracts(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	acme := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Acme", "source": "manual"})
	contoso := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Contoso", "source": "manual"})
	header := "ref,name,pipeline,stage,company_ref"
	row := func(company string) string {
		return header + "\n" + fmt.Sprintf("D-1,Imported expansion,%s,%s,%s\n", pipeline, open, company)
	}
	importDeals(t, e, map[string]string{"deals.csv": row(acme)}, dealImportParts(header))
	deal := dealNamed(t, e, "Imported expansion")
	if deal.CompanyID == nil || *deal.CompanyID != acme {
		t.Fatalf("the first import left the deal on %v, want Acme", deal.CompanyID)
	}
	createdID(t, e, "/v1/contracts", AnyMap{
		"company_id": acme, "deal_id": deal.ID, "title": "Acme MSA", "value_basis": "total",
	})

	_, report := importDeals(t, e, map[string]string{"deals.csv": row(contoso)}, dealImportParts(header))

	if report.Disposition.Updated != 0 || report.Disposition.Skipped != 1 {
		t.Fatalf("report = %+v, issues %+v, want the row skipped", report.Disposition, report.Issues)
	}
	if after := dealNamed(t, e, "Imported expansion"); after.CompanyID == nil || *after.CompanyID != acme {
		t.Errorf("the re-import moved the deal to %v, away from its contract", after.CompanyID)
	}
}
