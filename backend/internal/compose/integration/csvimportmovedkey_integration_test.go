// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A row that moves a key onto a record meets the same holders in both runs.
// The holders are the stored records and the rows of the file before it.

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// The column mappings the moved-key cases import with, one per record type.
var movedKeyLeadParts = []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": AnyMap{
	"ref": "ref", "full_name": "full_name", "email": "email", "linkedin": "linkedin_url",
}}}
var movedKeyContactParts = []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
	"ref": "ref", "full_name": "full_name", "email": "email", "linkedin": "linkedin_url",
}}}
var movedKeyCompanyParts = []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
	"ref": "ref", "id": "id", "name": "display_name", "domain": "domain",
}}}

func TestTheTestRunRefusesAMovedKeyTheImportRefuses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  func(first, second string) string
		prior map[string]string
		// link runs after the prior import, for a state no import writes.
		link  func(t *testing.T, e *apptest.AppEnv)
		parts []AnyMap
		want  []string
	}{{
		name: "an earlier row creates the company that holds the domain",
		file: func(first, _ string) string {
			return "ref,id,name,domain\nk1,,New Holder,moved.test\nk2," + first + ",Stored One,moved.test\n"
		},
		want: []string{"created=1 updated=0 unchanged=0 skipped=1", "companies.csv:3:domain_held"},
	}, {
		name: "two rows move the same domain onto two stored companies",
		file: func(first, second string) string {
			return "ref,id,name,domain\nk1," + first + ",Stored One,twin.test\nk2," + second + ",Stored Two,twin.test\n"
		},
		want: []string{"created=0 updated=1 unchanged=0 skipped=1", "companies.csv:3:domain_held"},
	}, {
		name: "a stored company already holds the domain",
		file: func(first, _ string) string {
			return "ref,id,name,domain\nk1," + first + ",Stored One,two.stored.test\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "companies.csv:2:domain_held"},
	}, {
		name:  "a stored contact holds the address a later row moves another contact onto",
		prior: map[string]string{"contacts.csv": "ref,full_name,email\nc1,Ada One,ada@moved.test\n"},
		parts: []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{"ref": "ref", "full_name": "full_name", "email": "email"}}},
		file: func(_, _ string) string {
			return "ref,full_name,email\nc1,Ada One,held@moved.test\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "contacts.csv:2:duplicate_email"},
	}, {
		name:  "a stored lead holds the address a later row moves another lead onto",
		prior: map[string]string{"leads.csv": "ref,full_name,email\nl1,Lee One,lee@moved.test\n"},
		parts: []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": AnyMap{"ref": "ref", "full_name": "full_name", "email": "email"}}},
		file: func(_, _ string) string {
			return "ref,full_name,email\nl1,Lee One,held-lead@moved.test\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "leads.csv:2:natural_key_taken"},
	}, {
		name:  "a stored lead holds the address a later row moves a contact onto",
		prior: map[string]string{"contacts.csv": "ref,full_name,email\nc1,Ada One,ada@moved.test\n"},
		parts: []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{"ref": "ref", "full_name": "full_name", "email": "email"}}},
		file: func(_, _ string) string {
			return "ref,full_name,email\nc1,Ada One,held-lead@moved.test\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "contacts.csv:2:contact_email_held"},
	}, {
		name:  "a lead takes an address of the contact it was worked from",
		prior: map[string]string{"leads.csv": "ref,full_name,email\nl1,Lee One,lee-own@moved.test\n"},
		link: func(t *testing.T, e *apptest.AppEnv) {
			contact := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Worked From", "emails": []AnyMap{
				{"email": "worked-from@moved.test", "email_type": "work", "is_primary": true},
			}})
			// Older leads carry this link; no door writes it today.
			if _, err := e.Pool.Exec(context.Background(),
				`UPDATE lead SET from_contact_id = $1 WHERE email = 'lee-own@moved.test'`, contact); err != nil {
				t.Fatalf("linking the lead to its contact: %v", err)
			}
		},
		parts: []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": AnyMap{"ref": "ref", "full_name": "full_name", "email": "email"}}},
		file: func(_, _ string) string {
			return "ref,full_name,email\nl1,Lee One,worked-from@moved.test\n"
		},
		want: []string{"created=0 updated=1 unchanged=0 skipped=0"},
	}, {
		name:  "a stored company holds the LinkedIn URL a later row moves another company onto",
		parts: []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "id": "id", "name": "display_name", "linkedin": "linkedin_url"}}},
		file: func(first, _ string) string {
			return "ref,id,name,linkedin\nk1," + first + ",Stored One,https://www.linkedin.com/company/held-co\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "companies.csv:2:linkedin_held"},
	}, {
		name:  "a stored contact holds the address a later row moves a lead onto",
		prior: map[string]string{"leads.csv": "ref,full_name,email,linkedin\nl1,Lee One,lee@moved.test,\n"},
		parts: movedKeyLeadParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nl1,Lee One,held@moved.test,\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "leads.csv:2:contact_holds_lead"},
	}, {
		name:  "a stored contact holds the profile a later row moves a lead onto",
		prior: map[string]string{"leads.csv": "ref,full_name,email,linkedin\nl1,Lee One,lee@moved.test,\n"},
		parts: movedKeyLeadParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nl1,Lee One,lee@moved.test,https://www.linkedin.com/in/held-profile\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "leads.csv:2:contact_holds_lead"},
	}, {
		name:  "a stored lead holds the profile a later row moves a contact onto",
		prior: map[string]string{"contacts.csv": "ref,full_name,email,linkedin\nc1,Ada One,ada@moved.test,\n"},
		parts: movedKeyContactParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nc1,Ada One,ada@moved.test,https://www.linkedin.com/in/held-lead-profile\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "contacts.csv:2:contact_email_held"},
	}, {
		name:  "an archived company holds no domain",
		parts: movedKeyCompanyParts,
		file: func(first, _ string) string {
			return "ref,id,name,domain\nk1," + first + ",Stored One,archived.test\n"
		},
		want: []string{"created=0 updated=1 unchanged=0 skipped=0"},
	}, {
		name:  "a Unicode address and its punycode twin are one address",
		prior: map[string]string{"contacts.csv": "ref,full_name,email,linkedin\nc2,Ada Two,ada2@moved.test,\n"},
		parts: movedKeyContactParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nc1,Ada One,info@b\u00fccher.example,\nc2,Ada Two,info@xn--bcher-kva.example,\n"
		},
		want: []string{"created=1 updated=0 unchanged=0 skipped=1", "contacts.csv:3:duplicate_email"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			storedCompany := func(name, domain string) string {
				return createdID(t, e, "/v1/companies", AnyMap{
					"source": "manual", "display_name": name,
					"domains": []AnyMap{{"domain": domain, "is_primary": true}},
				})
			}
			first := storedCompany("Stored One", "one.stored.test")
			second := storedCompany("Stored Two", "two.stored.test")
			heldEmail := []AnyMap{{"email": "held@moved.test", "email_type": "work", "is_primary": true}}
			createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Held Contact", "emails": heldEmail})
			createdID(t, e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Held Lead", "email": "held-lead@moved.test"})
			createdID(t, e, "/v1/companies", AnyMap{
				"source": "manual", "display_name": "Held Co", "linkedin_url": "https://www.linkedin.com/company/held-co",
			})
			createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Held Profile", "linkedin_url": "https://www.linkedin.com/in/held-profile"})
			createdID(t, e, "/v1/leads", AnyMap{
				"source": "manual", "full_name": "Held Lead Profile", "email": "held-lead-profile@moved.test",
				"linkedin_url": "https://www.linkedin.com/in/held-lead-profile",
			})
			archived := storedCompany("Archived Co", "archived.test")
			if status := e.Call(t, http.MethodDelete, "/v1/companies/"+archived, nil, nil, nil); status != http.StatusOK {
				t.Fatalf("archiving the company → %d, want 200", status)
			}
			parts := tc.parts
			if parts == nil {
				parts = movedKeyCompanyParts
			}
			if tc.prior != nil {
				approveAndRead(t, e, stageBundle(t, e, uploadZip(t, e, "prior.zip", zipBytes(t, tc.prior)).SourceRef, "moved", parts).ID)
			}
			if tc.link != nil {
				tc.link(t, e)
			}
			files := map[string]string{parts[0]["file"].(string): tc.file(first, second)}
			run := stageBundle(t, e, uploadZip(t, e, "moved.zip", zipBytes(t, files)).SourceRef, "moved", parts)
			predicted := verdicts(readImportReport(t, e, run.ID))
			if !slices.Equal(predicted, tc.want) {
				t.Fatalf("test run = %v, want %v", predicted, tc.want)
			}
			approveAndRead(t, e, run.ID)
			if committed := verdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
				t.Fatalf("import = %v, test run said %v", committed, predicted)
			}
		})
	}
}

