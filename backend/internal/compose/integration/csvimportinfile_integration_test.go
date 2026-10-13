// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The dry run must say what approval will do when the file's own rows collide.
// The commit lands them one at a time, so a later look-alike meets each earlier
// row in the estate.

import (
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// previewThenCommit stages a run, reads its preview, approves it and reads
// the finished report.
func previewThenCommit(t *testing.T, e *apptest.AppEnv, object, file string, run map[string]any) (importReportDTO, importReportDTO) {
	t.Helper()
	profile, status := uploadCSV(t, e, object, file)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	body := map[string]any{"connector": "csv", "object": object, "source_ref": profile.SourceRef}
	maps.Copy(body, run)
	var staged importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports", body, nil, &staged); status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	var preview, final importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+staged.ID+"/report", nil, nil, &preview); status != http.StatusOK {
		t.Fatalf("preview report → %d, want 200", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+staged.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+staged.ID+"/report", nil, nil, &final); status != http.StatusOK {
		t.Fatalf("final report → %d, want 200", status)
	}
	return preview, final
}

// assertPreviewIsTheCommit holds the preview to the finished run and both to
// the counts the file should produce.
func assertPreviewIsTheCommit(t *testing.T, preview, final importReportDTO, want importDispositionDTO) {
	t.Helper()
	if final.Status != "complete" {
		t.Fatalf("the run ended %q, want complete; issues: %+v", final.Status, final.Issues)
	}
	for _, report := range []struct {
		name string
		got  importReportDTO
	}{{"preview", preview}, {"commit", final}} {
		got := report.got.Disposition
		if got.Created != want.Created || got.Updated != want.Updated ||
			got.Unchanged != want.Unchanged || got.Skipped != want.Skipped {
			t.Errorf("%s = created %d updated %d unchanged %d skipped %d, want %d %d %d %d",
				report.name, got.Created, got.Updated, got.Unchanged, got.Skipped,
				want.Created, want.Updated, want.Unchanged, want.Skipped)
		}
		if got.Duplicates == nil || *got.Duplicates != *want.Duplicates {
			t.Errorf("%s reports %s duplicate(s), want %d", report.name, duplicatesSaid(report.got), *want.Duplicates)
		}
	}
}

func dispositionOf(created, unchanged, skipped, duplicates int) importDispositionDTO {
	return importDispositionDTO{Created: created, Unchanged: unchanged, Skipped: skipped, Duplicates: &duplicates}
}

func TestTheDryRunSkipsTheLookAlikesTheCommitSkips(t *testing.T) {
	e := setupImportApp(t)
	const file = "Company\nSkipco Trading\nSkipco Trading GmbH\nSkipco Tradng\nNordwind Logistik\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Company": "display_name"}, "on_duplicate": "skip",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(2, 0, 2, 2))
	if got := len(companyList(t, e).Data); got != 2 {
		t.Errorf("%d companies exist after the run, want the 2 the preview promised", got)
	}
}

func TestTheDryRunCountsTheLookAlikesTheCommitLands(t *testing.T) {
	e := setupImportApp(t)
	const file = "Company\nKestrel Data\nKestrel Dta\nKestrell Data\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Company": "display_name"},
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(3, 0, 0, 2))
}

// A domain is one company's across the estate, so the second row naming it is
// refused by the store however differently the two are named.
func TestTheDryRunRefusesADomainAnEarlierRowTakes(t *testing.T) {
	e := setupImportApp(t)
	const file = "Company,Website\nZephyr Freight,https://zephyr.example/\nNordwind Logistik,zephyr.example\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Company": "display_name", "Website": "domain"},
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
}

