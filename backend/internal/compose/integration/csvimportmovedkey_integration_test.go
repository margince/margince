// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A row that moves a key onto a record meets the same holders in both runs.
// The holders are the stored records and the rows of the file before it.

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestTheTestRunRefusesAMovedKeyTheImportRefuses(t *testing.T) {
	leadParts := []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": AnyMap{
		"ref": "ref", "full_name": "full_name", "email": "email", "linkedin": "linkedin_url",
	}}}
	contactParts := []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
		"ref": "ref", "full_name": "full_name", "email": "email", "linkedin": "linkedin_url",
	}}}
	companyParts := []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
		"ref": "ref", "id": "id", "name": "display_name", "domain": "domain",
	}}}
	for _, tc := range []struct {
		name  string
		file  func(first, second string) string
		prior map[string]string
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
		name:  "a stored company holds the LinkedIn URL a later row moves another company onto",
		parts: []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "id": "id", "name": "display_name", "linkedin": "linkedin_url"}}},
		file: func(first, _ string) string {
			return "ref,id,name,linkedin\nk1," + first + ",Stored One,https://www.linkedin.com/company/held-co\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "companies.csv:2:linkedin_held"},
	}, {
		name:  "a stored contact holds the address a later row moves a lead onto",
		prior: map[string]string{"leads.csv": "ref,full_name,email,linkedin\nl1,Lee One,lee@moved.test,\n"},
		parts: leadParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nl1,Lee One,held@moved.test,\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "leads.csv:2:contact_holds_lead"},
	}, {
		name:  "a stored contact holds the profile a later row moves a lead onto",
		prior: map[string]string{"leads.csv": "ref,full_name,email,linkedin\nl1,Lee One,lee@moved.test,\n"},
		parts: leadParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nl1,Lee One,lee@moved.test,https://www.linkedin.com/in/held-profile\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "leads.csv:2:contact_holds_lead"},
	}, {
		name:  "a stored lead holds the profile a later row moves a contact onto",
		prior: map[string]string{"contacts.csv": "ref,full_name,email,linkedin\nc1,Ada One,ada@moved.test,\n"},
		parts: contactParts,
		file: func(_, _ string) string {
			return "ref,full_name,email,linkedin\nc1,Ada One,ada@moved.test,https://www.linkedin.com/in/held-lead-profile\n"
		},
		want: []string{"created=0 updated=0 unchanged=0 skipped=1", "contacts.csv:2:contact_email_held"},
	}, {
		name:  "an archived company holds no domain",
		parts: companyParts,
		file: func(first, _ string) string {
			return "ref,id,name,domain\nk1," + first + ",Stored One,archived.test\n"
		},
		want: []string{"created=0 updated=1 unchanged=0 skipped=0"},
	}, {
		name:  "a Unicode address and its punycode twin are one address",
		prior: map[string]string{"contacts.csv": "ref,full_name,email,linkedin\nc2,Ada Two,ada2@moved.test,\n"},
		parts: contactParts,
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
				parts = companyParts
			}
			if tc.prior != nil {
				approveAndRead(t, e, stageBundle(t, e, uploadZip(t, e, "prior.zip", zipBytes(t, tc.prior)).SourceRef, "moved", parts).ID)
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