func TestAResumedCommitClaimsNothingForARefusedMovedKeyRow(t *testing.T) {
	e := setupImportApp(t)
	domains := []AnyMap{{"domain": "one.stored.test", "is_primary": true}}
	stored := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Stored One", "domains": domains})
	createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Held Co", "linkedin_url": "https://www.linkedin.com/company/held-co",
	})
	var file strings.Builder
	file.WriteString("ref,id,name,domain,linkedin\nk0," + stored + ",Stored One,fresh.test,https://www.linkedin.com/company/held-co\n")
	for i := 1; i <= 500; i++ {
		fmt.Fprintf(&file, "k%d,,Filler %d,f%d.resume.test,\n", i, i, i)
	}
	file.WriteString("k501,,Late Holder,fresh.test,\n")
	parts := []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
		"ref": "ref", "id": "id", "name": "display_name", "domain": "domain", "linkedin": "linkedin_url",
	}}}
	run := stageBundle(t, e, uploadZip(t, e, "resume.zip", zipBytes(t, map[string]string{"companies.csv": file.String()})).SourceRef, "resume", parts)
	predicted := verdicts(readImportReport(t, e, run.ID))
	want := []string{"created=501 updated=0 unchanged=0 skipped=1", "companies.csv:2:linkedin_held"}
	if !slices.Equal(predicted, want) {
		t.Fatalf("test run = %v, want %v", predicted, want)
	}
	approveAndRead(t, e, run.ID)
	if committed := verdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
		t.Fatalf("import = %v, test run said %v", committed, predicted)
	}
}

