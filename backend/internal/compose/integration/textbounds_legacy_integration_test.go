// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/migrations"
)

func withoutTextGuards(t *testing.T, ctx context.Context) func() {
	t.Helper()
	core, err := migrations.Core()
	if err != nil {
		t.Fatal(err)
	}
	var up, down string
	for _, migration := range core.Migrations {
		if migration.Version == "1794448079" {
			up, down = migration.UpSQL, migration.DownSQL
		}
	}
	if up == "" || down == "" {
		t.Fatal("text-bound migration is absent")
	}
	owner := SchemaPool(t)
	if _, err := owner.Exec(ctx, down); err != nil {
		t.Fatal(err)
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			if _, err := owner.Exec(ctx, up); err != nil {
				t.Errorf("restore text guards: %v", err)
			}
		}
	})
	return func() {
		t.Helper()
		if _, err := owner.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
		restored = true
	}
}

func TestATranslationUpsertKeepsItsUnchangedEarlierLongName(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	product, err := e.Deals.CreateProduct(ctx, deals.CreateProductInput{Name: "Support", UnitPriceMinor: 100, Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ProductKind](ids.UUID(product.Id))
	if _, err := e.Deals.PutProductTranslation(ctx, id, "de", "Betreuung", nil); err != nil {
		t.Fatal(err)
	}
	apply := withoutTextGuards(t, ctx)
	earlier := strings.Repeat("界", values.TextLabelCharacters+1)
	// The fixture models a value written before the narrower label bound existed.
	if err := database.WithWorkspaceTx(ctx, SchemaPool(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE product_translation SET name = @name WHERE product_id = @id AND language = 'de'`, pgx.NamedArgs{"name": earlier, "id": id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	apply()
	description := "Updated description"
	kept, err := e.Deals.PutProductTranslation(ctx, id, "de", earlier, &description)
	if err != nil {
		t.Fatalf("unchanged name through the real upsert: %v", err)
	}
	if kept.Name != earlier || kept.Description == nil || *kept.Description != description {
		t.Fatalf("translation was changed or shortened: %+v", kept)
	}
	if _, err := e.Deals.PutProductTranslation(ctx, id, "de", earlier+"界", nil); err == nil {
		t.Fatal("a changed oversized label was accepted")
	}
}
