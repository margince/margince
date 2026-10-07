// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// An export is the surface a slice larger than the list bound is read through,
// so a filter matching one row more than that bound must hand out every row.
func TestFilteredExportHandsOutEveryMatchPastTheListBound(t *testing.T) {
	e := SetupSearch(t)
	total := storekit.PredicateRowLimit + 1
	if _, err := e.Owner.Exec(t.Context(), `INSERT INTO company (id, display_name, owner_id, industry, visibility, source, captured_by)
		SELECT gen_random_uuid(), 'Bulk ' || n, $1, 'pharma', 'workspace', 'manual', 'human:x'
		FROM generate_series(1, $2) n`, e.Rep1, total); err != nil {
		t.Fatalf("seed companies: %v", err)
	}
	ctx := e.exportAdmin()
	engine, ok, err := compose.NewCollectionsStore(e.Pool).SegmentEngine(ctx, "company")
	if err != nil || !ok {
		t.Fatalf("resolve company engine: ok=%v err=%v", ok, err)
	}
	pharma := storekit.Predicate{Field: "industry", Op: "eq", Value: "pharma"}

	writer := compose.NewFilteredExportWriter(e.Pool)
	csvResult, err := writer.WriteFiltered(ctx, engine, pharma, "csv")
	if err != nil {
		t.Fatalf("csv export: %v", err)
	}
	jsonResult, err := writer.WriteFiltered(ctx, engine, pharma, "json")
	if err != nil {
		t.Fatalf("json export: %v", err)
	}
	for format, got := range map[string]int{"csv": csvResult.RowCount, "json": jsonResult.RowCount} {
		if got != total {
			t.Errorf("%s export holds %d rows, want all %d the filter matches", format, got, total)
		}
	}
}

// A slice past the export ceiling is refused with the reason, never handed out
// short.
func TestFilteredExportRefusesASliceLargerThanItCanHold(t *testing.T) {
	e := SetupSearch(t)
	total := storekit.ExportRowLimit + 1
	if _, err := e.Owner.Exec(t.Context(), `INSERT INTO company (id, display_name, owner_id, industry, visibility, source, captured_by)
		SELECT gen_random_uuid(), 'Bulk ' || n, $1, 'pharma', 'workspace', 'manual', 'human:x'
		FROM generate_series(1, $2) n`, e.Rep1, total); err != nil {
		t.Fatalf("seed companies: %v", err)
	}
	ctx := e.exportAdmin()
	engine, ok, err := compose.NewCollectionsStore(e.Pool).SegmentEngine(ctx, "company")
	if err != nil || !ok {
		t.Fatalf("resolve company engine: ok=%v err=%v", ok, err)
	}
	_, err = compose.NewFilteredExportWriter(e.Pool).WriteFiltered(ctx, engine,
		storekit.Predicate{Field: "industry", Op: "eq", Value: "pharma"}, "csv")
	fault, classified := httperr.Classify(err)
	if !classified || fault.Status != http.StatusUnprocessableEntity {
		t.Fatalf("an oversized export answered %v (classified=%v), want a 422", err, classified)
	}
}
