// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/contacts"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/migrations"
)

func withoutTextGuards(t *testing.T, ctx context.Context) func() {
	t.Helper()
	// Applied in this order, and removed in the reverse one: the shared functions,
	// the table guards, the custom-field guards, then the custom track on top.
	var ups, downs []string
	for _, pair := range []struct {
		namespace func() (dbmigrate.Namespace, error)
		version   string
	}{{migrations.Core, "1794600200"}, {migrations.Core, "1794600201"}, {migrations.Core, "1794600202"}, {migrations.Custom, "20261112130500"}} {
		up, down := migrationPair(t, pair.namespace, pair.version)
		ups = append(ups, up)
		downs = append([]string{down}, downs...)
	}
	owner := SchemaPool(t)
	for _, step := range downs {
		if _, err := owner.Exec(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	restore := func() error {
		for _, step := range ups {
			if _, err := owner.Exec(ctx, step); err != nil {
				return err
			}
		}
		return nil
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			if err := restore(); err != nil {
				t.Errorf("restore text guards: %v", err)
			}
		}
	})
	return func() {
		t.Helper()
		if err := restore(); err != nil {
			t.Fatal(err)
		}
		restored = true
	}
}

func migrationPair(t *testing.T, namespace func() (dbmigrate.Namespace, error), version string) (up, down string) {
	t.Helper()
	migrationSet, err := namespace()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrationSet.Migrations {
		if migration.Version == version {
			up, down = migration.UpSQL, migration.DownSQL
		}
	}
	if up == "" || down == "" {
		t.Fatalf("text-bound migration %s is absent", version)
	}
	return up, down
}

func TestATranslationUpsertKeepsItsUnchangedEarlierLongName(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, offerLanguagePerms)
	id := seedRetainer(t, e)
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

func TestACompanyEditKeepsItsUnchangedEarlierLongDescription(t *testing.T) {
	f := setupCFV(t)
	created, err := f.store.CreateCompany(f.ctx, contacts.CreateCompanyInput{DisplayName: "Earlier description", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := companyIDOf(ids.UUID(created.Id))
	owner := SchemaPool(t)
	apply := withoutTextGuards(t, f.ctx)
	// The fixture models a description written before the column had a bound.
	earlier := strings.Repeat("界", contacts.CompanyDescriptionMax+1)
	row := pgx.NamedArgs{"id": id, "description": earlier}
	if _, err := owner.Exec(f.ctx, `ALTER TABLE public.company DROP CONSTRAINT company_description_length`); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(f.ctx, `UPDATE public.company SET description = @description WHERE id = @id`, row); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(f.ctx, `UPDATE public.company SET description = NULL WHERE id = @id`, row); err != nil {
			t.Errorf("clear the earlier description: %v", err)
		}
		if _, err := owner.Exec(f.ctx, `ALTER TABLE public.company ADD CONSTRAINT company_description_length
			CHECK (description IS NULL OR length(description) <= 20000)`); err != nil {
			t.Errorf("restore the description bound: %v", err)
		}
	})
	apply()
	renamed := "Renamed with its earlier description"
	if _, err := f.store.UpdateCompany(f.ctx, id, contacts.UpdateCompanyInput{DisplayName: &renamed, Description: &earlier}); err != nil {
		t.Fatalf("an unchanged earlier description was refused: %v", err)
	}
	changed := earlier + "界"
	if _, err := f.store.UpdateCompany(f.ctx, id, contacts.UpdateCompanyInput{Description: &changed}); err == nil {
		t.Fatal("a changed oversized description was accepted")
	}
}

func TestATypedContactNamePastTheLabelLimitIsRefusedForEveryRole(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	tooLong := strings.Repeat("界", values.TextLabelCharacters+1)
	var refused problemWire
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": tooLong}, nil, &refused); status != http.StatusUnprocessableEntity {
		t.Fatalf("an admin's typed create with a %d-character name → %d, want 422", values.TextLabelCharacters+1, status)
	}
	assertTooLong(t, refused, "full_name")
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Typed Name"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	refused = problemWire{}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+contact.ID, AnyMap{"full_name": tooLong}, nil, &refused); status != http.StatusUnprocessableEntity {
		t.Fatalf("an admin's typed rename with a %d-character name → %d, want 422", values.TextLabelCharacters+1, status)
	}
	assertTooLong(t, refused, "full_name")
}

func assertTooLong(t *testing.T, refused problemWire, field string) {
	t.Helper()
	if len(refused.Details.Errors) != 1 || refused.Details.Errors[0].Field != field || refused.Details.Errors[0].Code != "too_long" {
		t.Fatalf("errors %+v, want one too_long naming %s", refused.Details.Errors, field)
	}
}
