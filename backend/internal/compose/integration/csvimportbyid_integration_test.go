// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A row that NAMES the company it is, by the id the CRM gave it.
//
// The correction workflow: export the companies with their ids, edit the file,
// import it back. This is the path a spreadsheet of corrections takes, and it
// replaces an attempt to reach the same place by matching NAMES — which cannot
// be made safe for overwriting.
//
// The dedupe ladder blurs on purpose, because its question is "should a human
// look at these two?": it strips legal forms, so `Acme Inc` and `Acme GmbH` are
// one string; it scores a trading name against a registered one; and where
// several records tie it picks the lowest uuid. Every blur is free for proposing
// a review and is a way to destroy the wrong company when it decides a write.
// The cases below are the ones that broke the name-matching attempt, kept here
// because an id must be immune to every one of them.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// createOrgReturningID makes one company and answers its id.
func createOrgReturningID(t *testing.T, e *apptest.AppEnv, body map[string]any) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/organizations", body, nil, &created); status != http.StatusCreated {
		t.Fatalf("creating a company → %d, want 201", status)
	}
	return created.ID
}

// orgByName reads one company back, by the name it is listed under.
func orgByName(t *testing.T, e *apptest.AppEnv, want string) (string, string, string) {
	t.Helper()
	var orgs struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			LegalName   string `json:"legal_name"`
			Address     struct {
				City string `json:"city"`
			} `json:"address"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); status != http.StatusOK {
		t.Fatalf("reading the companies → %d, want 200", status)
	}
	for _, o := range orgs.Data {
		if o.DisplayName == want {
			return o.ID, o.Address.City, o.LegalName
		}
	}
	return "", "", ""
}

func TestCSVImportByIDUpdatesTheNamedCompany(t *testing.T) {
	e := setupImportApp(t)
	id := createOrgReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})

	file := "Id,Company,City\n" + id + ",Kestrel Data,Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunWithMapping(t, e, "organization", profile.SourceRef,
		map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"})
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Updated != 1 || report.Disposition.Created != 0 {
		t.Fatalf("preview = created %d updated %d, want 0 and 1 — the row named a company, so it "+
			"updates it", report.Disposition.Created, report.Disposition.Updated)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	gotID, city, _ := orgByName(t, e, "Kestrel Data")
	if gotID != id {
		t.Fatalf("the company named Kestrel Data has id %q, want the one the file named (%q)", gotID, id)
	}
	if city != "Bremen" {
		t.Errorf("city = %q, want Bremen — the file's values land on the record it named", city)
	}
}

// The three cases that destroyed data under name matching, each harmless here.
func TestCSVImportByIDIsImmuneToEveryAmbiguityThatBrokeNameMatching(t *testing.T) {
	t.Run("two companies sharing a name", func(t *testing.T) {
		e := setupImportApp(t)
		wanted := createOrgReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})
		other := createOrgReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})

		file := "Id,Company,City\n" + wanted + ",Kestrel Data,Bremen\n"
		profile, _ := uploadCSV(t, e, "organization", file)
		run, _ := createRunWithMapping(t, e, "organization", profile.SourceRef,
			map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"})
		if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
			t.Fatalf("approve → %d, want 202", status)
		}

		// The named one moved; the other did not. Under name matching the ladder
		// picked between these two on uuid order.
		if city := cityOf(t, e, wanted); city != "Bremen" {
			t.Errorf("the company the file NAMED has city %q, want Bremen", city)
		}
		if city := cityOf(t, e, other); city != "" {
			t.Errorf("the company the file did not name has city %q, want none — an id names one "+
				"record, and the other was never a candidate", city)
		}
	})

	t.Run("different legal forms", func(t *testing.T) {
		e := setupImportApp(t)
		gmbh := createOrgReturningID(t, e, map[string]any{"display_name": "Falkenberg Maschinenbau GmbH"})
		ag := createOrgReturningID(t, e, map[string]any{"display_name": "Falkenberg Maschinenbau AG"})

		file := "Id,Company,City\n" + ag + ",Falkenberg Maschinenbau AG,Stuttgart\n"
		profile, _ := uploadCSV(t, e, "organization", file)
		run, _ := createRunWithMapping(t, e, "organization", profile.SourceRef,
			map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"})
		if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
			t.Fatalf("approve → %d, want 202", status)
		}
		if city := cityOf(t, e, ag); city != "Stuttgart" {
			t.Errorf("the AG has city %q, want Stuttgart", city)
		}
		if city := cityOf(t, e, gmbh); city != "" {
			t.Errorf("the GmbH has city %q, want none — the suffix strip that made these one string "+
				"for the ladder never runs on an id", city)
		}
	})

	t.Run("a trading name that is somebody else's registered name", func(t *testing.T) {
		e := setupImportApp(t)
		trading := createOrgReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})
		registered := createOrgReturningID(t, e, map[string]any{
			"display_name": "Nordwind Holding", "legal_name": "Kestrel Data",
		})

		file := "Id,Company,City\n" + trading + ",Kestrel Data,Bremen\n"
		profile, _ := uploadCSV(t, e, "organization", file)
		run, _ := createRunWithMapping(t, e, "organization", profile.SourceRef,
			map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"})
		if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
			t.Fatalf("approve → %d, want 202", status)
		}
		if city := cityOf(t, e, trading); city != "Bremen" {
			t.Errorf("the company the file named has city %q, want Bremen", city)
		}
		if city := cityOf(t, e, registered); city != "" {
			t.Errorf("the company holding that string as its REGISTERED name has city %q, want none",
				city)
		}
		if _, _, legal := orgByName(t, e, "Nordwind Holding"); legal != "Kestrel Data" {
			t.Errorf("its registered name is now %q; nothing should have touched it", legal)
		}
	})
}

// cityOf reads one company's city by id, "" when it has none.
func cityOf(t *testing.T, e *apptest.AppEnv, id string) string {
	t.Helper()
	var org struct {
		Address struct {
			City string `json:"city"`
		} `json:"address"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations/"+id, nil, nil, &org); status != http.StatusOK {
		t.Fatalf("reading company %s → %d, want 200", id, status)
	}
	return org.Address.City
}

