// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The dry run must say what approval will do, row for row. That covers rows
// of one file that collide, a row moving a key another record holds, and a
// cell that cannot land. The commit lands rows one at a time, so a later row meets each
// earlier one in the estate.

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
	if status := testCall(t, e, "/v1/imports", body, &staged); status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	preview := readImportReport(t, e, staged.ID)
	if status := importCall(t, e, staged.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	return preview, readImportReport(t, e, staged.ID)
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
		if len(report.Issues) != 1 || report.Issues[0].Line != 2 || report.Issues[0].ReasonCode != "company_name_empty" {
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
		if len(report.Issues) != 1 || report.Issues[0].ReasonCode != "contact_email_held" {
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

// A re-import moves a key onto a record the importer landed before, while a
// record made by hand holds it. The store refuses the move.
func TestAnUpdateMeetsAKeyTheCRMAlreadyHolds(t *testing.T) {
	cases := []struct {
		object, path, header, key, first, moved string
		incumbent                               map[string]any
		mapping                                 map[string]string
	}{{
		object: "contact", path: "/v1/contacts", header: "Full Name,Email", key: "Full Name",
		first: "Grace Hopper,grace@hopper.example", moved: "Grace Hopper,ada@lovelace.example",
		incumbent: map[string]any{
			"source": "manual", "full_name": "Ada Lovelace",
			"emails": []map[string]any{{"email": "ada@lovelace.example", "is_primary": true}},
		},
		mapping: map[string]string{"Full Name": "full_name", "Email": "email"},
	}, {
		object: "lead", path: "/v1/leads", header: "Full Name,Email", key: "Full Name",
		first: "Grace Hopper,grace@hopper.example", moved: "Grace Hopper,ada@lovelace.example",
		incumbent: map[string]any{"source": "manual", "full_name": "Ada Lovelace", "email": "ada@lovelace.example"},
		mapping:   map[string]string{"Full Name": "full_name", "Email": "email"},
	}, {
		object: "company", path: "/v1/companies", header: "Company,Website", key: "Company",
		first: "Nordwind Logistik,nordwind.example", moved: "Nordwind Logistik,zephyr.example",
		incumbent: map[string]any{
			"source": "manual", "display_name": "Zephyr Freight",
			"domains": []map[string]any{{"domain": "zephyr.example", "is_primary": true}},
		},
		mapping: map[string]string{"Company": "display_name", "Website": "domain"},
	}}
	for _, tc := range cases {
		t.Run(tc.object, func(t *testing.T) {
			e := setupImportApp(t)
			if status := e.Call(t, http.MethodPost, tc.path, tc.incumbent, nil, nil); status != http.StatusCreated {
				t.Fatalf("creating the incumbent → %d, want 201", status)
			}
			run := map[string]any{"mapping": tc.mapping, "source_key": tc.key}
			previewThenCommit(t, e, tc.object, tc.header+"\n"+tc.first+"\n", run)

			preview, final := previewThenCommit(t, e, tc.object, tc.header+"\n"+tc.moved+"\n", run)

			assertPreviewIsTheCommit(t, preview, final, dispositionOf(0, 0, 1, 0))
		})
	}
}

// The lead store checks a LinkedIn URL on create and not on update. A
// re-import may then move one onto its lead while another lead holds it.
func TestALeadUpdateMovesALinkedInURLAnotherLeadHolds(t *testing.T) {
	const url = "https://www.linkedin.com/in/ada"
	run := map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "Email": "email", "LinkedIn": "linkedin_url"},
		"source_key": "Full Name",
	}
	updated := dispositionOf(0, 0, 0, 0)
	updated.Updated = 1
	t.Run("held in the CRM", func(t *testing.T) {
		e := setupImportApp(t)
		if status := e.Call(t, http.MethodPost, "/v1/leads", map[string]any{
			"source": "manual", "full_name": "Ada Lovelace", "linkedin_url": url,
		}, nil, nil); status != http.StatusCreated {
			t.Fatalf("creating the incumbent lead → %d, want 201", status)
		}
		previewThenCommit(t, e, "lead", "Full Name,Email,LinkedIn\nGrace Hopper,grace@hopper.example,\n", run)

		preview, final := previewThenCommit(t, e, "lead", "Full Name,Email,LinkedIn\nGrace Hopper,grace@hopper.example,"+url+"\n", run)

		assertPreviewIsTheCommit(t, preview, final, updated)
	})
	t.Run("held by an earlier row", func(t *testing.T) {
		e := setupImportApp(t)
		previewThenCommit(t, e, "lead", "Full Name,Email,LinkedIn\nGrace Hopper,grace@hopper.example,\n", run)
		file := "Full Name,Email,LinkedIn\nAda Lovelace,ada@lovelace.example," + url + "\nGrace Hopper,grace@hopper.example," + url + "\n"

		preview, final := previewThenCommit(t, e, "lead", file, run)

		want := dispositionOf(1, 0, 0, 0)
		want.Updated = 1
		assertPreviewIsTheCommit(t, preview, final, want)
	})
}

// A lead's LinkedIn URL is compared as stored, case and all, so two URLs that
// differ only in case are two keys. A company's folds case.
func TestALeadLinkedInURLKeepsItsCase(t *testing.T) {
	e := setupImportApp(t)
	const file = "Full Name,LinkedIn\nAda Lovelace,https://www.linkedin.com/in/Ada\nAda Byron,https://www.linkedin.com/in/ada\n"

	preview, final := previewThenCommit(t, e, "lead", file, map[string]any{
		"mapping": map[string]string{"Full Name": "full_name", "LinkedIn": "linkedin_url"}, "source_key": "Full Name",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(2, 0, 0, 0))
}

// Two keys refuse one row: the CRM holds one, an earlier row the other. The
// row is refused for the key the store checks first.
func TestARowRefusedTwiceNamesTheKeyTheCommitNames(t *testing.T) {
	e := setupImportApp(t)
	const url = "https://www.linkedin.com/company/zephyr"
	if status := e.Call(t, http.MethodPost, "/v1/companies", map[string]any{
		"source": "manual", "display_name": "Zephyr Freight", "linkedin_url": url,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent company → %d, want 201", status)
	}
	const file = "Company,Website,LinkedIn\nAlpha Works,alpha.example,\nBeta Werke,alpha.example," + url + "\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Company": "display_name", "Website": "domain", "LinkedIn": "linkedin_url"},
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 0))
	for name, report := range map[string]importReportDTO{"preview": preview, "commit": final} {
		if len(report.Issues) != 1 || report.Issues[0].ReasonCode != "domain_held" {
			t.Errorf("%s issues = %+v, want line 3 refused for the domain", name, report.Issues)
		}
	}
}

