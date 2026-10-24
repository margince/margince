// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// An edit that makes a record the same human or business as another one runs
// the create's duplicate lanes and files the pair. An edit that does not files
// nothing.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReplacingAnAddressWithATaggedTwinFilesOnePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@example.com")
	edited := e.plainContact(ctx, t, "Anna Beispiel", "anna@elsewhere.test")
	if pairs := openCandidatePairs(ctx, t, e, "contact", edited); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the edit, want 0", len(pairs))
	}

	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](edited), UpdateContactInput{
		Emails: []ContactEmailInput{{Email: "ab+accounting@example.com", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("replace the addresses: %v", err)
	}

	pairs := openCandidatePairs(ctx, t, e, "contact", edited)
	if len(pairs) != 1 || pairs[0].OtherID != incumbent.String() {
		t.Fatalf("got pairs %+v, want exactly the pair with %s", pairs, incumbent)
	}
	var source string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT source FROM dedupe_candidate WHERE $1 IN (left_contact_id, right_contact_id)`,
			edited).Scan(&source)
	}); err != nil {
		t.Fatalf("read the pair's source: %v", err)
	}
	if source != contactRenameRecheckSource {
		t.Errorf("source %q, want %q", source, contactRenameRecheckSource)
	}
}

func TestReplacingAnAddressWithAnUnrelatedOneFilesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Ab", "ab@example.com")
	edited := e.plainContact(ctx, t, "Anna Beispiel", "anna@elsewhere.test")

	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](edited), UpdateContactInput{
		Emails: []ContactEmailInput{{Email: "anna.beispiel@elsewhere.test", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("replace the addresses: %v", err)
	}
	if pairs := openCandidatePairs(ctx, t, e, "contact", edited); len(pairs) != 0 {
		t.Fatalf("got pairs %+v, want none", pairs)
	}
}

func TestAddingASubdomainOfAHeldDomainToACompanyFilesOnePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holder := e.createNamedCompany(ctx, t, "Example Group", "example.com")
	edited := e.createNamedCompany(ctx, t, "Cloudy", "cloudy.test")
	if field := e.filedPairField(ctx, t, edited, holder); field != "" {
		t.Fatalf("a pair with field %q before the edit, want none", field)
	}

	desired := []CompanyDomainInput{{Domain: "cloudy.test", IsPrimary: true}, {Domain: "cloud.example.com"}}
	if _, err := e.store.UpdateCompany(ctx, edited, UpdateCompanyInput{Domains: &desired}); err != nil {
		t.Fatalf("add the domain: %v", err)
	}
	if field := e.filedPairField(ctx, t, edited, holder); field != laneDomain {
		t.Fatalf("filed field %q, want %q", field, laneDomain)
	}
	if n := e.countInWorkspace(ctx, t, `
		SELECT count(*) FROM dedupe_candidate
		 WHERE entity_type = 'company' AND disposition = 'open'`); n != 1 {
		t.Fatalf("%d open company pairs, want exactly 1", n)
	}
}

func TestAddingAnUnrelatedDomainToACompanyFilesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.createNamedCompany(ctx, t, "Example Group", "example.com")
	edited := e.createNamedCompany(ctx, t, "Cloudy", "cloudy.test")

	desired := []CompanyDomainInput{{Domain: "cloudy.test", IsPrimary: true}, {Domain: "cloud.other.test"}}
	if _, err := e.store.UpdateCompany(ctx, edited, UpdateCompanyInput{Domains: &desired}); err != nil {
		t.Fatalf("add the domain: %v", err)
	}
	if n := e.countInWorkspace(ctx, t, `
		SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'company'`); n != 0 {
		t.Fatalf("%d company pairs, want none", n)
	}
}
