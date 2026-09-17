// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// GET /companies/{id}/capture-triage over the real wire.
//
// The mapping from ledger state to rung is proved where it is written
// (compose/pipelinetrace/companytriage_test.go). What is proved HERE is the
// half a unit test cannot reach: that the ledger row a real installation holds
// arrives at a reader through the company's own gate, and that a company
// nobody triaged into is answered rather than refused.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type companyTriageDTO struct {
	CompanyID string `json:"company_id"`
	Domains   []struct {
		Domain string `json:"domain"`
		Rung   struct {
			Stage       string  `json:"stage"`
			Order       int     `json:"order"`
			SubjectKind string  `json:"subject_kind"`
			Status      string  `json:"status"`
			Reason      *string `json:"reason"`
			ReasonText  *string `json:"reason_text"`
			At          *string `json:"at"`
		} `json:"rung"`
	} `json:"domains"`
}

// triageEnv boots a harness that COMPOSED the capture trace.
//
// The company door is the trace's third door and lives in its handler set, so
// a composition without it answers 503 — correctly: a deployment that composed
// no pipeline read has no company check to report, and an empty list there
// would say the domains were never triaged. The default harness composes none,
// so a suite about the door opts in rather than the door being loosened to
// suit it.
func triageEnv(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithCaptureTrace(true))
	e.BootstrapWorkspace(t)
	return e
}

// seedTriagedDomain writes one ledger row resolving a domain into a company.
func seedTriagedDomain(t *testing.T, e *apptest.AppEnv, company, domain, status, source string) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO company_domain_disposition (domain, status, source, company_id)
		 VALUES ($1, $2, $3, $4)`, domain, status, source, company); err != nil {
		t.Fatalf("seeding a %s disposition for %s: %v", status, domain, err)
	}
}

func TestTheCompanyDoorAnswersWhichDomainsWereTriagedIntoIt(t *testing.T) {
	e := triageEnv(t)

	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{
		"display_name": "Nordwind Energie GmbH",
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}
	// Two domains, so the answer is a list rather than a single row that a
	// broken loop could still get right, and ordered so the assertion is about
	// the contract's stated order rather than about insertion order.
	seedTriagedDomain(t, e, company.ID, "zulu.example", "company", "site_read")
	seedTriagedDomain(t, e, company.ID, "alpha.example", "no_site", "heuristic")

	var report companyTriageDTO
	if status := e.Call(t, "GET", "/v1/companies/"+company.ID+"/capture-triage",
		nil, nil, &report); status != http.StatusOK {
		t.Fatalf("GET capture-triage = %d, want 200", status)
	}

	if len(report.Domains) != 2 {
		t.Fatalf("got %d domains, want both: %+v", len(report.Domains), report.Domains)
	}
	if report.Domains[0].Domain != "alpha.example" {
		t.Errorf("domains are not ordered by domain: %+v", report.Domains)
	}
	first := report.Domains[0].Rung
	if first.Stage != "company_triage" || first.SubjectKind != "domain" {
		t.Errorf("rung = %s/%s, want the registered company_triage/domain", first.Stage, first.SubjectKind)
	}
	if first.Status != "done" || first.Reason == nil || *first.Reason != "no_site_identified" {
		t.Errorf("rung = %s/%v, want done/no_site_identified — a settled domain that says nothing "+
			"about HOW it settled is the silence this surface removes", first.Status, first.Reason)
	}
	// The server's own sentence travels beside the class, so a client that does
	// not know this reason yet still renders something true.
	if first.ReasonText == nil || *first.ReasonText == "" {
		t.Error("the rung carries a class and no sentence, so an older client renders a raw key")
	}
	if first.At == nil {
		t.Error("the rung carries no date, so a reader cannot tell how long this has been the answer")
	}
}

func TestACompanyNobodyTriagedIntoIsAnsweredRatherThanRefused(t *testing.T) {
	e := triageEnv(t)

	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{
		"display_name": "Typed In By Hand Ltd",
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}

	var report companyTriageDTO
	// A company recorded by hand is ordinary, not a gap: 200 with no domains,
	// so the screen can say which it is rather than drawing a failure.
	if status := e.Call(t, "GET", "/v1/companies/"+company.ID+"/capture-triage",
		nil, nil, &report); status != http.StatusOK {
		t.Fatalf("GET capture-triage = %d for a hand-recorded company, want 200", status)
	}
	if len(report.Domains) != 0 {
		t.Errorf("a company nothing triaged into reported %+v", report.Domains)
	}
}

func TestTheCompanyDoorHidesACompanyThatIsNotThere(t *testing.T) {
	e := triageEnv(t)

	// Existence-hidden exactly as getCompany hides it. An empty list here would
	// say this company's domains were never triaged, which is a statement about
	// a record the caller cannot see.
	status := e.Call(t, "GET",
		"/v1/companies/00000000-0000-7000-8000-0000000000aa/capture-triage", nil, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("GET capture-triage for a company that does not exist = %d, want 404", status)
	}
}
