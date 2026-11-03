// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The tax rate sheet. Rates are added per country and category and never
// changed or removed. The first rate of a category of the installation's own
// country classifies the rate card.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// taxDeskPerms keeps the sheet and the rate card, as the seeded admin does.
var taxDeskPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"tax_rate": {Create: true, Read: true},
		"product":  {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// taxRepPerms is the seeded rep: it reads the rate card and holds nothing on
// the sheet.
var taxRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects:  map[string]principal.ObjectGrant{"product": {Read: true}},
	RowScope: principal.RowScopeAll,
}

func seedTaxedProduct(ctx context.Context, t *testing.T, e *Env, name string, rate float64) ids.ProductID {
	t.Helper()
	product, err := e.Deals.CreateProduct(ctx, deals.CreateProductInput{
		Name: name, UnitPriceMinor: 10000, Currency: "EUR", DefaultTaxRate: &rate, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return ids.From[ids.ProductKind](ids.UUID(product.Id))
}

func taxCategoryOf(ctx context.Context, t *testing.T, e *Env, id ids.ProductID) string {
	t.Helper()
	product, err := e.Deals.GetProduct(ctx, id, storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("read the product: %v", err)
	}
	if product.TaxCategory == nil {
		return "unclassified"
	}
	return string(*product.TaxCategory)
}

func record(ctx context.Context, e *Env, country, category, rate string, from time.Time) (deals.TaxRate, int, error) {
	return e.Deals.RecordTaxRate(ctx, deals.RecordTaxRateInput{Country: country, Category: category, Rate: rate, ValidFrom: from})
}

func TestTheFirstRateOfACategoryClassifiesTheRateCard(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `INSERT INTO setting (key, value, updated_at) VALUES ('installation.country', '"de"'::jsonb, now())`)
	desk := e.As(e.AdminUser, nil, taxDeskPerms)
	standard := seedTaxedProduct(desk, t, e, "Consulting", 19)
	reduced := seedTaxedProduct(desk, t, e, "Books", 7)
	zero := seedTaxedProduct(desk, t, e, "Donation", 0)
	odd := seedTaxedProduct(desk, t, e, "Old rate", 16)
	// Tomorrow in every zone, so no installation reads it as a past day.
	today := clocktest.Now(t).UTC().AddDate(0, 0, 2).Truncate(24 * time.Hour)

	// A rate for another country says nothing about products, which have none.
	if _, n, err := record(desk, e, "at", "standard", "19", today); err != nil || n != 0 {
		t.Fatalf("Austria's standard rate: classified %d (%v), want none", n, err)
	}
	if _, n, err := record(desk, e, "DE", "standard", "19", today); err != nil || n != 1 {
		t.Fatalf("Germany's first standard rate: classified %d (%v), want the one 19%% product", n, err)
	}
	if _, n, err := record(desk, e, "de", "reduced", "7.00", today); err != nil || n != 1 {
		t.Fatalf("Germany's first reduced rate: classified %d (%v), want the one 7%% product", n, err)
	}
	later := seedTaxedProduct(desk, t, e, "Later consulting", 19)
	if _, n, err := record(desk, e, "de", "standard", "19", today.AddDate(0, 1, 0)); err != nil || n != 0 {
		t.Fatalf("a later standard rate: classified %d (%v), want none: only the first rate classifies", n, err)
	}
	for id, want := range map[ids.ProductID]string{
		standard: "standard", reduced: "reduced", zero: "unclassified", odd: "unclassified", later: "unclassified",
	} {
		if got := taxCategoryOf(desk, t, e, id); got != want {
			t.Errorf("product %s is %s, want %s", id, got, want)
		}
	}
	unclassified := "unclassified"
	listed, _, err := e.Deals.ListProducts(desk, deals.ListProductsInput{TaxCategory: &unclassified})
	if err != nil || len(listed) != 3 {
		t.Errorf("the unclassified list holds %d products (%v), want the 0%%, the 16%% and the later one", len(listed), err)
	}
}

func TestTheSheetOnlyGrows(t *testing.T) {
	e := Setup(t)
	desk := e.As(e.AdminUser, nil, taxDeskPerms)
	day := func(text string) time.Time {
		parsed, err := time.Parse(time.DateOnly, text)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	if _, _, err := record(desk, e, "de", "standard", "16", day("2020-07-01")); err != nil {
		t.Fatalf("the first rate, in the past: %v", err)
	}
	if _, _, err := record(desk, e, "de", "standard", "19", day("2021-01-01")); err == nil {
		t.Error("a past rate after the first one was recorded")
	}
	if _, _, err := record(desk, e, "de", "standard", "15", day("2019-01-01")); err != nil {
		t.Errorf("a rate before the first one extends the history and was refused: %v", err)
	}
	later := clocktest.Now(t).UTC().AddDate(0, 0, 2).Truncate(24 * time.Hour)
	if _, _, err := record(desk, e, "de", "standard", "19", later); err != nil {
		t.Errorf("a rate from a later day was refused: %v", err)
	}
	if _, _, err := record(desk, e, "de", "standard", "20", later); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a second rate for the same day answered %v, want a conflict", err)
	}
	if err := e.WsExecErr(t, `UPDATE tax_rate SET rate = 1`); err == nil {
		t.Error("the table changed a recorded rate")
	}
	if err := e.WsExecErr(t, `DELETE FROM tax_rate`); err == nil {
		t.Error("the table removed a recorded rate")
	}
	rates, err := e.Deals.ListTaxRates(desk, new("DE"))
	if err != nil || len(rates) != 3 || rates[0].Rate != "19.00" || rates[2].Rate != "15.00" {
		t.Errorf("Germany's sheet reads %+v (%v), want three rates, newest first", rates, err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'tax_rate' AND action = 'create'`); got != 3 {
		t.Errorf("%d tax rates were audited, want 3", got)
	}
}

func TestARepReadsNoTaxRateAndRecordsNone(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, taxRepPerms)
	if _, err := e.Deals.ListTaxRates(rep, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep reading the sheet answered %v, want permission denied", err)
	}
	if _, _, err := record(rep, e, "de", "standard", "19", clocktest.Now(t).UTC()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep recording a rate answered %v, want permission denied", err)
	}
}