func TestTheDryRunMeetsAContactAnEarlierRowCreates(t *testing.T) {
	e := setupImportApp(t)
	const file = "Full Name,Email\n" +
		"Mareike Olsen,mareike@kestrel.example\n" +
		"Mareike Olson,m.olson@kestrel.example\n" +
		"Jonas Weber,jonas@kestrel.example\n"

	preview, final := previewThenCommit(t, e, "contact", file, map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "Email": "email"},
		"source_key": "Full Name", "on_duplicate": "skip",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(2, 0, 1, 1))
}

func TestTheDryRunRefusesAnAddressAnEarlierRowTakes(t *testing.T) {
	e := setupImportApp(t)
	const file = "Full Name,Email\nAda Lovelace,ada@lovelace.example\nGrace Hopper,ADA@lovelace.example\n"

	preview, final := previewThenCommit(t, e, "contact", file, map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "Email": "email"},
		"source_key": "Full Name",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
}

func TestALeadFileWithABadCellPreviewsTheSkipAndCommitsTheRest(t *testing.T) {
	e := setupImportApp(t)
	const file = "Email,Full Name,Phone\n" +
		"ada@lovelace.example,Ada Lovelace,+49 30 1234567\n" +
		"not-an-email,Grace Hopper,\n" +
		"katherine@johnson.example,Katherine Johnson,call me\n" +
		"dorothy@vaughan.example,Dorothy Vaughan,\n"

	preview, final := previewThenCommit(t, e, "lead", file, map[string]any{
		"mapping": map[string]string{"Email": "email", "Full Name": "full_name", "Phone": "phone"},
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(2, 0, 2, 0))
	if got := leadCount(t, e); got != 2 {
		t.Errorf("%d leads exist after the run, want the 2 the preview promised", got)
	}
}

func TestAReImportOfAPhoneSpelledWithSpacesChangesNothing(t *testing.T) {
	e := setupImportApp(t)
	const file = "Email,Full Name,Phone\nada@lovelace.example,Ada Lovelace,+84 90 111 0002\n"
	run := map[string]any{"mapping": map[string]string{"Email": "email", "Full Name": "full_name", "Phone": "phone"}}

	previewThenCommit(t, e, "lead", file, run)
	preview, final := previewThenCommit(t, e, "lead", file, run)

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(0, 1, 0, 0))
}

func TestTheDryRunRefusesALeadAddressAnEarlierRowTakes(t *testing.T) {
	e := setupImportApp(t)
	const file = "Full Name,Email\nAda Lovelace,ada@lovelace.example\nAda L.,ADA@lovelace.example\n"

	preview, final := previewThenCommit(t, e, "lead", file, map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "Email": "email"},
		"source_key": "Full Name",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
}

// The store refuses a second live lead on one address. The run skips that row
// and completes rather than failing at it.
func TestALeadAddressAlreadyHeldIsSkippedNotAFailedRun(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/leads", map[string]any{
		"full_name": "Ada Lovelace", "email": "ada@lovelace.example", "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent lead → %d, want 201", status)
	}
	const file = "Email,Full Name\nada@lovelace.example,Ada Byron\ngrace@hopper.example,Grace Hopper\n"

	preview, final := previewThenCommit(t, e, "lead", file, map[string]any{
		"mapping": map[string]string{"Email": "email", "Full Name": "full_name"},
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
}

// A company row with no name is refused at create. Its domain must not then
// refuse a later row that the commit lands.
func TestAnUnnamedCompanyRowHoldsNoDomain(t *testing.T) {
	e := setupImportApp(t)
	const file = "Note,Company,Legal,Website\nfirst,,Acme GmbH,acme.example\nsecond,Acme,,acme.example\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{
			"Note": "description", "Company": "display_name", "Legal": "legal_name", "Website": "domain",
		},
		"source_key": "Note",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
	for name, report := range map[string]importReportDTO{"preview": preview, "commit": final} {
		if len(report.Issues) != 1 || report.Issues[0].Line != 2 ||
			report.Issues[0].Reason != "the mapped display_name is empty, so the row names no company" {
			t.Errorf("%s issues = %+v, want only line 2 refused for naming no company", name, report.Issues)
		}
	}
}

