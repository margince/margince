// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The test run promises, per row, what the import then does. The two used to
// part on a file that repeats its own keys or leaves a name empty. The test
// run met only the stored records, and the import met the file's earlier rows.

import (
	"fmt"
	"slices"
	"testing"
)

// verdicts is what a report says about each row: the four counts and every
// issue by file, line and code.
func verdicts(report importReportDTO) []string {
	d := report.Disposition
	out := []string{fmt.Sprintf("created=%d updated=%d unchanged=%d skipped=%d", d.Created, d.Updated, d.Unchanged, d.Skipped)}
	for _, issue := range report.Issues {
		out = append(out, fmt.Sprintf("%s:%d:%s", issue.File, issue.Line, issue.ReasonCode))
	}
	slices.Sort(out[1:])
	return out
}

func TestTheTestRunReachesTheImportsVerdictOnEveryRow(t *testing.T) {
	contactMap := AnyMap{"ref": "ref", "full_name": "full_name", "email": "email", "phone": "phone"}
	leadMap := AnyMap{"ref": "ref", "full_name": "full_name", "email": "email", "linkedin": "linkedin_url"}
	for _, tc := range []struct {
		name  string
		files map[string]string
		parts []AnyMap
		want  []string
	}{{
		name: "a repeated address and a row with no name",
		files: map[string]string{"contacts.csv": "ref,full_name,email,phone\n" +
			"c1,Ada Byron,ada@twin.example,\n" +
			"c2,Sam Twin,twin@twin.example,\n" +
			"c3,Sam Twin Jr,twin@twin.example,\n" +
			"c4,,,+44 20 7946 0000\n"},
		parts: []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": contactMap}},
		want:  []string{"created=2 updated=0 unchanged=0 skipped=2", "contacts.csv:4:contact_email_held", "contacts.csv:5:contact_unnamed"},
	}, {
		name: "a repeated domain and a company with no name",
		files: map[string]string{"companies.csv": "ref,name,domain\n" +
			"k1,Alpha Works,alpha-twin.example\n" +
			"k2,Alpha Works Two,alpha-twin.example\n" +
			"k3,,beta-twin.example\n"},
		parts: []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name", "domain": "domain"}}},
		want:  []string{"created=1 updated=0 unchanged=0 skipped=2", "companies.csv:3:domain_held", "companies.csv:4:company_name_empty"},
	}, {
		name: "a repeated LinkedIn URL",
		files: map[string]string{"leads.csv": "ref,full_name,email,linkedin\n" +
			"l1,Lee One,lee1@twin.example,https://www.linkedin.com/in/lee-twin\n" +
			"l2,Lee Two,lee2@twin.example,https://www.linkedin.com/in/lee-twin\n"},
		parts: []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": leadMap}},
		want:  []string{"created=1 updated=0 unchanged=0 skipped=1", "leads.csv:3:linkedin_held"},
	}, {
		name: "a lead whose address a contact of the same ZIP holds",
		files: map[string]string{
			"contacts.csv": "ref,full_name,email,phone\nc1,Kim Held,kim@twin.example,\n",
			"leads.csv":    "ref,full_name,email,linkedin\nl1,Kim Lead,kim@twin.example,\nl2,Lou Lead,lou@twin.example,\n",
		},
		parts: []AnyMap{
			{"file": "contacts.csv", "object": "contact", "mapping": contactMap},
			{"file": "leads.csv", "object": "lead", "mapping": leadMap},
		},
		want: []string{"created=2 updated=0 unchanged=0 skipped=1", "leads.csv:2:contact_holds_lead"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			run := stageBundle(t, e, uploadZip(t, e, "twin.zip", zipBytes(t, tc.files)).SourceRef, "twin", tc.parts)
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
