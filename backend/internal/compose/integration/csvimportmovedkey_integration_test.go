// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// A row that moves its address onto one another lead holds is skipped by the
// commit, and the preview has to say so.
//
// An approval decided on "one update" that delivers one skip was decided on a
// number the run never meant to honour.
//
// Keyed on a ref column, because a file keyed on the address cannot express
// this. Changing the key there makes the row a different record.
func TestCSVImportPreviewsASkipForARowThatMovesAHeldAddress(t *testing.T) {
	e := setupImportApp(t)
	const seed = "Ref,Email,Full Name\n" +
		"R-1,first@moved.example,First Lead\n" +
		"R-2,second@moved.example,Second Lead\n"
	mapping := map[string]string{"Ref": "ref", "Email": "email", "Full Name": "full_name"}

	profile, _ := uploadCSV(t, e, "lead", seed)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, mapping)
	if status := importCall(t, e, run.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("seeding approve → %d, want 202", status)
	}
	if got := leadCount(t, e); got != 2 {
		t.Fatalf("leads = %d, want the two the seed names", got)
	}

	// R-1 keeps its ref and takes the address R-2 holds.
	const moved = "Ref,Email,Full Name\n" +
		"R-1,second@moved.example,First Lead\n"
	second, _ := uploadCSV(t, e, "lead", moved)
	movedRun, _ := createRunWithMapping(t, e, "lead", second.SourceRef, mapping)

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+movedRun.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	predicted := report.Disposition

	// What the commit actually does, which the preview above has to match.
	if status := importCall(t, e, movedRun.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	var after importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+movedRun.ID+"/report", nil, nil, &after); status != http.StatusOK {
		t.Fatalf("report after the commit → %d, want 200", status)
	}

	if predicted.Skipped != after.Disposition.Skipped || predicted.Updated != after.Disposition.Updated {
		t.Errorf("predicted %+v, delivered %+v: the preview and the commit answer one question, "+
			"and an approval is decided on the preview", predicted, after.Disposition)
	}
	if predicted.Skipped != 1 {
		t.Errorf("predicted skipped = %d, want 1: the address R-2 holds is not R-1's to take",
			predicted.Skipped)
	}
}