// A skip run refuses a held address for the address. The reason then does not
// depend on whether the caller can see who holds it.
func TestASkipRunRefusesAHeldAddressForTheAddress(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/contacts", map[string]any{
		"source": "manual", "full_name": "Ada Lovelace",
		"emails": []map[string]any{{"email": "ada@lovelace.example", "is_primary": true}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent contact → %d, want 201", status)
	}
	const file = "Full Name,Email\nAda Byron,ada@lovelace.example\n"

	preview, final := previewThenCommit(t, e, "contact", file, map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "Email": "email"},
		"source_key": "Full Name", "on_duplicate": "skip",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(0, 0, 1, 0))
	for _, report := range []importReportDTO{preview, final} {
		if len(report.Issues) != 1 || report.Issues[0].Reason != "this email address is already held in the CRM, "+
			"so the row cannot create a second contact under it" {
			t.Errorf("issues = %+v, want the held-address refusal", report.Issues)
		}
	}
}

// Two colleagues share a mail domain and nothing else. The ladder never admits
// one as the other's candidate, so neither is a duplicate on either side.
func TestColleaguesOnOneDomainAreNotLookAlikes(t *testing.T) {
	e := setupImportApp(t)
	const file = "Full Name,Email\n" +
		"Tom Brandt,tom@kestrel.example\nTim Berger,tim@kestrel.example\n" +
		"Anna Weber,anna@kestrel.example\nAnja Wolf,anja@kestrel.example\n" +
		"Lukas Hoffmann,lukas@kestrel.example\nLena Hartmann,lena@kestrel.example\n"

	preview, final := previewThenCommit(t, e, "contact", file, map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "Email": "email"},
		"source_key": "Full Name", "on_duplicate": "skip",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(6, 0, 0, 0))
}

// The estate's look-alike makes the second row a duplicate. The commit refuses
// it first for the domain the first row took.
func TestADuplicateSkipStillMeetsTheDomainAnEarlierRowTakes(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/companies", map[string]any{
		"display_name": "Nordwind Logistik", "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent company → %d, want 201", status)
	}
	const file = "Company,Website\nZephyr Freight,zephyr.example\nNordwind Logistik,zephyr.example\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Company": "display_name", "Website": "domain"}, "on_duplicate": "skip",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
}

func TestTheDryRunRefusesALinkedInURLAnEarlierRowTakes(t *testing.T) {
	e := setupImportApp(t)
	const file = "Company,LinkedIn\n" +
		"Zephyr Freight,https://www.linkedin.com/company/zephyr\n" +
		"Nordwind Logistik,https://www.linkedin.com/company/zephyr\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Company": "display_name", "LinkedIn": "linkedin_url"},
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
}

// A re-import moves an address onto a record the importer landed before, after
// an earlier row of the same file has taken it. The store refuses the move.
func TestAnUpdateMeetsTheAddressAnEarlierRowTakes(t *testing.T) {
	for _, object := range []string{"contact", "lead"} {
		t.Run(object, func(t *testing.T) {
			e := setupImportApp(t)
			run := map[string]any{
				"mapping":    map[string]string{"Full Name": "full_name", "Email": "email"},
				"source_key": "Full Name",
			}
			previewThenCommit(t, e, object, "Full Name,Email\nGrace Hopper,grace@hopper.example\n", run)
			const file = "Full Name,Email\nAda Lovelace,ada@lovelace.example\nGrace Hopper,ada@lovelace.example\n"

			preview, final := previewThenCommit(t, e, object, file, run)

			assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
			for name, report := range map[string]importReportDTO{"preview": preview, "commit": final} {
				if len(report.Issues) != 1 || report.Issues[0].Line != 3 {
					t.Errorf("%s issues = %+v, want only line 3 refused", name, report.Issues)
				}
			}
		})
	}
}
