// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A products file is a rate card. Its rows are known by their SKU, so the same
// file uploaded again updates the products it made instead of making a second
// set. The test run and the commit answer each row alike.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type importedProductRow struct {
	ID, Name, Currency string
	SKU, Unit          *string
	PriceMinor         int64
	Active             bool
	BillingModel       *string
	Months             *int
}

// importedProducts reads the live products by SKU, and the ones with none by name.
func importedProducts(t *testing.T, e *apptest.AppEnv) map[string]importedProductRow {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT id::text, name, currency, sku, unit, unit_price_minor, active, billing_model, billing_interval_months
		  FROM product WHERE archived_at IS NULL`)
	if err != nil {
		t.Fatalf("reading the products: %v", err)
	}
	defer rows.Close()
	out := map[string]importedProductRow{}
	for rows.Next() {
		var p importedProductRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Currency, &p.SKU, &p.Unit, &p.PriceMinor, &p.Active, &p.BillingModel, &p.Months); err != nil {
			t.Fatalf("reading a product: %v", err)
		}
		key := p.Name
		if p.SKU != nil {
			key = *p.SKU
		}
		out[key] = p
	}
	return out
}

const importProductHeader = "sku,name,description,unit,unit_price,currency,default_tax_rate,tax_category,billing_model,billing_interval_months,active"

func TestAProductFileCreatesProductsAndTheSameFileAgainChangesNothing(t *testing.T) {
	e := setupImportApp(t)
	body := importProductHeader + "\n" +
		"SUP-1Y,Annual support,Four-hour reply,year,1200.50,EUR,19,standard,recurring,12,yes\n" +
		"IMP-1,Implementation day,,day,950,EUR,19,,one_time,,\n" +
		"LIC-1,Legacy licence,,,0,EUR,,,,,no\n"
	runID, preview := stageFile(t, e, "product", body)
	if preview.Disposition.Created != 3 || preview.Disposition.Skipped != 0 {
		t.Fatalf("test run = %+v issues %+v, want three products created", preview.Disposition, preview.Issues)
	}
	wantSameAnswers(t, preview, commitFile(t, e, runID))
	products := importedProducts(t, e)
	support := products["SUP-1Y"]
	if support.PriceMinor != 120050 || support.Currency != "EUR" || support.BillingModel == nil || *support.BillingModel != "recurring" ||
		support.Months == nil || *support.Months != 12 || !support.Active {
		t.Fatalf("product = %+v, want a recurring price of 1200.50 EUR every 12 months", support)
	}
	if legacy := products["LIC-1"]; legacy.Active || legacy.PriceMinor != 0 {
		t.Fatalf("product = %+v, want an inactive product with no price", legacy)
	}

	againID, again := stageFile(t, e, "product", body)
	if again.Disposition.Unchanged != 3 || again.Disposition.Created != 0 || again.Disposition.Updated != 0 {
		t.Fatalf("second test run = %+v issues %+v, want the three products unchanged", again.Disposition, again.Issues)
	}
	wantSameAnswers(t, again, commitFile(t, e, againID))
	if len(importedProducts(t, e)) != 3 {
		t.Fatalf("products = %v, want no second set", importedProducts(t, e))
	}

	changedID, changed := stageFile(t, e, "product", importProductHeader+"\n"+
		"SUP-1Y,Annual support,Four-hour reply,year,1300,EUR,19,standard,recurring,12,yes\n"+
		"IMP-1,Implementation day,,day,950,EUR,19,,recurring,,\n")
	if changed.Disposition.Updated != 1 || changed.Disposition.Skipped != 1 {
		t.Fatalf("third test run = %+v issues %+v, want one price updated and one billing shape refused", changed.Disposition, changed.Issues)
	}
	wantSameAnswers(t, changed, commitFile(t, e, changedID))
	if got := importedProducts(t, e)["SUP-1Y"].PriceMinor; got != 130000 {
		t.Fatalf("price = %d, want 130000", got)
	}
	if kept := importedProducts(t, e)["IMP-1"]; kept.BillingModel == nil || *kept.BillingModel != "one_time" {
		t.Fatalf("product = %+v, want the refused row to leave its one-off price", kept)
	}
}

func TestTheDryRunAndTheCommitAnswerEveryProductRowAlike(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/products", AnyMap{"source": "manual", "name": "Held", "sku": "HELD-1", "unit_price_minor": 100, "currency": "EUR"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the product that holds a SKU → %d", status)
	}
	runID, preview := stageFile(t, e, "product", "ref,sku,name,unit_price,currency,default_tax_rate,active\n"+
		"R1,NEW-1,Fine,10.00,EUR,19,yes\n"+
		"R2,NEW-1,Same SKU in the file,10.00,EUR,,\n"+
		"R3,HELD-1,Held by a stored product,10.00,EUR,,\n"+
		"R4,NEW-2,,10.00,EUR,,\n"+
		"R5,NEW-3,No currency,10.00,,,\n"+
		"R6,NEW-4,Bad currency,10.00,EURO,,\n"+
		"R7,NEW-5,Bad price,ten,EUR,,\n"+
		"R8,NEW-6,Rate too high,10.00,EUR,2000,\n"+
		"R9,NEW-7,Not yes or no,10.00,EUR,,maybe\n")
	if preview.Disposition.Created != 1 || preview.Disposition.Skipped != 8 {
		t.Fatalf("test run = %+v issues %+v, want one product created and eight skipped", preview.Disposition, preview.Issues)
	}
	wantSameAnswers(t, preview, commitFile(t, e, runID))
	if products := importedProducts(t, e); len(products) != 2 || products["NEW-1"].ID == "" {
		t.Fatalf("products = %v, want the held one and the one fine row", products)
	}
}

func TestUndoArchivesImportedProductsAndKeepsOneAColleagueEdited(t *testing.T) {
	e := setupImportApp(t)
	runID, _ := stageFile(t, e, "product", "sku,name,unit_price,currency\nA-1,Edited by a rep,5,EUR\nA-2,Left alone,5,EUR\n")
	commitFile(t, e, runID)
	edited := importedProducts(t, e)["A-1"].ID
	if status := e.Call(t, http.MethodPatch, "/v1/products/"+edited, AnyMap{"description": "Repriced by hand"}, nil, nil); status != http.StatusOK {
		t.Fatalf("editing the product → %d", status)
	}
	if status := importCall(t, e, runID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	var report importReportWithUndoDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+runID+"/report", nil, nil, &report)
	if report.Undo == nil || report.Undo.ReversedCount != 1 || len(report.Undo.Kept) != 1 || report.Undo.Kept[0].ID != edited {
		t.Fatalf("undo = %+v, want one product reversed and the edited one kept", report.Undo)
	}
	if left := importedProducts(t, e); len(left) != 1 || left["A-1"].ID != edited {
		t.Fatalf("products = %v, want only the edited one left", left)
	}
}
