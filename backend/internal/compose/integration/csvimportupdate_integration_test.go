// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// `on_duplicate: update` — writing a file ONTO the companies already held.
//
// The correction case, and the one the other two modes cannot serve: `create`
// mints a twin, `skip` leaves the estate stale, and before this the honest
// answer to "I have a corrected export" was "retype it in the app".
//
// Three properties, and the middle one is the reason the mode is bounded rather
// than general: it writes where the match is CERTAIN, refuses where the names
// merely score alike, and reports nothing changed when nothing did. An import
// cannot be undone from this surface, so overwriting the wrong company is
// permanent in a way that a twin — repairable by merging — is not.

import (
	"net/http"
	"testing"
)

// on_duplicate: update — the correction case. A spreadsheet of companies the
// CRM already holds, where the file is the better copy.
//
// Before this existed the only ways to apply such a file were to mint twins or
// to leave the estate stale, so the honest answer to "I have a corrected export"
// was "paste it into the app by hand". The preview and the commit must agree
// here for the same reason they must everywhere: an approval is a decision about
// what the report said.
func TestCSVImportUpdateDuplicatesWritesOntoTheIncumbent(t *testing.T) {
	e := setupImportApp(t)

	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Kestrel Data"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}
	orgsBefore := len(organizations(t, e).Data)

	const file = "Company,City,Country\nKestrel Data,Bremen,DE\nNordwind Logistik,Kiel,DE\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city", "Country": "address.country"},
		"update")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 1 || report.Disposition.Updated != 1 {
		t.Fatalf("preview = created %d updated %d, want 1 and 1 — the duplicate is written onto, "+
			"the new company lands", report.Disposition.Created, report.Disposition.Updated)
	}
	if report.Disposition.Skipped != 0 {
		t.Errorf("skipped = %d, want 0 — an update run skips nothing it can write",
			report.Disposition.Skipped)
	}
	// Still disclosed. `updated` says what happens; `duplicates` says the row was
	// already here, which is the fact a person weighs before approving an edit.
	if report.Disposition.Duplicates == nil || *report.Disposition.Duplicates != 1 {
		t.Errorf("duplicates = %v, want 1 — an update is still a duplicate and the report must say so",
			report.Disposition.Duplicates)
	}
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read", got, report.RowsRead)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	after := organizations(t, e).Data
	if len(after) != orgsBefore+1 {
		t.Errorf("companies went from %d to %d; an update run adds only the genuinely new one",
			orgsBefore, len(after))
	}
	kestrels := 0
	for _, o := range after {
		if o.DisplayName == "Kestrel Data" {
			kestrels++
		}
	}
	if kestrels != 1 {
		t.Fatalf("%d companies named Kestrel Data; update writes onto the incumbent rather than "+
			"minting a twin", kestrels)
	}

	// The file's values on the record that was already here — the whole point of
	// the mode, and not something the counts alone can show.
	var withAddress struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Address     struct {
				City    string `json:"city"`
				Country string `json:"country"`
			} `json:"address"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil,
		&withAddress); status != http.StatusOK {
		t.Fatalf("re-reading the companies → %d, want 200", status)
	}
	for _, o := range withAddress.Data {
		if o.DisplayName != "Kestrel Data" {
			continue
		}
		if o.Address.City != "Bremen" || o.Address.Country != "DE" {
			t.Errorf("the incumbent has city %q country %q, want Bremen/DE — the file's values are "+
				"what an update lands", o.Address.City, o.Address.Country)
		}
	}
}

// The refusal that makes update safe: a name that merely LOOKS similar is not
// written onto.
//
// The ladder answers `exact_collision` for a domain match and `fuzzy_review` for
// a name score, and only the first is an identity. Writing on the second would
// move one company's address onto another's, silently and with no undo from this
// surface — a strictly worse mistake than the twin that `create` risks, which a
// merge can repair. So the row is reported as a skip naming what to do about it,
// and the person holding the file decides.
func TestCSVImportUpdateWillNotWriteOnAMerelySimilarName(t *testing.T) {
	e := setupImportApp(t)

	// Two names a person can tell apart and a trigram score cannot: same first
	// word, different companies. NOT a legal-suffix variation — NormalizeOrgName
	// strips those deliberately, so "Kestrel Data" and "Kestrel Data GmbH" ARE
	// the same company to the ladder and updating across them is correct.
	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Kestrel Data Systems"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}

	const file = "Company,City\nKestrel Data Solutions,Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city"}, "update")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	// Whichever way the ladder scored these two names, the run must not have
	// silently overwritten: either it saw no match and created, or it saw a fuzzy
	// one and skipped. What it may never do is write onto a guess.
	if report.Disposition.Updated != 0 {
		t.Errorf("updated = %d for two names that merely SCORE alike — Systems and Solutions are "+
			"different companies, and an update run must not overwrite a record it guessed at",
			report.Disposition.Updated)
	}
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read", got, report.RowsRead)
	}
}

// An update run that finds nothing to change reports nothing changed.
//
// The same reason `unchanged` exists at all: an import that re-applies a file it
// already applied must not report a hundred writes and an audit trail to match.
func TestCSVImportUpdateOfIdenticalValuesIsUnchanged(t *testing.T) {
	e := setupImportApp(t)

	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Kestrel Data", "address": map[string]any{"city": "Bremen"}},
		nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}

	const file = "Company,City\nKestrel Data,Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city"}, "update")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Unchanged != 1 || report.Disposition.Updated != 0 {
		t.Errorf("preview = updated %d unchanged %d, want 0 and 1 — the file says nothing new",
			report.Disposition.Updated, report.Disposition.Unchanged)
	}
	if report.Disposition.Duplicates == nil || *report.Disposition.Duplicates != 1 {
		t.Errorf("duplicates = %v, want 1 — still a row already here", report.Disposition.Duplicates)
	}
}

// Two legal entities sharing a stem are not one company, and update leaves them
// alone.
//
// This is the defect the first version of the mode shipped with. The dedupe
// ladder strips the trailing legal suffix before scoring, so "Acme Inc" and
// "Acme GmbH" reach it as the same string and score a perfect 1.0 — and
// fuzzyOrganization says in as many words that they are a human's call rather
// than a merge. A rule keyed on that score wrote onto whichever record ranked
// first, permanently, performing the merge the ladder had refused.
//
// End to end rather than only in the unit gate, because the unit gate asks
// people.SameOrganizationName directly and this asks what a person importing a
// file actually gets.
func TestCSVImportUpdateWillNotWriteAcrossLegalForms(t *testing.T) {
	e := setupImportApp(t)

	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Falkenberg Maschinenbau GmbH"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}

	const file = "Company,City\nFalkenberg Maschinenbau AG,Stuttgart\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city"}, "update")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Updated != 0 {
		t.Fatalf("updated = %d — the GmbH and the AG are different legal entities, and an update "+
			"run overwrote one with the other's values", report.Disposition.Updated)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	// The incumbent still has no city: nothing was written onto it.
	var orgs struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Address     struct {
				City string `json:"city"`
			} `json:"address"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); status != http.StatusOK {
		t.Fatalf("reading the companies → %d, want 200", status)
	}
	for _, o := range orgs.Data {
		if o.DisplayName == "Falkenberg Maschinenbau GmbH" && o.Address.City != "" {
			t.Errorf("the GmbH now has city %q, taken from a row naming the AG", o.Address.City)
		}
	}
}

