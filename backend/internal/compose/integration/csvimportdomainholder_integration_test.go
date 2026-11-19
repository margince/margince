// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company row whose domain an existing company holds is filed onto that
// company. Its ref then names that company, so a contact linking to the ref
// keeps its employer instead of losing it with a refused row.

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// heldDomainZip stages a companies.csv row on example.test under ref SRC-9,
// and a contact linking to SRC-9.
func heldDomainZip(t *testing.T, e *apptest.AppEnv, companyName, legalName string) importjobtest.Run {
	t.Helper()
	files := map[string]string{
		"companies.csv": "ref,name,legal,domain,about\nSRC-9," + companyName + "," + legalName + ",https://www.example.test/,Fibre and mobile\n",
		"contacts.csv":  "ref,email,name,company\nP-1,ana@example.test,Ana,SRC-9\n",
	}
	parts := append([]AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
		"ref": "ref", "name": "display_name", "legal": "legal_name", "domain": "domain", "about": "description",
	}}}, contactRefParts()...)
	return stageBundle(t, e, uploadZip(t, e, "held.zip", zipBytes(t, files)).SourceRef, "held", parts)
}

// companyFacts reads the fields a domain match may fill but never overwrite.
func companyFacts(t *testing.T, e *apptest.AppEnv, id string) []string {
	t.Helper()
	var name, legalName, description string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT display_name, coalesce(legal_name, ''), coalesce(description, '') FROM company WHERE id = $1`,
		id).Scan(&name, &legalName, &description); err != nil {
		t.Fatalf("reading the company: %v", err)
	}
	return []string{name, legalName, description}
}

func TestACompanyRowOnAHeldDomainKeepsItsContactsLinkAndOnlyFillsEmptyFields(t *testing.T) {
	stored := []string{"Example Telecom", "Example Telecom GmbH", "Fibre and mobile"}
	for _, tc := range []struct {
		name, fileName, fileLegal, warning string
	}{
		{name: "the same name", fileName: "Example Telecom", warning: "company_domain_matched"},
		{name: "a different name and legal name", fileName: "Example Telco Ltd", fileLegal: "Telco Ltd", warning: "stored_values_kept"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			holder := createdID(t, e, "/v1/companies", AnyMap{
				"source": "manual", "display_name": "Example Telecom", "legal_name": "Example Telecom GmbH",
				"domains": []AnyMap{{"domain": "example.test", "is_primary": true}},
			})
			before := len(companyList(t, e).Data)
			run := heldDomainZip(t, e, tc.fileName, tc.fileLegal)

			predicted := readImportReport(t, e, run.ID)
			if d := predicted.Disposition; d.Created != 1 || d.Updated != 1 || d.Skipped != 0 {
				t.Fatalf("test run = %+v, want the contact created and the empty description filled", d)
			}
			if !hasReasonCode(predicted, tc.warning) || hasIssue(predicted, "broken_link", "SRC-9") {
				t.Fatalf("test run issues = %+v, want %s and no broken link", predicted.Issues, tc.warning)
			}
			approveAndRead(t, e, run.ID)
			committed := readImportReport(t, e, run.ID)
			if !slices.Equal(verdicts(committed), verdicts(predicted)) {
				t.Fatalf("import = %v, test run said %v", verdicts(committed), verdicts(predicted))
			}
			if after := len(companyList(t, e).Data); after != before {
				t.Fatalf("companies went from %d to %d, want no second company", before, after)
			}
			if employer := employerOf(t, e, "ana@example.test"); employer != "Example Telecom" {
				t.Fatalf("employer = %q, want the company holding the domain", employer)
			}
			if facts := companyFacts(t, e, holder); !slices.Equal(facts, stored) {
				t.Fatalf("company = %v, want %v: stored values kept, the empty description filled", facts, stored)
			}

			again := heldDomainZip(t, e, tc.fileName, tc.fileLegal)
			approveAndRead(t, e, again.ID)
			if d := readImportReport(t, e, again.ID).Disposition; d.Created != 0 || d.Updated != 0 || d.Skipped != 0 {
				t.Fatalf("repeated import = %+v, want both rows found again and nothing written", d)
			}
			if facts := companyFacts(t, e, holder); !slices.Equal(facts, stored) {
				t.Fatalf("company after a repeated import = %v, want %v", facts, stored)
			}
		})
	}
}

// A domain held by a company the caller cannot see is still refused. The
// contact's link to the row reports what it could not reach.
func TestACompanyRowOnAHiddenHeldDomainStaysRefused(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	colleague := createdID(t, e, "/v1/users", AnyMap{
		"email": "colleague@held-domain.test", "display_name": "Colleague", "role": "rep",
	})
	hidden := createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Example Telecom",
		"domains": []AnyMap{{"domain": "example.test", "is_primary": true}},
	})
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE company SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden, colleague); err != nil {
		t.Fatalf("making the company private to the colleague: %v", err)
	}
	run := heldDomainZip(t, e, "Example Telco Ltd", "")
	predicted := readImportReport(t, e, run.ID)
	if !hasReasonCode(predicted, "domain_held") || hasReasonCode(predicted, "company_domain_matched") {
		t.Fatalf("test run issues = %+v, want the row refused as domain_held", predicted.Issues)
	}
	if !hasIssue(predicted, "broken_link", "SRC-9") {
		t.Fatalf("test run issues = %+v, want the contact's link reported as broken", predicted.Issues)
	}
	approveAndRead(t, e, run.ID)
	if committed := readImportReport(t, e, run.ID); !slices.Equal(verdicts(committed), verdicts(predicted)) {
		t.Fatalf("import = %v, test run said %v", verdicts(committed), verdicts(predicted))
	}
}

func hasReasonCode(report importReportDTO, code string) bool {
	return slices.ContainsFunc(report.Issues, func(issue importIssueDTO) bool { return issue.ReasonCode == code })
}

// A domain held by a company the importer may read but not change is refused
// too. Filing the row onto it would edit a colleague's record.
func TestACompanyRowOnADomainTheImporterCannotChangeStaysRefused(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	var role struct {
		Key     string `json:"key"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/roles", AnyMap{"copy_from": "admin", "name": "Own importer"}, nil, &role); status != http.StatusCreated {
		t.Fatalf("creating the role → %d, want 201", status)
	}
	ifMatch := map[string]string{"If-Match": strconv.FormatInt(role.Version, 10)}
	if status := e.Call(t, "PATCH", "/v1/roles/"+role.Key, AnyMap{"row_scope": "own"}, ifMatch, nil); status != http.StatusOK {
		t.Fatalf("narrowing the role to its own records → %d, want 200", status)
	}
	const email, password = "importer@held-domain.test", "held-domain-importer-password"
	importer := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Importer", "role": role.Key})
	apptest.SetPasswordByLink(t, e, importer, password)
	createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Example Telecom",
		"domains": []AnyMap{{"domain": "example.test", "is_primary": true}},
	})
	seat := e.AsSecondSeat(t, email, password)
	predicted := readImportReport(t, seat, heldDomainZip(t, seat, "Example Telecom", "").ID)
	if !hasReasonCode(predicted, "domain_held") || hasReasonCode(predicted, "company_domain_matched") {
		t.Fatalf("test run issues = %+v, want the row refused as domain_held", predicted.Issues)
	}
}

// A row naming its company by id still overwrites the stored name. Only a
// domain match the file did not ask for fills empty fields alone.
func TestARowNamingItsCompanyByIDStillOverwritesItsName(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	approveAndRead(t, e, namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Renamed Works,\n").ID)
	if facts := companyFacts(t, e, named); facts[0] != "Renamed Works" {
		t.Fatalf("company = %v, want the name the file gave the company it named", facts)
	}
}
