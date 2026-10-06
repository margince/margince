// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a merge makes of a partner row that is no longer live.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// retireProgramme archives a company's partner row and its relationship types
// together, which is the pair companyarchive keeps in lockstep.
//
// PLANTED, and worth saying why. No writer in the product reaches this state on
// a LIVE company today: the only statement that archives a partner row archives
// the whole company with it, and a merge refuses an archived source. What the
// merge's verdict must not do is read a row's EXISTENCE as a live programme,
// and this is the input that asks it.
func (e *dedupeEnv) retireProgramme(ctx context.Context, t *testing.T, company ids.CompanyID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE partner SET archived_at = now() WHERE company_id = $1`, company); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE company_relationship_type SET archived_at = now() WHERE company_id = $1`, company)
		return err
	}); err != nil {
		t.Fatalf("retiring the partner programme: %v", err)
	}
}

// A RETIRED PROGRAMME DOES NOT MAKE THE SURVIVOR A PARTNER.
//
// The merge decides what the survivor IS from the partner extension, and read
// the row's existence rather than its liveness. A survivor typed `partner`
// whose own partner endpoint answers 404 is the half-state the relationship-type
// invariant exists to make impossible: the table stays consistent and the
// product stops being.
func TestMergingIntoARetiredPartnerLeavesTheSurvivorUntyped(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)

	survivor, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Survivor Glazing GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	survivorID := ids.From[ids.CompanyKind](ids.UUID(survivor.Id))
	if _, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{
		CompanyID: survivorID, PartnerRole: "consulting",
	}); err != nil {
		t.Fatalf("opening the survivor's partner programme: %v", err)
	}
	e.retireProgramme(ctx, t, survivorID)

	source, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Plain Source GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.MergeCompany(ctx,
		ids.From[ids.CompanyKind](ids.UUID(source.Id)), survivorID); err != nil {
		t.Fatalf("merging into the retired partner: %v", err)
	}

	if liveTypesOf(ctx, t, e, survivorID)[relationshipTypePartner] {
		t.Errorf("the survivor is typed %q on the strength of a RETIRED partner row", relationshipTypePartner)
	}
	// The other half of the same claim, asked of the endpoint a reader would
	// click through to: a type they can see and a record they cannot open.
	if _, err := e.store.GetPartner(ctx, survivorID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the survivor's partner programme answered %v, want %v", err, apperrors.ErrNotFound)
	}
}

// THE POSITIVE CONTROL: a LIVE programme still carries over.
//
// The verdict moved out of the move's own statement, so the case the move
// exists for has to be asserted beside the case it was getting wrong. A merge
// that carried the row and then said the survivor is not a partner would leave
// the extension row with nothing naming it — the opposite half of the same
// invariant, and exactly what ADR-0079 amended into place.
func TestMergingALivePartnerMakesTheSurvivorOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)

	survivor, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Plain Survivor GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Live Partner GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.From[ids.CompanyKind](ids.UUID(source.Id))
	if _, err := e.store.UpsertPartner(ctx, UpsertPartnerInput{
		CompanyID: sourceID, PartnerRole: "consulting",
	}); err != nil {
		t.Fatalf("opening the source's partner programme: %v", err)
	}

	survivorID := ids.From[ids.CompanyKind](ids.UUID(survivor.Id))
	if _, err := e.store.MergeCompany(ctx, sourceID, survivorID); err != nil {
		t.Fatalf("merging the live partner away: %v", err)
	}

	if !liveTypesOf(ctx, t, e, survivorID)[relationshipTypePartner] {
		t.Errorf("the survivor inherited a live partner programme and is not typed %q, "+
			"so the extension row it now holds says nothing about it", relationshipTypePartner)
	}
	if _, err := e.store.GetPartner(ctx, survivorID); err != nil {
		t.Errorf("the survivor's inherited partner programme does not read back: %v", err)
	}
}