// One row reaches a company by its id and a later row by its ref. They are one
// holder, so the second row's domain is its own.
func TestACompanyNamedByIdAndByRefHoldsItsOwnDomain(t *testing.T) {
	e := setupImportApp(t)
	run := map[string]any{
		"mapping":    map[string]string{"Ref": "description", "Id": "id", "Company": "display_name", "Website": "domain", "City": "address.city"},
		"source_key": "Ref",
	}
	previewThenCommit(t, e, "company", "Ref,Id,Company,Website,City\na,,Nordwind Logistik,,\n", run)
	id, _, _ := companyByName(t, e, "Nordwind Logistik")
	file := "Ref,Id,Company,Website,City\nb," + id + ",Nordwind Logistik,nordwind.example,\na,,Nordwind Logistik,nordwind.example,Bremen\n"

	preview, final := previewThenCommit(t, e, "company", file, run)

	want := dispositionOf(0, 0, 0, 0)
	want.Updated = 2
	assertPreviewIsTheCommit(t, preview, final, want)
}

// An update that replaces a single-valued key frees the old value, so a later
// row of the same file may create a record under it.
func TestALaterRowTakesTheKeyAnUpdateFrees(t *testing.T) {
	cases := []struct {
		name, object, header, key, landed, file string
		mapping                                 map[string]string
	}{{
		name: "company LinkedIn", object: "company", header: "Company,LinkedIn", key: "Company",
		landed: "Nordwind Logistik,https://www.linkedin.com/company/nordwind",
		file: "Nordwind Logistik,https://www.linkedin.com/company/nordwind-logistik\n" +
			"Zephyr Freight,https://www.linkedin.com/company/nordwind",
		mapping: map[string]string{"Company": "display_name", "LinkedIn": "linkedin_url"},
	}, {
		name: "lead address", object: "lead", header: "Full Name,Email", key: "Full Name",
		landed:  "Grace Hopper,grace@hopper.example",
		file:    "Grace Hopper,grace@navy.example\nAda Lovelace,grace@hopper.example",
		mapping: map[string]string{"Full Name": "full_name", "Email": "email"},
	}, {
		name: "lead LinkedIn", object: "lead", header: "Full Name,LinkedIn", key: "Full Name",
		landed:  "Grace Hopper,https://www.linkedin.com/in/grace",
		file:    "Grace Hopper,https://www.linkedin.com/in/grace-hopper\nAda Lovelace,https://www.linkedin.com/in/grace",
		mapping: map[string]string{"Full Name": "full_name", "LinkedIn": "linkedin_url"},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			run := map[string]any{"mapping": tc.mapping, "source_key": tc.key}
			previewThenCommit(t, e, tc.object, tc.header+"\n"+tc.landed+"\n", run)

			preview, final := previewThenCommit(t, e, tc.object, tc.header+"\n"+tc.file+"\n", run)

			want := dispositionOf(1, 0, 0, 0)
			want.Updated = 1
			assertPreviewIsTheCommit(t, preview, final, want)
		})
	}
}

