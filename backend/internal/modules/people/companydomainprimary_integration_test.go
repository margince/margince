// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A company with live domains names one of them primary, whatever the
// caller sent.
//
// The column is not decoration. capture's auto-enrich sweep INNER JOINs on it,
// so a company whose only domain carries false is never a candidate for the
// website read — silently, with no job and no error. The contract admits that
// state: is_primary defaults to false and only domain is required, so an agent
// constructing the minimal valid body reaches it through the ordinary door.
//
// The sweep's own query lives in capture and cannot be called from here, so
// these tests assert the column this module writes; the compose integration
// test asserts the sweep then finds such a company.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// livePrimaryOf reads the primary domain the record actually carries, or "" for
// the state this whole file exists to prevent. It reads through liveDomainsOf
// rather than its own query, because a second SELECT over company_domain
// would be a second definition of "live" to keep in step with the archival
// column.
func livePrimaryOf(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID) string {
	t.Helper()
	primary := ""
	for domain, isPrimary := range liveDomainsOf(ctx, t, e, companyID) {
		if !isPrimary {
			continue
		}
		if primary != "" {
			t.Fatalf("two live primaries on one company: %q and %q", primary, domain)
		}
		primary = domain
	}
	return primary
}

// The reported defect, at the door it came through: one domain, no is_primary,
// which is the whole of a valid minimal request body.
func TestACompanyCreatedWithOneDomainAndNoPrimaryStillNamesIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Terralogic", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "terralogic.test"}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	if got := livePrimaryOf(ctx, t, e, companyID); got != "terralogic.test" {
		t.Fatalf("primary domain is %q, want terralogic.test — the auto-enrich sweep joins on it, so this company would never be read", got)
	}
}

// Several domains and no primary is the same defect with more rows: the sweep
// needs exactly one, and the caller named none.
func TestACompanyCreatedWithSeveralDomainsAndNoPrimaryNamesTheFirst(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Northwind Handel", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "northwind-a.test"}, {Domain: "northwind-b.test"}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	if got := livePrimaryOf(ctx, t, e, companyID); got != "northwind-a.test" {
		t.Fatalf("primary domain is %q, want the first domain northwind-a.test", got)
	}
}

// The election fills SILENCE and never overrules a choice. Without this the
// two tests above would also pass against a writer that ignored the caller.
func TestACallerWhoNamesThePrimaryKeepsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Kestrel Labs", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "kestrel-a.test"}, {Domain: "kestrel-b.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	if got := livePrimaryOf(ctx, t, e, companyID); got != "kestrel-b.test" {
		t.Fatalf("primary domain is %q, want the caller's kestrel-b.test", got)
	}
}

// The edit path has the same hole and one extra rule: an edit that merely adds
// a domain must not move the primary the record already had.
func TestAnEditThatNamesNoPrimaryLeavesTheLiveOneAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Halden Werke", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "halden-a.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	desired := []CompanyDomainInput{{Domain: "halden-a.test"}, {Domain: "halden-b.test"}}
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Domains: &desired}); err != nil {
		t.Fatalf("updating the company: %v", err)
	}

	if got := livePrimaryOf(ctx, t, e, companyID); got != "halden-a.test" {
		t.Fatalf("primary domain is %q, want the live halden-a.test kept", got)
	}
}

// And an edit that REMOVES the live primary still leaves one, rather than a
// record the sweep cannot see.
func TestAnEditThatDropsTheLivePrimaryElectsAnother(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Ostsee Fracht", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "ostsee-old.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	desired := []CompanyDomainInput{{Domain: "ostsee-new.test"}}
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Domains: &desired}); err != nil {
		t.Fatalf("updating the company: %v", err)
	}

	if got := livePrimaryOf(ctx, t, e, companyID); got != "ostsee-new.test" {
		t.Fatalf("primary domain is %q, want ostsee-new.test", got)
	}
}

// The audit trail has to say what the row says.
//
// is_primary is what admits a company to the website read, so the after-image is
// the record of why that read happened. An election applied to the row but not
// to the image the patch audits leaves a trail claiming the company has no
// primary domain — the exact state that would mean no read was due — while the
// row that triggered one says otherwise.
func TestTheAuditImageNamesThePrimaryTheRowActuallyCarries(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Rheinfels Guss", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "rheinfels-old.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))

	// A replace-set naming no primary: the election has to fill it, and the
	// image has to report what it filled.
	desired := []CompanyDomainInput{{Domain: "rheinfels-new.test"}}
	if _, err := e.store.UpdateCompany(ctx, companyID, UpdateCompanyInput{Domains: &desired}); err != nil {
		t.Fatalf("updating the company: %v", err)
	}

	onRow := livePrimaryOf(ctx, t, e, companyID)
	if onRow != "rheinfels-new.test" {
		t.Fatalf("primary domain on the row is %q, want rheinfels-new.test", onRow)
	}

	inTrail := auditedPrimaryDomain(ctx, t, e, companyID)
	if inTrail != onRow {
		t.Fatalf("the audit trail names %q as primary and the row carries %q — an auditor replaying the log would conclude no website read was due", inTrail, onRow)
	}
}

// auditedPrimaryDomain reads the primary domain the newest company update
// recorded in its after-image, or "" when it recorded none.
func auditedPrimaryDomain(ctx context.Context, t *testing.T, e *dedupeEnv, companyID ids.CompanyID) string {
	t.Helper()
	var after map[string]any
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT after FROM audit_log
			 WHERE entity_type = 'company' AND entity_id = $1 AND action = 'update'
			 ORDER BY occurred_at DESC, id DESC
			 LIMIT 1`, companyID).Scan(&after)
	}); err != nil {
		t.Fatalf("reading the company's newest audit image: %v", err)
	}
	domains, ok := after["domains"].([]any)
	if !ok {
		t.Fatalf("the audit image records no domains at all: %v", after)
	}
	primary := ""
	for _, entry := range domains {
		row, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("a domain entry is not an object: %v", entry)
		}
		if isPrimary, _ := row["is_primary"].(bool); !isPrimary {
			continue
		}
		domain, _ := row["domain"].(string)
		if primary != "" {
			t.Fatalf("the audit image names two primaries: %q and %q", primary, domain)
		}
		primary = domain
	}
	return primary
}