// The four counts sum to rows_read when SEVERAL rows are skipped for the same
// reason, and their external ids are not line numbers.
//
// The report deduplicates skipped rows so that one row refused by both the dry
// run and the commit is named once. It used to key that on the LINE derived from
// the external id — and lineOf answers 0 for every id not shaped "line N", which
// is every id in a file carrying its own source_key column. Several skips then
// collapsed onto one another: rows_read 2, skipped 1, nothing else, so the four
// counts summed to 1.
//
// `update` is what made this reachable in bulk rather than in theory: refusing a
// near-miss name is its ordinary behaviour, not an error case, so a real file
// produces many of them at once.
func TestCSVImportSkippedRowsAreCountedIndividually(t *testing.T) {
	e := setupImportApp(t)

	for _, name := range []string{"Aurora Metallbau GmbH", "Boreas Logistik GmbH"} {
		if status := e.Call(t, http.MethodPost, "/v1/organizations",
			map[string]any{"display_name": name}, nil, nil); status != http.StatusCreated {
			t.Fatalf("creating %s → %d, want 201", name, status)
		}
	}

	// Two rows, each a near miss of a different incumbent, each refused for the
	// same reason — under a source key that is a NAME rather than a line number,
	// which is what makes lineOf answer 0 for both.
	const file = "Company,City\n" +
		"Aurora Metallbau AG,Bremen\n" +
		"Boreas Logistik AG,Kiel\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	var run importRunDTO
	status := e.Call(t, http.MethodPost, "/v1/imports", map[string]any{
		"connector": "csv", "object": "organization", "source_ref": profile.SourceRef,
		"mapping": map[string]string{
			"Company": "display_name", "City": "address.city",
		},
		"source_key": "Company", "on_duplicate": "update",
	}, nil, &run)
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	t.Logf("rows_read=%d created=%d updated=%d unchanged=%d skipped=%d issues=%d",
		report.RowsRead, report.Disposition.Created, report.Disposition.Updated,
		report.Disposition.Unchanged, report.Disposition.Skipped, len(report.Issues))
	if report.Disposition.Skipped != 2 {
		t.Errorf("skipped = %d, want 2 — both near misses are refused, and each is its own row",
			report.Disposition.Skipped)
	}
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read — a row went missing from the counts, "+
			"which is exactly the report 'hiding something' its own contract warns about",
			got, report.RowsRead)
	}
}