// A row naming an id nothing answers to is REFUSED, not quietly created.
//
// Silently creating would be the worst of both: the person meant to correct one
// company, the file had a typo or a stale export, and they get a new record
// instead — reported as a create they would have to read carefully to notice.
func TestCSVImportByIDRefusesAnIDNothingAnswersTo(t *testing.T) {
	for _, tc := range []struct {
		name, id, wants string
	}{
		{"not an id at all", "ACME-001", "not a company id"},
		{"well-formed but unknown", "01a02ed1-0000-7000-8000-000000000000", "no company you can see"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			before := len(organizations(t, e).Data)

			file := "Id,Company,City\n" + tc.id + ",Kestrel Data,Bremen\n"
			profile, _ := uploadCSV(t, e, "organization", file)
			run, status := createRunWithMapping(t, e, "organization", profile.SourceRef,
				map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"})
			if status != http.StatusAccepted {
				t.Fatalf("create run → %d, want 202", status)
			}

			var report importReportDTO
			if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
				t.Fatalf("report → %d, want 200", status)
			}
			if report.Disposition.Skipped != 1 || report.Disposition.Created != 0 {
				t.Fatalf("preview = created %d skipped %d, want 0 and 1 — a row naming a company that "+
					"is not there is a mistake in the file, not a new company",
					report.Disposition.Created, report.Disposition.Skipped)
			}
			if len(report.Issues) != 1 || !strings.Contains(report.Issues[0].Reason, tc.wants) {
				t.Fatalf("issues = %+v, want one naming %q so the person can go fix the file",
					report.Issues, tc.wants)
			}
			if got := report.Disposition.Created + report.Disposition.Updated +
				report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
				t.Errorf("the disposition sums to %d for %d rows read", got, report.RowsRead)
			}

			if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
				t.Fatalf("approve → %d, want 202", status)
			}
			if after := len(organizations(t, e).Data); after != before {
				t.Errorf("companies went from %d to %d; a refused row must land nothing", before, after)
			}
		})
	}
}

