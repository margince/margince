// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Editing a company's domains is a replace-set that rides the company's own write
// shape: one version bump, one audit row, one company.updated event,
// every added domain human-stamped. A domain owned by another company is the
// typed 409; keeping a domain the company already owns is never a false
// conflict.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func liveDomainsOf(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT domain, is_primary FROM company_domain
			  WHERE company_id = $1 AND archived_at IS NULL`, companyID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d string
			var p bool
			if err := rows.Scan(&d, &p); err != nil {
				return err
			}
			out[d] = p
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("read live domains: %v", err)
	}
	return out
}

func companyVersion(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID) int64 {
	t.Helper()
	var v int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT version FROM company WHERE id = $1`, companyID).Scan(&v)
	}); err != nil {
		t.Fatalf("read company version: %v", err)
	}
	return v
}

func countCompanyUpdatedEvents(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM event_outbox
			  WHERE envelope->>'type' = 'company.updated'
			    AND envelope->'entity'->>'id' = $1`, companyID.String()).Scan(&n)
	}); err != nil {
		t.Fatalf("count company.updated events: %v", err)
	}
	return n
}

func domainCapturedBy(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID, domain string) string {
	t.Helper()
	var by string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT captured_by FROM company_domain
			  WHERE company_id = $1 AND domain = $2 AND archived_at IS NULL`, companyID, domain).Scan(&by)
	}); err != nil {
		t.Fatalf("read domain captured_by: %v", err)
	}
	return by
}

func TestUpdateCompanyDomainsReplaceSet(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Voltaq Systems GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "voltaq.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	v0 := companyVersion(ctx, t, e, companyID)

	// Add a second domain and move the primary onto it; the first stays but
	// is demoted.
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{
			{Domain: "voltaq.test", IsPrimary: false},
			{Domain: "voltaq-systems.test", IsPrimary: true},
		},
	}); err != nil {
		t.Fatalf("replace-set update: %v", err)
	}

	live := liveDomainsOf(ctx, t, e, companyID)
	if len(live) != 2 || live["voltaq.test"] != false || live["voltaq-systems.test"] != true {
		t.Fatalf("live domains after replace-set = %+v, want {voltaq.test:false, voltaq-systems.test:true}", live)
	}
	if by := domainCapturedBy(ctx, t, e, companyID, "voltaq-systems.test"); len(by) < 6 || by[:6] != "human:" {
		t.Fatalf("added domain captured_by = %q, want human:*", by)
	}
	if v1 := companyVersion(ctx, t, e, companyID); v1 <= v0 {
		t.Fatalf("company version did not bump: %d -> %d", v0, v1)
	}
	if n := countCompanyUpdatedEvents(ctx, t, e, companyID); n != 1 {
		t.Fatalf("got %d company.updated events, want exactly 1", n)
	}

	// Remove the demoted domain: the replace-set archives it.
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{{Domain: "voltaq-systems.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("remove update: %v", err)
	}
	live = liveDomainsOf(ctx, t, e, companyID)
	if len(live) != 1 || live["voltaq-systems.test"] != true {
		t.Fatalf("live domains after removal = %+v, want {voltaq-systems.test:true}", live)
	}
}

func TestUpdateCompanyDomainConflictIsTyped409(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	owner, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Owner GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "claimed.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Other GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "other.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.store.UpdateCompany(ctx, ids.From[ids.CompanyKind](ids.UUID(other.Id)),
		UpdateCompanyInput{Domains: &[]CompanyDomainInput{{Domain: "claimed.test", IsPrimary: true}}})
	var dup *DuplicateDomainError
	if !errors.As(err, &dup) {
		t.Fatalf("claiming another company's domain must be the typed 409, got %v", err)
	}
	if dup.ExistingID != ids.From[ids.CompanyKind](ids.UUID(owner.Id)) {
		t.Fatalf("409 discloses %s, want the owner %s", dup.ExistingID, owner.Id)
	}
}

func TestUpdateCompanyClearsAllDomains(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Clear GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "clear.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	// An explicit empty replace-set clears every domain (not "untouched").
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{},
	}); err != nil {
		t.Fatalf("clear-all update: %v", err)
	}
	if live := liveDomainsOf(ctx, t, e, companyID); len(live) != 0 {
		t.Fatalf("clear-all left %+v, want none", live)
	}
}

func TestUpdateCompanyDeduplicatesDomainInput(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Dup GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "dup.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	// The same host named twice (here also via its www form, which normalizes
	// to the same domain) must reconcile to one row — not self-collide on
	// uq_company_domain with a misleading ownership 409.
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{
			{Domain: "dup.test", IsPrimary: true},
			{Domain: "www.dup.test", IsPrimary: false},
		},
	}); err != nil {
		t.Fatalf("self-duplicate replace-set must dedupe, not conflict: %v", err)
	}
	live := liveDomainsOf(ctx, t, e, companyID)
	if len(live) != 1 || !live["dup.test"] {
		t.Fatalf("dedupe left %+v, want {dup.test:true}", live)
	}
}

func TestUpdateCompanyKeepingOwnDomainIsNoFalseConflict(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Keep GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "keep.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	// Re-submitting the company's own live domain must not read as a dedupe hit.
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		DisplayName: strPtr("Keep GmbH (edited)"),
		Domains:     &[]CompanyDomainInput{{Domain: "keep.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("keeping own domain must not conflict: %v", err)
	}
	live := liveDomainsOf(ctx, t, e, companyID)
	if len(live) != 1 || !live["keep.test"] {
		t.Fatalf("live domains = %+v, want {keep.test:true}", live)
	}
}

// Both replace-sets in ONE request — the pair nothing covered, and the shape that
// used to 500: each branch bumped updated_at, and two assignments to one column
// is a statement Postgres rejects.
//
// What this asserts is the endpoint's property, not the patch's: a request naming
// both sets succeeds, BOTH sets land, and it is a single write. The rendering rule
// that makes it possible belongs to storekit.Patch and is pinned there
// (patch_test.go) — which is why this test stays green even if the de-duplication
// is removed, now that this caller bumps updated_at once.
func TestUpdateCompanyAcceptsDomainsAndRelationshipTypesTogether(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Kerrix Industrial", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "kerrix.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	v0 := companyVersion(ctx, t, e, companyID)

	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{
		Domains:           &[]CompanyDomainInput{{Domain: "kerrix-industrial.test", IsPrimary: true}},
		RelationshipTypes: &[]string{"customer"},
	}); err != nil {
		t.Fatalf("domains and relationship_types in one patch: %v", err)
	}

	// Both replace-sets actually landed — the request must not merely stop
	// erroring while silently applying one half.
	live := liveDomainsOf(ctx, t, e, companyID)
	if len(live) != 1 || !live["kerrix-industrial.test"] {
		t.Errorf("live domains = %+v, want only kerrix-industrial.test as primary", live)
	}
	if types := liveTypesOf(ctx, t, e, companyID); len(types) != 1 || !types["customer"] {
		t.Errorf("live relationship types = %+v, want only customer", types)
	}
	// One request is one write: the version bumps once, not once per replace-set.
	if v1 := companyVersion(ctx, t, e, companyID); v1 != v0+1 {
		t.Errorf("version %d -> %d, want a single bump for a single request", v0, v1)
	}
	if n := countCompanyUpdatedEvents(ctx, t, e, companyID); n != 1 {
		t.Errorf("got %d company.updated events, want exactly 1", n)
	}
}
