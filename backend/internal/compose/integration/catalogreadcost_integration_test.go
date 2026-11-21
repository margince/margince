// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What the custom-field catalog costs a record read or write that carries it.
// Each cost is the same call with the catalog wired minus without it, so the
// difference is the catalog read and nothing else.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// catalogWithFields defines one text field on each object named, so the
// catalog has an answer to read.
func catalogWithFields(t *testing.T, e *Env, objects ...string) *customfields.Service {
	t.Helper()
	catalog := customfields.NewService(e.Pool, SchemaPool(t))
	for _, object := range objects {
		if _, err := catalog.Create(e.As(e.Rep1, nil, CustomFieldAdminPerms), customfields.FieldSpec{
			Object: object, Label: "Region " + object, Type: customfields.TypeText, Source: "manual",
		}); err != nil {
			t.Fatalf("define a %s field: %v", object, err)
		}
	}
	return catalog
}

// metered answers what run cost on the statement meter.
func metered(t *testing.T, ctx context.Context, run func(context.Context) error) database.StatementCost {
	t.Helper()
	ctx, meter := database.WithStatementMeter(ctx)
	if err := run(ctx); err != nil {
		t.Fatal(err)
	}
	return meter.Cost()
}

// assertOneTransactionWithOneCatalogStatement holds a store-opened entry point
// to one transaction and one pool checkout, the catalog read being one statement in it.
func assertOneTransactionWithOneCatalogStatement(t *testing.T, call string, with, without database.StatementCost) {
	t.Helper()
	t.Logf("%s: %d statements, %d transactions, %d acquires with the catalog; %d statements without",
		call, with.Statements, with.Transactions, with.Acquires, without.Statements)
	if with.Transactions != 1 || with.Acquires != 1 {
		t.Errorf("%s opened %d transactions on %d pool checkouts, want 1 on 1: the catalog read must ride the store's transaction",
			call, with.Transactions, with.Acquires)
	}
	if got := with.Statements - without.Statements; got != 1 {
		t.Errorf("%s spent %d statements on the catalog, want 1", call, got)
	}
}

func TestAStoreReadsTheCatalogOnItsOwnTransaction(t *testing.T) {
	e := Setup(t)
	catalog := catalogWithFields(t, e, "deal", "contact")
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Catalog cost", pipeline, open, &e.Rep1))
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Catalog Cost"})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))

	getDeal := func(store *deals.Store) func(context.Context) error {
		return func(ctx context.Context) error {
			_, err := store.GetDeal(ctx, deal, storekit.LiveOnly)
			return err
		}
	}
	assertOneTransactionWithOneCatalogStatement(t, "GetDeal",
		metered(t, e.Admin(), getDeal(deals.NewStore(e.DB(), installseam.Deals()).WithFieldCatalog(catalog))),
		metered(t, e.Admin(), getDeal(deals.NewStore(e.DB(), installseam.Deals()))))

	title := 0
	updateContact := func(store *contacts.Store) func(context.Context) error {
		return func(ctx context.Context) error {
			title++
			_, err := store.UpdateContact(ctx, contactID, contacts.UpdateContactInput{Title: new(fmt.Sprintf("Title %d", title))})
			return err
		}
	}
	assertOneTransactionWithOneCatalogStatement(t, "UpdateContact",
		metered(t, e.Admin(), updateContact(contacts.NewStore(e.DB()).WithFieldCatalog(catalog))),
		metered(t, e.Admin(), updateContact(contacts.NewStore(e.DB()))))
}

func TestTenWritesInOneTransactionReadTheCatalogOnce(t *testing.T) {
	e := Setup(t)
	catalog := catalogWithFields(t, e, "contact")
	contactIDs := make([]ids.UUID, 10)
	for i := range contactIDs {
		created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: fmt.Sprintf("Catalog Cost %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		contactIDs[i] = ids.UUID(created.Id)
	}

	round := 0
	updateAll := func(provider *contacts.Provider) func(context.Context) error {
		round++
		return func(ctx context.Context) error {
			return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				for _, id := range contactIDs {
					write, err := provider.PrepareUpdateTx(ctx, datasource.EntityContact)
					if err != nil {
						return err
					}
					if _, err := write(ctx, tx, datasource.UpdateInput{
						Ref:   datasource.EntityRef{Type: datasource.EntityContact, ID: id},
						Patch: map[string]any{"title": fmt.Sprintf("Round %d", round)},
					}); err != nil {
						return err
					}
				}
				return nil
			})
		}
	}
	with := metered(t, e.Admin(), updateAll(contacts.NewProvider(e.DB()).WithFieldCatalog(catalog)))
	without := metered(t, e.Admin(), updateAll(contacts.NewProvider(e.DB())))
	t.Logf("ten writes: %d statements, %d transactions with the catalog; %d statements without",
		with.Statements, with.Transactions, without.Statements)
	if with.Transactions != 1 {
		t.Errorf("ten writes opened %d transactions, want the caller's one", with.Transactions)
	}
	if got := with.Statements - without.Statements; got != 1 {
		t.Errorf("ten writes in one transaction spent %d statements on the catalog, want 1", got)
	}
}

// errRollBack ends a test transaction without keeping what it wrote.
var errRollBack = errors.New("roll the test transaction back")

func TestAFieldRetiredInTheTransactionRefusesAWriteAfterAReadThere(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.Rep1, nil, CustomFieldAdminPerms)
	catalog := customfields.NewService(e.Pool, SchemaPool(t))
	field, err := catalog.Create(admin, customfields.FieldSpec{
		Object: "contact", Label: "Region contact", Type: customfields.TypeText, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := contacts.NewStore(e.DB()).WithFieldCatalog(catalog)
	created, err := store.CreateContact(admin, contacts.CreateContactInput{FullName: "Retired Field"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))

	var written error
	err = database.WithWorkspaceWriteTx(admin, e.Pool, func(ctx context.Context) error {
		if _, err := store.GetContact(ctx, id, storekit.LiveOnly); err != nil {
			return err
		}
		if _, err := catalog.Retire(ctx, ids.UUID(field.Id)); err != nil {
			return err
		}
		_, written = store.UpdateContact(ctx, id, contacts.UpdateContactInput{
			CustomFields: map[string]any{*field.ColumnName: "north"},
		})
		return errRollBack
	})
	if !errors.Is(err, errRollBack) {
		t.Fatalf("the transaction ended with %v before the write", err)
	}
	if written == nil {
		t.Error("a write naming a field retired earlier in the same transaction was accepted: the catalog read before the retirement was reused")
	}
}
