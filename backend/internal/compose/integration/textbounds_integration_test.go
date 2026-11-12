// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/migrations"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestNewCustomTextFieldsKeepTheSharedDocumentBound(t *testing.T) {
	f := setupCFV(t)
	column := f.defineField(t, customfields.FieldSpec{Object: "contact", Label: "Long note", Type: customfields.TypeText, Source: "manual"})
	atLimit := strings.Repeat("界", values.TextDocumentCharacters)
	created, err := f.store.CreateContact(f.ctx, contacts.CreateContactInput{
		FullName: "Text boundary", Source: "manual", CustomFields: map[string]any{column: atLimit},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCF(t, created.AdditionalProperties, column, atLimit)
	id := ContactIDOf(ids.UUID(created.Id))
	if _, err := f.store.UpdateContact(f.ctx, id, contacts.UpdateContactInput{CustomFields: map[string]any{column: atLimit + "界"}}); err == nil {
		t.Fatal("an oversized custom value was stored")
	}
	kept, err := f.store.GetContact(f.ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertCF(t, kept.AdditionalProperties, column, atLimit)
}

func TestCompanyDescriptionsAdmitTheWholeDocumentWithoutTruncation(t *testing.T) {
	f := setupCFV(t)
	atLimit := strings.Repeat("界", values.TextDocumentCharacters)
	created, err := f.store.CreateCompany(f.ctx, contacts.CreateCompanyInput{
		DisplayName: "Description boundary", Source: "manual", Description: &atLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Description == nil || *created.Description != atLimit {
		t.Fatal("the description was lost or shortened")
	}
	id := companyIDOf(ids.UUID(created.Id))
	tooLong := atLimit + "界"
	if _, err := f.store.UpdateCompany(f.ctx, id, contacts.UpdateCompanyInput{Description: &tooLong}); err == nil {
		t.Fatal("an oversized description was stored")
	}
	kept, err := f.store.GetCompany(f.ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Description == nil || *kept.Description != atLimit {
		t.Fatal("a refused change altered the saved description")
	}
}

func TestAnUpgradeBoundsCustomTextWithoutChangingEarlierValues(t *testing.T) {
	f := setupCFV(t)
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
	if _, err := owner.Exec(f.ctx, down); err != nil {
		t.Fatal(err)
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			if _, err := owner.Exec(f.ctx, up); err != nil {
				t.Errorf("restore text guards: %v", err)
			}
		}
	})
	column := f.defineField(t, customfields.FieldSpec{Object: "contact", Label: "Earlier note", Type: customfields.TypeText, Source: "manual"})
	// Model the earlier schema, whose runtime text columns carried no length check.
	var constraint string
	if err := owner.QueryRow(f.ctx, `SELECT conname FROM pg_catalog.pg_constraint
  WHERE conrelid = 'public.contact'::regclass AND contype = 'c'
  AND conkey = ARRAY[(SELECT attnum FROM pg_catalog.pg_attribute WHERE attrelid = 'public.contact'::regclass AND attname = @column)]`, pgx.NamedArgs{"column": column}).Scan(&constraint); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(f.ctx, fmt.Sprintf("ALTER TABLE public.contact DROP CONSTRAINT %s", pgx.Identifier{constraint}.Sanitize())); err != nil {
		t.Fatal(err)
	}
	earlier := strings.Repeat("界", values.TextDocumentCharacters+1)
	created, err := f.store.CreateContact(f.ctx, contacts.CreateContactInput{FullName: "Earlier custom text", Source: "manual", CustomFields: map[string]any{column: earlier}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(f.ctx, up); err != nil {
		t.Fatal(err)
	}
	restored = true
	id := ContactIDOf(ids.UUID(created.Id))
	renamed := "Updated contact"
	if _, err := f.store.UpdateContact(f.ctx, id, contacts.UpdateContactInput{FullName: &renamed, CustomFields: map[string]any{column: earlier}}); err != nil {
		t.Fatalf("unchanged earlier text: %v", err)
	}
	if _, err := f.store.UpdateContact(f.ctx, id, contacts.UpdateContactInput{CustomFields: map[string]any{column: earlier + "界"}}); err == nil {
		t.Fatal("a changed oversized custom value was accepted")
	}
	kept, err := f.store.GetContact(f.ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertCF(t, kept.AdditionalProperties, column, earlier)
	if kept.FullName != renamed {
		t.Fatalf("contact name = %q", kept.FullName)
	}
}
