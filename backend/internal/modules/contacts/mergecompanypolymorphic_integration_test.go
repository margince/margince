// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a merge does with rows that name a record by kind and id. Those carry
// no foreign key. The same file covers the LinkedIn address a survivor lacks.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	retiredLinkedIn   = "https://www.linkedin.com/company/acme-glazing"
	survivorsLinkedIn = "https://www.linkedin.com/company/survivor"
)

func (e *dedupeEnv) companyPair(ctx context.Context, t *testing.T) (source, survivor ids.CompanyID) {
	t.Helper()
	mk := func(name string) ids.CompanyID {
		c, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.CompanyKind](ids.UUID(c.Id))
	}
	return mk("Retired Glazing GmbH"), mk("Surviving Glazing GmbH")
}

func (e *dedupeEnv) setLinkedIn(ctx context.Context, t *testing.T, id ids.CompanyID, url string) {
	t.Helper()
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{LinkedInURL: &url}); err != nil {
		t.Fatal(err)
	}
}

// A signal and a file about the retired company are about the survivor now.
func TestMergingACompanyMovesTheRowsThatNameItByKindAndID(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	source, survivor := e.companyPair(ctx, t)
	e.execAs(ctx, t, `INSERT INTO signal (id, kind, source, source_channel, entity_type, entity_id, summary, severity, captured_by)
		VALUES ($1, 'risk', 'qc', 'inbound', 'company', $2, 'flagged', 'warn', 'human:x')`, ids.NewV7(), source)

	e.execAs(ctx, t, `INSERT INTO attachment (id, entity_type, entity_id, company_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'company', $2, $2, 'msa.pdf', 'k/msa', 'upload', 'human:x')`, ids.NewV7(), source)

	if _, err := e.store.MergeCompany(ctx, source, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	for _, table := range []string{"signal", "attachment"} {
		named := `SELECT count(*) FROM ` + table + ` WHERE entity_type = 'company' AND entity_id = $1`
		if n := e.countAs(ctx, t, named, source); n != 0 {
			t.Errorf("%d %s rows still name the retired company", n, table)
		}
		if n := e.countAs(ctx, t, named, survivor); n != 1 {
			t.Errorf("the survivor holds %d %s rows, want 1", n, table)
		}
	}
}

// A signal about the retired contact is about the survivor now.
func TestMergingAContactMovesTheSignalsNamingIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeper@kindid.test", "Keeper", "kindid.test"))
	if err != nil {
		t.Fatal(err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dupe@kindid.test", "Dupe", "kindid.test"))
	if err != nil {
		t.Fatal(err)
	}
	e.execAs(ctx, t, `INSERT INTO signal (id, kind, source, source_channel, entity_type, entity_id, summary, severity, captured_by)
		VALUES ($1, 'risk', 'qc', 'inbound', 'contact', $2, 'flagged', 'warn', 'human:x')`, ids.NewV7(), retired.ContactID)

	if _, err := e.store.MergeContact(ctx, retired.ContactID, survivor.ContactID, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	const named = `SELECT count(*) FROM signal WHERE entity_type = 'contact' AND entity_id = $1`
	if n := e.countAs(ctx, t, named, survivor.ContactID); n != 1 {
		t.Errorf("the survivor holds %d of the retired contact's signals, want 1", n)
	}
}

// The address on the retired company moves when the survivor has none.
func TestMergingACompanyCarriesItsLinkedInAddressWhenTheSurvivorHasNone(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	source, survivor := e.companyPair(ctx, t)
	e.setLinkedIn(ctx, t, source, retiredLinkedIn)

	if _, err := e.store.MergeCompany(ctx, source, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	got, err := e.store.GetCompany(ctx, survivor, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.LinkedinUrl == nil || *got.LinkedinUrl != retiredLinkedIn {
		t.Errorf("survivor LinkedIn = %v, want %s", got.LinkedinUrl, retiredLinkedIn)
	}
}

// A survivor that has its own address keeps it.
func TestMergingACompanyKeepsTheSurvivorsOwnLinkedInAddress(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	source, survivor := e.companyPair(ctx, t)
	e.setLinkedIn(ctx, t, survivor, survivorsLinkedIn)
	e.setLinkedIn(ctx, t, source, retiredLinkedIn)

	if _, err := e.store.MergeCompany(ctx, source, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	got, err := e.store.GetCompany(ctx, survivor, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.LinkedinUrl == nil || *got.LinkedinUrl != survivorsLinkedIn {
		t.Errorf("survivor LinkedIn = %v, want its own %s", got.LinkedinUrl, survivorsLinkedIn)
	}
	// The retired record keeps its own address: the merge frees nothing by
	// erasing it.
	retired, err := e.store.GetCompany(ctx, source, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	if retired.LinkedinUrl == nil || *retired.LinkedinUrl != retiredLinkedIn {
		t.Errorf("retired LinkedIn = %v, want %s kept", retired.LinkedinUrl, retiredLinkedIn)
	}
}

// An address another live company holds is a conflict, never a 500.
func TestAnAddressAnotherCompanyHoldsIsRefusedAsAConflict(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	a, b := e.companyPair(ctx, t)
	e.setLinkedIn(ctx, t, a, retiredLinkedIn)

	url := retiredLinkedIn
	_, err := e.store.UpdateCompany(ctx, b, UpdateCompanyInput{LinkedInURL: &url})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a taken address answered %v, want %v", err, apperrors.ErrConflict)
	}
}
