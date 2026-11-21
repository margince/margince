// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A file may name one company or contact twice under two spellings. The import
// lands its rows one at a time and meets the earlier spelling as a stored
// record. So the test run counts that duplicate before anyone approves.

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

func TestTheTestRunCountsLookAlikesInsideTheFile(t *testing.T) {
	companyMap := map[string]string{"Company": "display_name", "Street": "address.line1", "City": "address.city"}
	contactMap := map[string]string{"Name": "full_name", "Email": "email"}
	for _, tc := range []struct {
		name, object, onDuplicate, file string
		mapping                         map[string]string
		// stored is a company the CRM holds before the file arrives.
		stored string
		want   []string
	}{{
		name: "one brand twice, skipped", object: "company", onDuplicate: "skip", mapping: companyMap,
		file: "Company,Street,City\nSkipco Trading,,\nSkipco Trading GmbH,,\nNordwind Logistik,,\n",
		want: []string{"created=2 updated=0 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "one brand twice, kept", object: "company", onDuplicate: "create", mapping: companyMap,
		file: "Company,Street,City\nSkipco Trading,,\nSkipco Trading GmbH,,\nNordwind Logistik,,\n",
		want: []string{"created=3 updated=0 unchanged=0 skipped=0 duplicates=1", "line 3: company_duplicate_found"},
	}, {
		name: "a stored look-alike's twin at one street, kept", object: "company", onDuplicate: "create", mapping: companyMap,
		stored: "Kestrel Data",
		file:   "Company,Street,City\nKestrel Data GmbH,Marienplatz 8,München\nKestrel Datta,Marienplatz 8,München\n",
		want:   []string{"created=2 updated=0 unchanged=0 skipped=0 duplicates=2", "line 2: company_duplicate_found", "line 3: company_duplicate_found"},
	}, {
		name: "a stored look-alike's twin at one street, skipped", object: "company", onDuplicate: "skip", mapping: companyMap,
		stored: "Kestrel Data",
		file:   "Company,Street,City\nKestrel Data GmbH,Marienplatz 8,München\nKestrel Datta,Marienplatz 8,München\n",
		want:   []string{"created=1 updated=0 unchanged=0 skipped=1 duplicates=1", "line 2: company_duplicate_skipped"},
	}, {
		name: "one contact twice at one employer, skipped", object: "contact", onDuplicate: "skip", mapping: contactMap,
		file: "Name,Email\nAnna Schmidt,anna.schmidt@twinco.example\nAnna Schmitt,a.schmitt@twinco.example\nBo Lind,bo@lindco.example\n",
		want: []string{"created=2 updated=0 unchanged=0 skipped=1 duplicates=1", "line 3: contact_duplicate_skipped"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			if tc.stored != "" {
				if status := e.Call(t, http.MethodPost, "/v1/companies",
					map[string]any{"source": "manual", "display_name": tc.stored}, nil, nil); status != http.StatusCreated {
					t.Fatalf("storing %s → %d, want 201", tc.stored, status)
				}
			}
			profile, _ := importjobtest.UploadCSV(t, e, tc.object, tc.file)
			run, status := createRunOnDuplicate(t, e, tc.object, profile.SourceRef, tc.mapping, tc.onDuplicate)
			if status != http.StatusAccepted {
				t.Fatalf("create run → %d, want 202", status)
			}
			predicted := duplicateVerdicts(readImportReport(t, e, run.ID))
			if !slices.Equal(predicted, tc.want) {
				t.Fatalf("test run = %v, want %v", predicted, tc.want)
			}
			approveAndRead(t, e, run.ID)
			if committed := duplicateVerdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
				t.Fatalf("import = %v, test run said %v", committed, predicted)
			}
		})
	}
}

// duplicateVerdicts is verdicts with the duplicate count, which sits outside
// the four outcomes, and each issue by line and code alone.
func duplicateVerdicts(report importReportDTO) []string {
	d := report.Disposition
	duplicates := 0
	if d.Duplicates != nil {
		duplicates = *d.Duplicates
	}
	out := []string{fmt.Sprintf("created=%d updated=%d unchanged=%d skipped=%d duplicates=%d",
		d.Created, d.Updated, d.Unchanged, d.Skipped, duplicates)}
	for _, issue := range report.Issues {
		out = append(out, fmt.Sprintf("line %d: %s", issue.Line, issue.ReasonCode))
	}
	slices.Sort(out[1:])
	return out
}