// A mixed file: corrections that name their company, and new companies that do
// not. This is what a real export-edit-import cycle produces.
//
// The id column says which rows are corrections. The row identity every import
// needs — for re-import and for undo — comes from the file's own key column, so
// a row with no id is simply a row naming no record: an ordinary create.
func TestCSVImportByIDTreatsAnEmptyIDAsANewCompany(t *testing.T) {
	e := setupImportApp(t)
	existing := createOrgReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})

	file := "Id,Company,City\n" +
		existing + ",Kestrel Data,Bremen\n" +
		",Nordwind Logistik,Kiel\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	// Identified by the company column, which every row carries — so the id
	// column is free to be empty on the rows that name no record.
	run, status := createRunWithSourceKey(t, e, "organization", profile.SourceRef,
		map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"}, "Company")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Updated != 1 || report.Disposition.Created != 1 {
		t.Fatalf("preview = created %d updated %d, want 1 and 1 — one row names a company and one "+
			"does not", report.Disposition.Created, report.Disposition.Updated)
	}
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read", got, report.RowsRead)
	}
}

// Re-importing an unchanged file changes nothing and says so.
func TestCSVImportByIDOfIdenticalValuesIsUnchanged(t *testing.T) {
	e := setupImportApp(t)
	id := createOrgReturningID(t, e, map[string]any{
		"display_name": "Kestrel Data", "address": map[string]any{"city": "Bremen"},
	})

	file := "Id,Company,City\n" + id + ",Kestrel Data,Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, _ := createRunWithMapping(t, e, "organization", profile.SourceRef,
		map[string]string{"Id": "id", "Company": "display_name", "City": "address.city"})

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Unchanged != 1 || report.Disposition.Updated != 0 {
		t.Errorf("preview = updated %d unchanged %d, want 0 and 1 — the file says nothing new",
			report.Disposition.Updated, report.Disposition.Unchanged)
	}
}

