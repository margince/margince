// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a mapping must satisfy before a run exists. Separate from the
// end-to-end file next door because this is the REQUEST being judged, not the
// import: nothing here is ever staged, and the assertions are about what the
// caller is told rather than what the estate holds afterwards.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A mapping may name a column the file does not carry — a model that guessed at
// a header, a screen built against a different export. The row builder DROPS
// such a column, so the field it named would import empty on every row while
// the preview reported clean creates and the commit landed them. It is refused
// at the door instead, and the refusal carries the file's own header, because a
// caller driving this over MCP cannot open the file to read it.
func TestCSVImportRefusesAMappingNamingAColumnTheFileLacks(t *testing.T) {
	e := setupImportApp(t)

	const file = "Company,Employees\nNordwind Logistik,240\n"
	profile, _ := uploadCSV(t, e, "company", file)

	before := importRunCount(t, e)
	var refusal struct {
		Detail string `json:"detail"`
	}
	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "company", "source_ref": profile.SourceRef,
		"mapping": map[string]string{"Company": "display_name", "Firma": "legal_name"},
	}, nil, &refusal)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a mapping naming a column the file lacks → %d, want 422", status)
	}
	for _, name := range []string{"Firma", "Company", "Employees"} {
		if !strings.Contains(refusal.Detail, name) {
			t.Errorf("the refusal does not name %q, so the caller cannot correct it: %q", name, refusal.Detail)
		}
	}
	// A request fault stages nothing. Left behind, the run would sit in
	// `validating` with neither approve nor resume able to move it.
	if after := importRunCount(t, e); after != before {
		t.Errorf("the refused mapping left %d run(s) behind; a request fault must stage nothing", after-before)
	}

	// The same file, mapped onto the columns it actually carries, still stages
	// and still previews — the refusal is about the name, not the file.
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef,
		map[string]string{"Company": "display_name"})
	if status != http.StatusAccepted {
		t.Fatalf("the honest mapping of the same file → %d, want 202", status)
	}
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 1 {
		t.Errorf("created = %d, want 1 — the file is importable, the column name was not",
			report.Disposition.Created)
	}
}

// importRunCount reads the run tally from the table rather than a list
// endpoint, because what is asserted is that a refused request wrote no row at
// all — including one no read surface would show.
func importRunCount(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var runs int
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM import_run`).Scan(&runs)
	}); err != nil {
		t.Fatalf("counting import runs: %v", err)
	}
	return runs
}
