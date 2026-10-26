// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type failingCatalog struct{}

func (failingCatalog) ActiveColumns(context.Context, string) ([]fieldcatalog.Column, error) {
	return nil, errors.New("catalog down")
}

// What a company's offered values, outside-id adoption and read-through-a-
// transaction refuse, and the events a company's erasure stages.
func TestCompanyOfferedValuesAdoptionAndListRefuseWhatTheyMust(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.as(e.owner, principal.RowScopeAll)

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Brandt ERP", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	missing := ids.New[ids.CompanyKind]()

	// Offers on a company that does not exist are refused, read and write alike.
	if err := e.store.RecordOfferedValues(ctx, missing, "erp", map[string]string{"industry": "x"}); err == nil {
		t.Error("RecordOfferedValues on a missing company succeeded")
	}
	if _, err := e.store.ListOfferedValues(ctx, missing); err == nil {
		t.Error("ListOfferedValues on a missing company succeeded")
	}
	// An archived company serves no offers.
	if err := e.store.RecordOfferedValues(ctx, id, "erp", map[string]string{"industry": "x"}); err != nil {
		t.Fatal(err)
	}
	p, _ := principal.Actor(ctx)
	p.Permissions.Objects["company"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	if _, err := e.store.ArchiveCompany(principal.WithActor(ctx, p), id, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ListOfferedValues(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("ListOfferedValues on an archived company = %v, want ErrNotFound", err)
	}

	// Adoption: a bad pair is refused, a first adoption lands, a second is a conflict,
	// and a pair another company holds is a conflict too.
	a, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Adopt A", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Adopt B", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	aID, bID := ids.From[ids.CompanyKind](ids.UUID(a.Id)), ids.From[ids.CompanyKind](ids.UUID(b.Id))
	if _, err := e.store.UpdateCompany(ctx, aID, UpdateCompanyInput{AdoptSource: &OutsideID{System: "erp", ID: ""}}); err == nil {
		t.Error("adopting an empty source id succeeded")
	}
	if _, err := e.store.UpdateCompany(ctx, aID, UpdateCompanyInput{AdoptSource: &OutsideID{System: "erp", ID: "1"}}); err != nil {
		t.Fatalf("first adoption: %v", err)
	}
	if _, err := e.store.UpdateCompany(ctx, aID, UpdateCompanyInput{AdoptSource: &OutsideID{System: "erp", ID: "2"}}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("second adoption = %v, want ErrConflict", err)
	}
	if _, err := e.store.UpdateCompany(ctx, bID, UpdateCompanyInput{AdoptSource: &OutsideID{System: "erp", ID: "1"}}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("adopting a pair another company holds = %v, want ErrConflict", err)
	}
	// A company that vanished mid-adoption cannot be read for its outside id.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return stageAdoptedSource(ctx, tx, missing, nil, &OutsideID{System: "erp", ID: "9"})
	}); err == nil {
		t.Error("adopting onto a missing company succeeded")
	}

	// The erasure event stages in the erasing transaction.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		auditID, err := storekit.AuditEvent(ctx, tx, "erase", "company", aID.UUID, nil)
		if err != nil {
			return err
		}
		return EmitCompanyErased(ctx, tx, auditID, aID)
	}); err != nil {
		t.Errorf("EmitCompanyErased: %v", err)
	}

	// The catalog is read only for a caller who may read companies.
	if _, err := CompanyColumnsFrom(ctx, failingCatalog{}); err == nil {
		t.Error("CompanyColumnsFrom swallowed a catalog failure")
	}
	noRead := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: e.owner})
	if _, err := CompanyColumnsFrom(noRead, failingCatalog{}); err == nil {
		t.Error("CompanyColumnsFrom admitted a caller without company:read")
	}
	if _, _, err := e.store.ListCompaniesTx(noRead, nil, ListCompaniesInput{}, CustomColumns{}); err == nil {
		t.Error("ListCompaniesTx admitted a caller without company:read")
	}

	// A list that finds nothing answers an empty slice, and a half outside-id filter is refused.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		sys := "no-such-system"
		recs, _, err := e.store.ListCompaniesTx(ctx, tx, ListCompaniesInput{SourceSystem: &sys, SourceID: &sys}, CustomColumns{})
		if err != nil || recs == nil || len(recs) != 0 {
			t.Errorf("empty ListCompaniesTx = %v, %v; want an empty non-nil slice", recs, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{SourceID: &empty}); err == nil {
		t.Error("an empty source_id filter was admitted")
	}
}