// importedReport lands one file as its own run when approve is set, and
// answers the run's report either way.
func importedReport(t *testing.T, e *apptest.AppEnv, object, csv string, mapping map[string]string, approve bool) importReportDTO {
	t.Helper()
	profile, _ := importjobtest.UploadCSV(t, e, object, csv)
	run, _ := importjobtest.CreateRunWithMapping(t, e, object, profile.SourceRef, mapping)
	if approve {
		if status := importCall(t, e, run.ID, "approve", nil, nil); status != http.StatusAccepted {
			t.Fatalf("approve → %d, want 202", status)
		}
	}
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	return report
}

// A row that moves its key onto a free value previews as the update the
// commit makes. The preview leaves the stored key where it was.
func TestCSVImportPreviewsAnUpdateForARowThatMovesAFreeKey(t *testing.T) {
	e := setupImportApp(t)
	mapping := map[string]string{"Ref": "ref", "Email": "email", "Full Name": "full_name"}
	importedReport(t, e, "lead", "Ref,Email,Full Name\nR-1,first@moved.example,First\n", mapping, true)
	const moved = "Ref,Email,Full Name\nR-1,free@moved.example,First\n"

	for range 2 {
		predicted := importedReport(t, e, "lead", moved, mapping, false).Disposition
		if predicted.Updated != 1 || predicted.Skipped != 0 {
			t.Fatalf("predicted %+v, want one update: a preview that wrote would call the second one unchanged", predicted)
		}
	}
	delivered := importedReport(t, e, "lead", moved, mapping, true).Disposition
	if delivered.Updated != 1 {
		t.Errorf("delivered %+v, want one update", delivered)
	}
}
