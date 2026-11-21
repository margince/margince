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
		name string
		file func(first, second string) string
		want []string
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
			files := map[string]string{"companies.csv": tc.file(first, second)}
			parts := []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
				"ref": "ref", "id": "id", "name": "display_name", "domain": "domain",
			}}}
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