// Two leads can hold one LinkedIn URL, since an update moves one onto a lead
// unchecked. Freeing it from one leaves it held by the other.
func TestAKeyAnUpdateFreesStaysHeldByItsOtherHolder(t *testing.T) {
	const url = "https://www.linkedin.com/in/grace"
	e := setupImportApp(t)
	run := map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "LinkedIn": "linkedin_url"},
		"source_key": "Full Name",
	}
	previewThenCommit(t, e, "lead", "Full Name,LinkedIn\nGrace Hopper,"+url+"\nAda Lovelace,\n", run)
	previewThenCommit(t, e, "lead", "Full Name,LinkedIn\nAda Lovelace,"+url+"\n", run)
	file := "Full Name,LinkedIn\nGrace Hopper,https://www.linkedin.com/in/grace-hopper\nLinus Torvalds," + url + "\n"

	preview, final := previewThenCommit(t, e, "lead", file, run)

	want := dispositionOf(0, 0, 1, 0)
	want.Updated = 1
	assertPreviewIsTheCommit(t, preview, final, want)
}

// Two updates may free one key from both its holders, and then it is free.
func TestAKeyTwoUpdatesFreeIsFreeForALaterRow(t *testing.T) {
	const url = "https://www.linkedin.com/in/grace"
	e := setupImportApp(t)
	run := map[string]any{
		"mapping":    map[string]string{"Full Name": "full_name", "LinkedIn": "linkedin_url"},
		"source_key": "Full Name",
	}
	previewThenCommit(t, e, "lead", "Full Name,LinkedIn\nGrace Hopper,"+url+"\nAda Lovelace,\n", run)
	previewThenCommit(t, e, "lead", "Full Name,LinkedIn\nAda Lovelace,"+url+"\n", run)
	file := "Full Name,LinkedIn\nGrace Hopper,https://www.linkedin.com/in/grace-hopper\n" +
		"Ada Lovelace,https://www.linkedin.com/in/ada\nLinus Torvalds," + url + "\n"

	preview, final := previewThenCommit(t, e, "lead", file, run)

	want := dispositionOf(1, 0, 0, 0)
	want.Updated = 2
	assertPreviewIsTheCommit(t, preview, final, want)
}

// A typo in a company's one distinctive word still makes it a look-alike.
func TestTheDryRunSkipsALookAlikeWithATypo(t *testing.T) {
	e := setupImportApp(t)

	preview, final := previewThenCommit(t, e, "company", "Company\nMicrosoft\nMircosoft\n", map[string]any{
		"mapping": map[string]string{"Company": "display_name"}, "on_duplicate": "skip",
	})

	assertPreviewIsTheCommit(t, preview, final, dispositionOf(1, 0, 1, 1))
}

// An update that renames a company leaves it a look-alike under its new name.
func TestARenamedCompanyIsALookAlikeUnderItsNewName(t *testing.T) {
	e := setupImportApp(t)
	id := createCompanyReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})
	file := "Id,Company\n" + id + ",Nordwind Logistik\n,Nordwind Logistik GmbH\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Id": "id", "Company": "display_name"}, "on_duplicate": "skip",
	})

	want := dispositionOf(0, 0, 1, 1)
	want.Updated = 1
	assertPreviewIsTheCommit(t, preview, final, want)
}

// A company two rows rename is met under its last name only.
func TestACompanyRenamedTwiceIsNoLookAlikeOfItsFirstNewName(t *testing.T) {
	e := setupImportApp(t)
	id := createCompanyReturningID(t, e, map[string]any{"display_name": "Kestrel Data"})
	file := "Id,Company\n" + id + ",Nordwind Logistik\n" + id + ",Zephyr Freight\n,Nordwind Logistik GmbH\n"

	preview, final := previewThenCommit(t, e, "company", file, map[string]any{
		"mapping": map[string]string{"Id": "id", "Company": "display_name"}, "on_duplicate": "skip",
	})

	want := dispositionOf(1, 0, 0, 0)
	want.Updated = 2
	assertPreviewIsTheCommit(t, preview, final, want)
}
