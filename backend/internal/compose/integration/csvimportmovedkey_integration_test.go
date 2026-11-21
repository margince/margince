// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// importedReport lands one file as its own run when approve is set, and
// answers the run's report either way.
func importedReport(t *testing.T, e *apptest.AppEnv, object, csv string, mapping map[string]string, approve bool) importReportDTO {
	t.Helper()
	profile, _ := importjobtest.UploadCSV(t, e, object, csv)
	run, _ := importjobtest.CreateRunWithMapping(t, e, object, profile.SourceRef, mapping)
	var report importReportDTO
	if !approve {
		if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
			t.Fatalf("report → %d, want 200", status)
		}
		return report
	}
	if status := importCall(t, e, run.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report after the commit → %d, want 200", status)
	}
	return report
}

// A row that moves a key onto a value another record holds is skipped by the
// commit, and the preview says so. Each file is keyed on a ref column, because
// a file keyed on the moved key names a different record instead.
func TestCSVImportPreviewsASkipForARowThatMovesAHeldKey(t *testing.T) {
	byAddress := map[string]string{"Ref": "ref", "Email": "email", "Full Name": "full_name"}
	byDomain := map[string]string{"Ref": "ref", "Name": "display_name", "Domain": "domain"}
	for _, tc := range []struct {
		name, object, holderObject string
		mapping                    map[string]string
		holder, seed, moved        string
	}{
		{
			name: "a lead takes another lead's address", object: "lead", holderObject: "lead", mapping: byAddress,
			holder: "Ref,Email,Full Name\nH-1,held@moved.example,Holder\n",
			seed:   "Ref,Email,Full Name\nR-1,first@moved.example,First\n",
			moved:  "Ref,Email,Full Name\nR-1,held@moved.example,First\n",
		},
		{
			name: "a contact takes another contact's address", object: "contact", holderObject: "contact", mapping: byAddress,
			holder: "Ref,Email,Full Name\nH-1,held@moved.example,Holder\n",
			seed:   "Ref,Email,Full Name\nR-1,first@moved.example,First\n",
			moved:  "Ref,Email,Full Name\nR-1,held@moved.example,First\n",
		},
		{
			name: "a contact takes a lead's address", object: "contact", holderObject: "lead", mapping: byAddress,
			holder: "Ref,Email,Full Name\nH-1,held@moved.example,Holder\n",
			seed:   "Ref,Email,Full Name\nR-1,first@moved.example,First\n",
			moved:  "Ref,Email,Full Name\nR-1,held@moved.example,First\n",
		},
		{
			name: "a lead takes a contact's address", object: "lead", holderObject: "contact", mapping: byAddress,
			holder: "Ref,Email,Full Name\nH-1,held@moved.example,Holder\n",
			seed:   "Ref,Email,Full Name\nR-1,first@moved.example,First\n",
			moved:  "Ref,Email,Full Name\nR-1,held@moved.example,First\n",
		},
		{
			name: "a company takes another company's domain", object: "company", holderObject: "company", mapping: byDomain,
			holder: "Ref,Name,Domain\nH-1,Holder GmbH,held-moved.example\n",
			seed:   "Ref,Name,Domain\nR-1,First GmbH,first-moved.example\n",
			moved:  "Ref,Name,Domain\nR-1,First GmbH,held-moved.example\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			importedReport(t, e, tc.holderObject, tc.holder, tc.mapping, true)
			importedReport(t, e, tc.object, tc.seed, tc.mapping, true)

			predicted := importedReport(t, e, tc.object, tc.moved, tc.mapping, false).Disposition
			delivered := importedReport(t, e, tc.object, tc.moved, tc.mapping, true).Disposition

			if predicted.Skipped != delivered.Skipped || predicted.Updated != delivered.Updated {
				t.Errorf("predicted %+v, delivered %+v: an approval is decided on the preview", predicted, delivered)
			}
			if predicted.Skipped != 1 || predicted.Updated != 0 {
				t.Errorf("predicted %+v, want one skip and no update: the key is another record's", predicted)
			}
		})
	}
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
