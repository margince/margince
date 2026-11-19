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
			if !hasReasonCode(predicted, tc.warning) || !hasReasonCode(predicted, "company_domain_matched") || hasIssue(predicted, "broken_link", "SRC-9") {
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

// A ref whose earlier run was undone binds nothing. Its row is filed onto the
// company now holding its domain, and fills only empty fields there.
func TestARefFromAnUndoneRunFillsOnlyEmptyFieldsOfTheDomainsHolder(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{"companies.csv": "ref,name,domain\nSRC-1,First Name,undone-ref.test\n"}
	first := stageBundle(t, e, uploadZip(t, e, "undone.zip", zipBytes(t, files)).SourceRef, "undone", companyDomainParts)
	approveAndRead(t, e, first.ID)
	if status := importCall(t, e, first.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d, want 202", status)
	}
	holder := createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Holder Name", "domains": []AnyMap{{"domain": "undone-ref.test", "is_primary": true}},
	})
	files["companies.csv"] = "ref,name,domain\nSRC-1,Second Name,undone-ref.test\n"
	again := stageBundle(t, e, uploadZip(t, e, "undone2.zip", zipBytes(t, files)).SourceRef, "undone", companyDomainParts)
	predicted := verdicts(readImportReport(t, e, again.ID))
	approveAndRead(t, e, again.ID)
	if committed := verdicts(readImportReport(t, e, again.ID)); !slices.Equal(committed, predicted) {
		t.Fatalf("import = %v, test run said %v", committed, predicted)
	}
	if facts := companyFacts(t, e, holder); facts[0] != "Holder Name" {
		t.Fatalf("company = %v, want the holder's own name kept", facts)
	}
}

// An importer who may read companies but not update them files no row onto a
// domain's holder. The row stays refused.
func TestACompanyRowOnAHeldDomainNeedsTheCompanyUpdateGrant(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	var role struct {
		Key string `json:"key"`
	}
	if status := e.Call(t, "POST", "/v1/roles", AnyMap{"copy_from": "admin", "name": "No company edits"}, nil, &role); status != http.StatusCreated {
		t.Fatalf("creating the role → %d, want 201", status)
	}
	grant := AnyMap{"create": true, "read": true, "update": false, "delete": false}
	if status := e.Call(t, "PATCH", "/v1/roles/"+role.Key+"/objects/company", grant, nil, nil); status != http.StatusOK {
		t.Fatalf("removing the company update grant → %d, want 200", status)
	}
	const email, password = "no-update@held-domain.test", "held-domain-no-update-password"
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

// A holder whose domain the store keeps in its Unicode spelling still takes a
// row that writes the ASCII spelling.
func TestACompanyRowFindsTheHolderOfAnotherSpellingOfItsDomain(t *testing.T) {
	e := setupImportApp(t)
	holder := createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Bücher Haus", "domains": []AnyMap{{"domain": "idn-placeholder.test", "is_primary": true}},
	})
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE company_domain SET domain = 'bücher-idn.test' WHERE company_id = $1`, holder); err != nil {
		t.Fatalf("storing the Unicode spelling: %v", err)
	}
	files := map[string]string{
		"companies.csv": "ref,name,domain\nSRC-9,Bücher Haus,xn--bcher-idn-q9a.test\n",
		"contacts.csv":  "ref,email,name,company\nP-1,ana@idn.example,Ana,SRC-9\n",
	}
	run := stageBundle(t, e, uploadZip(t, e, "idn.zip", zipBytes(t, files)).SourceRef, "idn", append(companyDomainParts, contactRefParts()...))
	predicted := readImportReport(t, e, run.ID)
	if !hasReasonCode(predicted, "company_domain_matched") || hasReasonCode(predicted, "domain_held") {
		t.Fatalf("test run issues = %+v, want the row filed onto the holder", predicted.Issues)
	}
	approveAndRead(t, e, run.ID)
	if employer := employerOf(t, e, "ana@idn.example"); employer != "Bücher Haus" {
		t.Fatalf("employer = %q, want the holder of the Unicode spelling", employer)
	}
}

// The holder of a domain may appear after the test run. The import then
// reports the match it meets and the values it kept.
func TestTheImportReportsAMatchItMeetsAfterTheTestRun(t *testing.T) {
	e := setupImportApp(t)
	run := heldDomainZip(t, e, "Example Telco Ltd", "")
	if predicted := readImportReport(t, e, run.ID); predicted.Disposition.Created != 2 {
		t.Fatalf("test run = %+v, want both rows created while no company holds the domain", predicted.Disposition)
	}
	createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Example Telecom",
		"domains": []AnyMap{{"domain": "example.test", "is_primary": true}},
	})
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if !hasReasonCode(committed, "company_domain_matched") || !hasReasonCode(committed, "stored_values_kept") {
		t.Fatalf("import issues = %+v, want the match and the kept name both reported", committed.Issues)
	}
}
