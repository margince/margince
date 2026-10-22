// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func createSourcedCompany(ctx context.Context, e *dedupeEnv, name, domain, system, sourceID string) error {
	_, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: name, Source: "manual", SourceSystem: &system, SourceID: &sourceID,
		Domains: []CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	return err
}

func TestACompanyIsFoundByItsIDInTheOutsideSystem(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if err := createSourcedCompany(ctx, e, "Voltaq Systems GmbH", "voltaq.test", "lookup-erp", "C-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := createSourcedCompany(ctx, e, "Andere AG", "andere.test", "lookup-erp", "C-2"); err != nil {
		t.Fatalf("create: %v", err)
	}
	seedLogoCompany(ctx, t, e, "Typed Here GmbH", "typed.test")

	found, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{SourceSystem: new("lookup-erp"), SourceID: new("C-1")})
	if err != nil || len(found) != 1 || found[0].DisplayName != "Voltaq Systems GmbH" {
		t.Fatalf("lookup by (lookup-erp, C-1) = %d companies, err %v; want exactly Voltaq", len(found), err)
	}
	for _, miss := range [][2]string{{"lookup-erp", "C-9"}, {"other-erp", "C-1"}} {
		page, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{SourceSystem: &miss[0], SourceID: &miss[1]})
		if err != nil || len(page) != 0 {
			t.Fatalf("lookup by %v = %d companies, err %v; want an empty page", miss, len(page), err)
		}
	}
	fromSystem, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{SourceSystem: new("lookup-erp")})
	if err != nil || len(fromSystem) != 2 {
		t.Fatalf("lookup by system alone = %d companies, err %v; want the two it issued ids for", len(fromSystem), err)
	}
}

func TestTheSameOutsideIDCannotNameTwoCompaniesButTwoSystemsMayShareOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if err := createSourcedCompany(ctx, e, "Voltaq Systems GmbH", "voltaq.test", "acme-erp", "C-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := createSourcedCompany(ctx, e, "Rival GmbH", "rival.test", "acme-erp", "C-1")
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second company with (acme-erp, C-1) = %v, want a conflict", err)
	}
	if err := createSourcedCompany(ctx, e, "Rival GmbH", "rival.test", "other-erp", "C-1"); err != nil {
		t.Fatalf("the same id in another system is a different pair: %v", err)
	}
}

func TestACompanyTheCallerCannotSeeIsNotFoundByItsOutsideID(t *testing.T) {
	e := setupDedupe(t)
	owner := e.as()
	if err := createSourcedCompany(owner, e, "Fremdfirma GmbH", "fremd.test", "acme-erp", "C-7"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(owner, `UPDATE company SET owner_id = $1, visibility = 'owner' WHERE source_id = 'C-7'`, e.rep)
		return err
	}); err != nil {
		t.Fatalf("make the company capture-private: %v", err)
	}

	found, _, err := e.store.ListCompanies(e.asOwnScoped(ids.NewV7()), ListCompaniesInput{
		SourceSystem: new("acme-erp"), SourceID: new("C-7"),
	})
	if err != nil || len(found) != 0 {
		t.Fatalf("a stranger's lookup = %d companies, err %v; want none", len(found), err)
	}
}

// The clash answer must not depend on whether the caller may read the holder,
// and the refused create must leave nothing behind.
func TestACreateOverAHiddenHoldersOutsideIDAnswersTheSameConflict(t *testing.T) {
	e := setupDedupe(t)
	owner := e.as()
	if err := createSourcedCompany(owner, e, "Fremdfirma GmbH", "fremd2.test", "hidden-erp", "C-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(owner, `UPDATE company SET owner_id = $1, visibility = 'owner' WHERE source_system = 'hidden-erp'`, e.rep)
		return err
	}); err != nil {
		t.Fatalf("make the company capture-private: %v", err)
	}
	strangerID := ids.NewV7()
	stranger := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + strangerID.String(), UserID: strangerID,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"company": {Create: true, Read: true}},
			RowScope: principal.RowScopeOwn,
		},
	})
	stranger = principal.WithCorrelationID(stranger, ids.NewV7())
	create := func(ctx context.Context) error {
		_, err := e.store.CreateCompany(ctx, CreateCompanyInput{
			DisplayName: "Squatter GmbH", Source: "manual", SourceSystem: new("hidden-erp"), SourceID: new("C-1"),
		})
		return err
	}
	fromStranger, fromOwner := create(stranger), create(owner)
	if !errors.Is(fromStranger, apperrors.ErrConflict) || !errors.Is(fromOwner, apperrors.ErrConflict) {
		t.Fatalf("clash answers: stranger %v, owner %v; want a conflict for both", fromStranger, fromOwner)
	}
	if fromStranger.Error() != fromOwner.Error() {
		t.Fatalf("the clash answer differs by visibility: %q and %q", fromStranger, fromOwner)
	}
	if n := e.countInWorkspace(owner, t, `SELECT count(*) FROM company WHERE display_name = 'Squatter GmbH'`); n != 0 {
		t.Fatalf("the refused creates left %d companies behind", n)
	}
}
