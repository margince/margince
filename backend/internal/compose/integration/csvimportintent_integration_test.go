// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An uploaded import source is customer PII with no row naming it until a run
// is staged, so the upload declares it provisional and the staging clears it.
// An upload nobody maps keeps its intent, which is what lets the reap find it.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/storedobject"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A source the reap condemned is gone or going, so staging a run from it is
// refused as not found — the caller uploads again — rather than reading bytes
// the reap is deleting and parking a run whose file no longer exists.
func TestStagingFromASourceTheReapCondemnedIsRefused(t *testing.T) {
	e := setupImportApp(t)
	staged, status := uploadCSV(t, e, "lead", prospectCSV)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	ledger, err := storedobject.NewLedger(compose.InstallationDB(e.Pool), compose.StoredObjectReferences()...)
	if err != nil {
		t.Fatalf("building the reap's ledger: %v", err)
	}
	system := principal.SystemActing(context.Background(), "stored_object_reap_test")
	if condemned, err := ledger.Condemn(system, time.Now().Add(48*time.Hour), staged.SourceRef); err != nil || !condemned {
		t.Fatalf("condemning the unmapped source: condemned=%v, err %v", condemned, err)
	}

	before := importRunCount(t, e)
	var refusal struct {
		Detail string `json:"detail"`
	}
	status = e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead", "source_ref": staged.SourceRef, "mapping": staged.SuggestedMapping,
	}, nil, &refusal)
	if status != http.StatusNotFound {
		t.Fatalf("staging from a condemned source → %d, want 404", status)
	}
	if !strings.Contains(refusal.Detail, "upload it again") {
		t.Errorf("the refusal does not tell the caller to upload the file again: %q", refusal.Detail)
	}
	if after := importRunCount(t, e); after != before {
		t.Errorf("a refused staging still opened %d run(s) on the condemned source", after-before)
	}
	if !keyIsProvisional(t, e.DB(), staged.SourceRef) {
		t.Error("the refused staging cleared the condemned key, so the reap would never retire it")
	}
}

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
