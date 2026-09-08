// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// An organization with live domains names one of them primary, whatever the
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
// test asserts the sweep then finds such an organization.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// livePrimaryOf reads the primary domain the record actually carries, or "" for
// the state this whole file exists to prevent. It reads through liveDomainsOf
// rather than its own query, because a second SELECT over organization_domain
// would be a second definition of "live" to keep in step with the archival
// column.
func livePrimaryOf(ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID) string {
	t.Helper()
	primary := ""
	for domain, isPrimary := range liveDomainsOf(ctx, t, e, orgID) {
		if !isPrimary {
			continue
		}
		if primary != "" {
			t.Fatalf("two live primaries on one organization: %q and %q", primary, domain)
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

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Terralogic", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "terralogic.test"}},
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	if got := livePrimaryOf(ctx, t, e, orgID); got != "terralogic.test" {
		t.Fatalf("primary domain is %q, want terralogic.test — the auto-enrich sweep joins on it, so this company would never be read", got)
	}
}

// Several domains and no primary is the same defect with more rows: the sweep
// needs exactly one, and the caller named none.
func TestACompanyCreatedWithSeveralDomainsAndNoPrimaryNamesTheFirst(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Northwind Handel", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "northwind-a.test"}, {Domain: "northwind-b.test"}},
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	if got := livePrimaryOf(ctx, t, e, orgID); got != "northwind-a.test" {
		t.Fatalf("primary domain is %q, want the first domain northwind-a.test", got)
	}
}

// The election fills SILENCE and never overrules a choice. Without this the
// two tests above would also pass against a writer that ignored the caller.
func TestACallerWhoNamesThePrimaryKeepsIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Kestrel Labs", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "kestrel-a.test"}, {Domain: "kestrel-b.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	if got := livePrimaryOf(ctx, t, e, orgID); got != "kestrel-b.test" {
		t.Fatalf("primary domain is %q, want the caller's kestrel-b.test", got)
	}
}

// The edit path has the same hole and one extra rule: an edit that merely adds
// a domain must not move the primary the record already had.
func TestAnEditThatNamesNoPrimaryLeavesTheLiveOneAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Halden Werke", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "halden-a.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	desired := []OrgDomainInput{{Domain: "halden-a.test"}, {Domain: "halden-b.test"}}
	if _, err := e.store.UpdateOrganization(ctx, orgID, UpdateOrganizationInput{Domains: &desired}); err != nil {
		t.Fatalf("updating the organization: %v", err)
	}

	if got := livePrimaryOf(ctx, t, e, orgID); got != "halden-a.test" {
		t.Fatalf("primary domain is %q, want the live halden-a.test kept", got)
	}
}

// And an edit that REMOVES the live primary still leaves one, rather than a
// record the sweep cannot see.
func TestAnEditThatDropsTheLivePrimaryElectsAnother(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Ostsee Fracht", Source: "manual",
		Domains: []OrgDomainInput{{Domain: "ostsee-old.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	desired := []OrgDomainInput{{Domain: "ostsee-new.test"}}
	if _, err := e.store.UpdateOrganization(ctx, orgID, UpdateOrganizationInput{Domains: &desired}); err != nil {
		t.Fatalf("updating the organization: %v", err)
	}

	if got := livePrimaryOf(ctx, t, e, orgID); got != "ostsee-new.test" {
		t.Fatalf("primary domain is %q, want ostsee-new.test", got)
	}
}
