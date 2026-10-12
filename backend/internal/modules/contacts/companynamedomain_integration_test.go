// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A company whose display name is a web domain is paired for review with the
// company that claims that domain, whichever of the two arrives second. The
// names in each case share no word, so the fuzzy tier cannot be what pairs
// them.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createNamedCompany creates a company through the manual create path,
// claiming the given domains with the first as primary.
func (e *dedupeEnv) createNamedCompany(ctx context.Context, t *testing.T, name string, domains ...string) ids.CompanyID {
	t.Helper()
	in := CreateCompanyInput{DisplayName: name, Source: "manual"}
	for i, domain := range domains {
		in.Domains = append(in.Domains, CompanyDomainInput{Domain: domain, IsPrimary: i == 0})
	}
	company, err := e.store.CreateCompany(ctx, in)
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

// filedPairField answers the evidence field of the open review pair joining
// the two companies, or "" when the queue holds no open pair for them.
func (e *dedupeEnv) filedPairField(ctx context.Context, t *testing.T, a, b ids.CompanyID) string {
	t.Helper()
	left, right := a.UUID, b.UUID
	if right.String() < left.String() {
		left, right = right, left
	}
	var field string
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(evidence->0->>'field', '') FROM dedupe_candidate
			 WHERE entity_type = 'company' AND disposition = 'open'
			   AND left_company_id = $1 AND right_company_id = $2`, left, right).Scan(&field)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("reading the review pair: %v", err)
	}
	return field
}

func TestACompanyNamedAfterAClaimedDomainIsFiledForReview(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	claimant := e.createNamedCompany(ctx, t, "Litware Inc", "proseware.example")

	named := e.createNamedCompany(ctx, t, "proseware.example")

	if field := e.filedPairField(ctx, t, named, claimant); field != laneDomain {
		t.Fatalf("the pair is filed on %q, want it on the review queue on %q", field, laneDomain)
	}
}

func TestClaimingTheDomainACompanyIsNamedAfterFilesThePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	named := e.createNamedCompany(ctx, t, "contoso.example")
	created := e.createNamedCompany(ctx, t, "Wingtip Toys", "contoso.example")
	if field := e.filedPairField(ctx, t, created, named); field != laneDomain {
		t.Fatalf("a create claiming the domain filed %q, want the pair on %q", field, laneDomain)
	}

	namedFirst := e.createNamedCompany(ctx, t, "fabrikam.example")
	edited := e.createNamedCompany(ctx, t, "Adatum Corp")
	if _, err := e.store.UpdateCompany(ctx, edited, UpdateCompanyInput{
		Domains: &[]CompanyDomainInput{{Domain: "fabrikam.example", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("claiming the domain on an edit: %v", err)
	}
	if field := e.filedPairField(ctx, t, edited, namedFirst); field != laneDomain {
		t.Fatalf("an edit claiming the domain filed %q, want the pair on %q", field, laneDomain)
	}
}

// A name that merely contains the domain spells another host.
func TestASubdomainNameIsNotATwinOfItsParentDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	shop := e.createNamedCompany(ctx, t, "shop.tailspin.example")

	claimant := e.createNamedCompany(ctx, t, "Northwind Health", "tailspin.example")

	if field := e.filedPairField(ctx, t, claimant, shop); field != "" {
		t.Fatalf("a company named after a subdomain was paired on %q, want no pair", field)
	}
}

// The own company refuses every merge, so a pair naming it could only sit on
// the queue unanswerable — from either direction.
func TestTheOwnCompanyIsNeverPairedOnANameDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	website := "https://ourhouse.example"
	anchor, err := e.store.SaveCompany(e.asAdmin(), SaveCompanyInput{DisplayName: "fourthcoffee.example", Website: &website})
	if err != nil {
		t.Fatalf("seeding the own company: %v", err)
	}

	namedAfterOwnDomain := e.createNamedCompany(ctx, t, "ourhouse.example")
	claimsOwnName := e.createNamedCompany(ctx, t, "Coho Winery", "fourthcoffee.example")

	if field := e.filedPairField(ctx, t, namedAfterOwnDomain, anchor.CompanyID); field != "" {
		t.Errorf("a company named after the own domain was paired with the own company on %q", field)
	}
	if field := e.filedPairField(ctx, t, claimsOwnName, anchor.CompanyID); field != "" {
		t.Errorf("a claim of the domain the own company is named after paired them on %q", field)
	}
}

// A domain edit reads names for twins, so it takes the name lock before the
// company row, as a rename does.
func TestADomainEditWaitsOnTheNameLockBeforeTheCompanyRow(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.createNamedCompany(ctx, t, "Lamna Healthcare")

	holder, pid := e.holdCompanyNameLock(ctx, t)
	done := make(chan error, 1)
	go func() {
		_, err := e.store.UpdateCompany(ctx, company, UpdateCompanyInput{
			Domains: &[]CompanyDomainInput{{Domain: "lamna.example", IsPrimary: true}},
		})
		done <- err
	}()

	if waited, finished := waitUntilBlockedBy(t, holder, pid, done); !waited {
		t.Fatalf("the domain edit never waited on the name lock (err=%v)", finished)
	}
	assertParkedBeforeTheCompanyRow(ctx, t, holder, pid)
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock holder: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the domain edit failed once the lock was free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the domain edit never finished after the lock was released")
	}
}

// A company merge takes the name lock before it marks the pair, the order a
// domain claim filing that same pair keeps. Taken the other way round, the
// merge held the pair row while it waited on the name lock, and the claim held
// the name lock while it waited on the row.
func TestACompanyMergeWaitsOnTheNameLockBeforeThePairRow(t *testing.T) {
	e := setupDedupe(t)
	ctx := asArchiver(e)
	first, _ := seedCompanyPair(ctx, t, e)
	open := openCandidates(ctx, t, e, entityCompany)
	if len(open) != 1 {
		t.Fatalf("the seed left %d open candidates, want 1", len(open))
	}
	pair := open[0].ID

	holder, pid := e.holdCompanyNameLock(ctx, t)
	done := make(chan error, 1)
	go func() {
		_, err := e.store.DisposeDedupeCandidate(ctx, pair, "merge", &first)
		done <- err
	}()
	if waited, finished := waitUntilBlockedBy(t, holder, pid, done); !waited {
		t.Fatalf("the merge never waited on the name lock (err=%v)", finished)
	}
	// The pair row is still free while the merge waits: it was not taken first.
	var free bool
	if err := holder.QueryRow(ctx, `SELECT true FROM dedupe_candidate WHERE id = $1 FOR UPDATE NOWAIT`,
		pair).Scan(&free); err != nil {
		t.Fatalf("the parked merge already holds the pair row: %v", err)
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock holder: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the merge failed once the lock was free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the merge never finished after the lock was released")
	}
}
