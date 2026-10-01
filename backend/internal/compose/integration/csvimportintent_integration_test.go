// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An uploaded import source is customer PII with no row naming it until a run
// is staged, so the upload declares it provisional and the staging clears it.
// An upload nobody maps keeps its intent, which is what lets the reap find it.

import (
	"net/http"
	"testing"
)

func TestCSVImportSourceIsProvisionalUntilARunNamesIt(t *testing.T) {
	e := setupImportApp(t)

	staged, status := uploadCSV(t, e, "lead", prospectCSV)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	abandoned, status := uploadCSV(t, e, "lead", prospectCSV)
	if status != http.StatusOK {
		t.Fatalf("second upload → %d, want 200", status)
	}
	if !keyIsProvisional(t, e.DB(), staged.SourceRef) {
		t.Fatalf("upload %s left no intent, so an upload never mapped would be an orphan nothing can find", staged.SourceRef)
	}

	if _, status := createRunWithMapping(t, e, "lead", staged.SourceRef, staged.SuggestedMapping); status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	if keyIsProvisional(t, e.DB(), staged.SourceRef) {
		t.Errorf("source %s is still provisional after a run named it; the reap would delete a live import's file", staged.SourceRef)
	}
	// A source may back more than one run, and the second staging finds the
	// intent already cleared.
	if _, status := createRunWithMapping(t, e, "lead", staged.SourceRef, staged.SuggestedMapping); status != http.StatusAccepted {
		t.Errorf("second run from one source → %d, want 202", status)
	}

	if !keyIsProvisional(t, e.DB(), abandoned.SourceRef) {
		t.Errorf("abandoned upload %s lost its intent with no run naming it; the reap could never collect it", abandoned.SourceRef)
	}
}
