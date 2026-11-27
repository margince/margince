// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"slices"
	"strings"
	"testing"
)

// Minor units of two currencies are not one scale, so the price order groups
// by currency first and pages through each currency's amounts in turn.
func TestProductPriceSortGroupsByCurrencyAcrossPages(t *testing.T) {
	t.Parallel()
	e := setupReviewEnv(t)
	// The package's tests share one catalogue, so the list is narrowed to this test's products.
	query := e.ws.String()
	token := " " + query
	seeds := []struct {
		name     string
		currency string
		minor    int64
	}{
		{"Dinar item", "KWD", 12345},
		{"Large widget", "EUR", 7500},
		{"Widget", "EUR", 3333},
		{"Yen item", "JPY", 1500},
		{"Small widget", "EUR", 1000},
	}
	for _, seed := range seeds {
		if _, err := e.store.CreateProduct(e.productCtx(), CreateProductInput{
			Name: seed.name + token, UnitPriceMinor: seed.minor, Currency: seed.currency, Source: "manual",
		}); err != nil {
			t.Fatalf("seeding product %s: %v", seed.name, err)
		}
	}

	var got []string
	limit := 2
	sort := "-" + productPriceColumn
	var cursor *string
	for range len(seeds) {
		products, page, err := e.store.ListProducts(e.productCtx(), ListProductsInput{
			Sort: &sort, Limit: &limit, Cursor: cursor, Query: &query,
		})
		if err != nil {
			t.Fatalf("listing products by price: %v", err)
		}
		for _, p := range products {
			got = append(got, strings.TrimSuffix(p.Name, token))
		}
		if !page.HasMore {
			break
		}
		cursor = &page.NextCursor
	}
	want := []string{"Dinar item", "Yen item", "Large widget", "Widget", "Small widget"}
	if !slices.Equal(got, want) {
		t.Fatalf("descending by price read %v, want %v", got, want)
	}
}