// One address column named alongside an id leaves the rest of the address alone.
func TestCSVImportByIDOfOneAddressColumnKeepsTheRest(t *testing.T) {
	e := setupImportApp(t)
	id := createOrgReturningID(t, e, map[string]any{
		"display_name": "Kestrel Data",
		"address": map[string]any{
			"line1": "Hafenstr. 4", "city": "Hamburg", "postal_code": "20359", "country": "DE",
		},
	})

	file := "Id,City\n" + id + ",Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, _ := createRunWithMapping(t, e, "organization", profile.SourceRef,
		map[string]string{"Id": "id", "City": "address.city"})
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	var org struct {
		Address struct {
			Line1      string `json:"line1"`
			City       string `json:"city"`
			PostalCode string `json:"postal_code"`
			Country    string `json:"country"`
		} `json:"address"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations/"+id, nil, nil, &org); status != http.StatusOK {
		t.Fatalf("reading the company → %d, want 200", status)
	}
	if org.Address.City != "Bremen" {
		t.Errorf("city = %q, want Bremen", org.Address.City)
	}
	if org.Address.Line1 != "Hafenstr. 4" || org.Address.PostalCode != "20359" || org.Address.Country != "DE" {
		t.Errorf("the rest of the address is %+v; a column the file did not carry must not be erased",
			org.Address)
	}
}

// createRunWithSourceKey names the column that identifies a row, where the
// default (a company's display name) is not what the file uses.
func createRunWithSourceKey(t *testing.T, e *apptest.AppEnv, object, sourceRef string,
	mapping map[string]string, sourceKey string,
) (importRunDTO, int) {
	t.Helper()
	var run importRunDTO
	status := e.Call(t, http.MethodPost, "/v1/imports", map[string]any{
		"connector": "csv", "object": object, "source_ref": sourceRef,
		"mapping": mapping, "source_key": sourceKey,
	}, nil, &run)
	return run, status
}

// The four counts sum to rows_read when SEVERAL rows are refused and their
// external ids are not line numbers.
//
// The report collapses a row refused by BOTH the dry run and the commit into a
// single issue, so a person is not sent to fix the same line twice. It keyed that
// collapsing on the LINE derived from the external
// id, and lineOf answers 0 for every id not shaped "line N" — which is every id
// in a file carrying its own key column. Several refusals then collapsed onto
// one another: two refused rows reported as one skip and one phantom
// "unchanged", so the counts summed to less than rows_read.
//
// Two rows naming ids nothing answers to is the cheapest way to produce several
// refusals at once, and an export edited against the wrong workspace produces
// exactly that.
func TestCSVImportSkippedRowsAreCountedIndividually(t *testing.T) {
	e := setupImportApp(t)

	const file = "Company,Id,City\n" +
		"Aurora Metallbau,01a02ed1-0000-7000-8000-000000000001,Bremen\n" +
		"Boreas Logistik,01a02ed1-0000-7000-8000-000000000002,Kiel\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunWithSourceKey(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "Id": "id", "City": "address.city"}, "Company")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Skipped != 2 {
		t.Errorf("skipped = %d, want 2 — both rows name an id nothing answers to, and each is its "+
			"own row", report.Disposition.Skipped)
	}
	if len(report.Issues) != 2 {
		t.Errorf("%d issue(s) for 2 refused rows; a person fixing the file needs both named",
			len(report.Issues))
	}
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read — a row went missing from the counts, "+
			"which is exactly the report 'hiding something' its own contract warns about",
			got, report.RowsRead)
	}
}

// The preview promises what the commit performs, for a row that collides.
//
// Asserted as behaviour rather than by reading the source. An earlier gate
// compared the two call sites' argument text, which caught a parameter being
// added back and would have passed a divergence expressed any other way — a
// different receiver, a different enclosing condition, a helper in between.
// Preview and commit are two code paths that must agree about an OUTCOME, and
// the outcome is the thing to compare.
//
// Both duplicate policies, because they disagree differently: `create` lands a
// second record and discloses it, `skip` leaves the incumbent alone.
func TestThePreviewPromisesWhatTheCommitPerformsForACollision(t *testing.T) {
	for _, policy := range []string{"create", "skip"} {
		t.Run(policy, func(t *testing.T) {
			e := setupImportApp(t)
			if status := e.Call(t, http.MethodPost, "/v1/organizations",
				map[string]any{"display_name": "Kestrel Data"}, nil, nil); status != http.StatusCreated {
				t.Fatalf("creating the incumbent → %d, want 201", status)
			}

			const file = "Company,City\nKestrel Data,Bremen\nNordwind Logistik,Kiel\n"
			profile, _ := uploadCSV(t, e, "organization", file)
			run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
				map[string]string{"Company": "display_name", "City": "address.city"}, policy)
			if status != http.StatusAccepted {
				t.Fatalf("create run → %d, want 202", status)
			}

			var predicted importReportDTO
			if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report",
				nil, nil, &predicted); status != http.StatusOK {
				t.Fatalf("report → %d, want 200", status)
			}
			before := len(organizations(t, e).Data)

			if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve",
				nil, nil, nil); status != http.StatusAccepted {
				t.Fatalf("approve → %d, want 202", status)
			}

			// What the preview PROMISED, measured against what the estate did.
			if landed := len(organizations(t, e).Data) - before; landed != predicted.Disposition.Created {
				t.Errorf("the preview promised %d create(s) and %d company/companies landed — a row "+
					"previewing as one outcome and committing as another is what makes an approval "+
					"a decision about something else", predicted.Disposition.Created, landed)
			}

			var actual importReportDTO
			if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report",
				nil, nil, &actual); status != http.StatusOK {
				t.Fatalf("report after commit → %d, want 200", status)
			}
			if actual.Disposition.Created != predicted.Disposition.Created ||
				actual.Disposition.Skipped != predicted.Disposition.Skipped {
				t.Errorf("preview said created %d skipped %d, the finished run says created %d "+
					"skipped %d", predicted.Disposition.Created, predicted.Disposition.Skipped,
					actual.Disposition.Created, actual.Disposition.Skipped)
			}
			if got := actual.Disposition.Created + actual.Disposition.Updated +
				actual.Disposition.Unchanged + actual.Disposition.Skipped; got != actual.RowsRead {
				t.Errorf("the finished disposition sums to %d for %d rows read", got, actual.RowsRead)
			}
		})
	}
}