// Two companies legitimately sharing a name, and an update run picking one.
//
// The name axis has no unique index and DedupeOrganizationForCreate says why in
// as many words: two organizations may legitimately share a name. When several
// do, fuzzyOrganization ranks them and breaks the tie on the LOWEST UUID — an
// arbitrary winner, correct for proposing a review pair and not a basis for
// overwriting one of them.
//
// So an exact name match is not an identity either, and this is the case that
// shows it: the file cannot say which "Kestrel Data" it means, and neither can
// the importer.
func TestCSVImportUpdateWillNotPickBetweenTwoCompaniesSharingAName(t *testing.T) {
	e := setupImportApp(t)

	for range 2 {
		if status := e.Call(t, http.MethodPost, "/v1/organizations",
			map[string]any{"display_name": "Kestrel Data"}, nil, nil); status != http.StatusCreated {
			t.Fatalf("creating an incumbent → %d, want 201", status)
		}
	}

	const file = "Company,City\nKestrel Data,Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city"}, "update")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Updated != 0 {
		t.Fatalf("updated = %d — two companies share this name, so the run overwrote one of them "+
			"chosen by uuid order, permanently, and the file never said which it meant",
			report.Disposition.Updated)
	}
}

// A trading name matching somebody's REGISTERED name is not the same name.
//
// bestOrgNamePairing scores all four combinations of the two names on each side
// and keeps the best, so a row naming "Kestrel Data" as a display name matches an
// incumbent whose legal_name is "Kestrel Data" — and reports the hit as
// `legal_name`, because MatchedField names the stored side only. Reading that
// label alone makes a cross-axis hit look like a legal-to-legal identity.
//
// It is a fine reason to show a human two records. It is not grounds to overwrite
// one, and specifically not grounds to write the row's values over an
// incumbent's registered name.
func TestCSVImportUpdateWillNotWriteAcrossTheNameAxes(t *testing.T) {
	e := setupImportApp(t)

	if status := e.Call(t, http.MethodPost, "/v1/organizations", map[string]any{
		"display_name": "Nordwind Holding", "legal_name": "Kestrel Data",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}

	// The row names Kestrel Data as its TRADING name and says nothing about a
	// registered one.
	const file = "Company,City\nKestrel Data,Bremen\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city"}, "update")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Updated != 0 {
		t.Fatalf("updated = %d — the row's trading name met an incumbent's REGISTERED name, and the "+
			"run overwrote a company that was never the one named", report.Disposition.Updated)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	var orgs struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			LegalName   string `json:"legal_name"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); status != http.StatusOK {
		t.Fatalf("reading the companies → %d, want 200", status)
	}
	for _, o := range orgs.Data {
		if o.DisplayName == "Nordwind Holding" && o.LegalName != "Kestrel Data" {
			t.Errorf("the incumbent's registered name is now %q; an import overwrote it from a row "+
				"that only ever named a trading name", o.LegalName)
		}
	}
}
