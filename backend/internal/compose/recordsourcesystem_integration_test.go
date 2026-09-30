// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A contact, company, deal or project created through its OWN store carries the
// system it came from, all the way to the column.
//
// Two assertions:
//
//   - the value survives mapper → input → spec → INSERT on all four types, and
//     the company one is checked separately because its spec is copied twice
//     (createCompanyInTx) and only an end-to-end read catches a dropped copy;
//   - author.via strips the importer prefix, so no reader ever sees `mirror:`.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storeBuiltImports creates one of each record type through its own store,
// carrying an ordinary (non-reserved) source system.
type storeBuiltImports struct {
	contact, company, deal, project ids.UUID
}

func seedStoreBuiltImports(t *testing.T, e *integration.Env, sourceSystem *string) storeBuiltImports {
	t.Helper()
	ctx := e.Admin()
	db := InstallationDB(e.Pool)
	contactStore, dealStore, projectStore := contacts.NewStore(db), deals.NewStore(db, DealsInstallation()), ProjectsStoreOver(db)

	company, err := contactStore.CreateCompany(ctx, contacts.CreateCompanyInput{
		DisplayName: "Imported Company", Source: "hubspot_import", SourceSystem: sourceSystem,
	})
	if err != nil {
		t.Fatalf("creating the company through its store: %v", err)
	}
	contact, err := contactStore.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Imported Contact", Source: "hubspot_import", SourceSystem: sourceSystem,
	})
	if err != nil {
		t.Fatalf("creating the contact through its store: %v", err)
	}
	project, err := projectStore.CreateProject(ctx, projects.CreateProjectInput{
		Name: "Imported Project", CompanyID: ids.From[ids.CompanyKind](ids.UUID(company.Id)),
		Source: "hubspot_import", SourceSystem: sourceSystem,
	})
	if err != nil {
		t.Fatalf("creating the project through its store: %v", err)
	}

	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`, pipeline)
	e.WsExec(t, `
		INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, stage, pipeline)
	deal, err := dealStore.CreateDeal(ctx, deals.CreateDealInput{
		Name:       "Imported Deal",
		PipelineID: ids.From[ids.PipelineKind](pipeline),
		StageID:    ids.From[ids.StageKind](stage),
		Source:     "hubspot_import", SourceSystem: sourceSystem,
	})
	if err != nil {
		t.Fatalf("creating the deal through its store: %v", err)
	}

	return storeBuiltImports{
		contact: ids.UUID(contact.Id), company: ids.UUID(company.Id),
		deal: ids.UUID(deal.Id), project: ids.UUID(project.Id),
	}
}

func TestTheRecordStoresWriteTheSourceSystemTheyWereGiven(t *testing.T) {
	e := integration.Setup(t)
	ordinary := "legacy_crm"
	built := seedStoreBuiltImports(t, e, &ordinary)

	for table, id := range map[string]ids.UUID{
		"contact": built.contact, "company": built.company,
		"deal": built.deal, "project": built.project,
	} {
		// Read per table rather than in one union: a dropped spec copy shows
		// up as one NULL, and the failure has to name which table it was.
		if got := e.WsScalar(t, `SELECT coalesce(source_system, '<null>') FROM `+table+` WHERE id = $1`, id); got != ordinary {
			t.Errorf("%s.source_system = %q, want %q — the value did not survive the write chain", table, got, ordinary)
		}
	}
}

// The prefix is machinery for the replay key. A reader must never meet it, and
// the read is the last place it could leak.
func TestTheImporterPrefixNeverReachesAReader(t *testing.T) {
	e := integration.Setup(t)
	stored := "mirror:hubspot"
	built := seedStoreBuiltImportsRaw(t, e, stored)

	contact, err := contacts.NewStore(InstallationDB(e.Pool)).
		GetContact(e.Admin(), ids.From[ids.ContactKind](built), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the imported contact back: %v", err)
	}
	if contact.Author == nil {
		t.Fatal("a contact carrying an author must answer one")
	}
	if contact.Author.Via == nil || *contact.Author.Via != "hubspot" {
		t.Errorf("author.via = %v, want hubspot — the timeline would read \"Logged in mirror:hubspot by …\"", contact.Author.Via)
	}
}

// seedStoreBuiltImportsRaw plants a contact already holding the reserved
// namespace and an author. Raw, because the reserved value is exactly what no
// caller may send: the store is the wrong door for a row only an import writes.
func seedStoreBuiltImportsRaw(t *testing.T, e *integration.Env, sourceSystem string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, full_name, source, captured_by, source_system, source_author_name)
			VALUES ($1, 'Imported Contact', 'hubspot_import', 'human:'||$2, $3, 'Mutaz Suleiman')`,
			id, e.AdminUser, sourceSystem)
		return err
	}); err != nil {
		t.Fatalf("seeding a contact inside the importer namespace: %v", err)
	}
	return id
}
