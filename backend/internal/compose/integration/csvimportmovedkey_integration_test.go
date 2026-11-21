// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A row that moves a key onto a record meets the same holders in both runs.
// The holders are the stored records and the rows of the file before it.

import (
	"slices"
	"testing"
)

func TestTheTestRunRefusesAMovedKeyTheImportRefuses(t *testing.T) {
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
			createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Held Contact",
				"emails": []AnyMap{{"email": "held@moved.test", "email_type": "work", "is_primary": true}}})
			createdID(t, e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Held Lead", "email": "held-lead@moved.test"})
			createdID(t, e, "/v1/companies", AnyMap{
				"source": "manual", "display_name": "Held Co", "linkedin_url": "https://www.linkedin.com/company/held-co",
			})
			parts := tc.parts
			if parts == nil {
				parts = []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
					"ref": "ref", "id": "id", "name": "display_name", "domain": "domain",
				}}}
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
