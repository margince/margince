// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A column the accounts list SHOWS is a column the accounts list SORTS BY.
//
// The list draws eight columns and its vocabulary reached three of them, so
// five headers were dead controls a reader had to learn to ignore. Three of the
// five are DERIVED — the website comes off the primary domain row and the two
// counts are read per page rather than stored — so each is asserted the way
// that matters: the order the list gives agrees with the values it prints.
// That is what makes the count in the ORDER BY the same count as the one on
// screen rather than a second derivation that will drift.
//
// The relationship column is deliberately absent. An account can be a partner
// AND a customer, so there is no single value to order by.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// orgsIn lists the accounts this caller sees under one sort spec, in order.
func orgsIn(ctx context.Context, t *testing.T, e *Env, spec string) []crmcontracts.Organization {
	t.Helper()
	rows, _, err := e.People.ListOrganizations(ctx, people.ListOrganizationsInput{Sort: &spec})
	if err != nil {
		t.Fatalf("ListOrganizations(sort=%s): %v", spec, err)
	}
	return rows
}

func orgIDsIn(ctx context.Context, t *testing.T, e *Env, spec string) []ids.UUID {
	t.Helper()
	rows := orgsIn(ctx, t, e, spec)
	out := make([]ids.UUID, len(rows))
	for i, o := range rows {
		out[i] = ids.UUID(o.Id)
	}
	return out
}

// The two plain columns the vocabulary simply did not name.
func TestTheAccountsListSortsByTheDescriptionAndLifecycleItDraws(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	// Seeded so that neither answer can come from insertion order or from the
	// name, which is the sort the list already had.
	zeta := seedAccount(t, e, people.CreateOrganizationInput{
		DisplayName: "Zeta Holding", Description: strPtr("An early note"),
	})
	alma := seedAccount(t, e, people.CreateOrganizationInput{
		DisplayName: "Alma Werke", Description: strPtr("Zero interest so far"),
	})
	setLifecycle(t, e, zeta, "customer")
	setLifecycle(t, e, alma, "target")

	assertIDOrder(t, orgIDsIn(ctx, t, e, "description"), []ids.UUID{zeta, alma},
		"description ascending")
	assertIDOrder(t, orgIDsIn(ctx, t, e, "lifecycle"), []ids.UUID{zeta, alma},
		"lifecycle ascending — customer before target")
}

// The Website header orders by the HOST it prints, off the primary domain row
// that website_url is derived from.
func TestTheAccountsListSortsByTheWebsiteItDraws(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	// Names and hosts deliberately disagree, so the answer cannot come from the
	// sort this list already had.
	// TWO domains, the non-primary one created first and sorting last: the
	// column prints the PRIMARY host, so a sort that read any live domain —
	// or read them oldest-first — would key this account on "zzz.example" and
	// put it the other side of the account below.
	zeta := seedAccount(t, e, people.CreateOrganizationInput{
		DisplayName: "Zeta Holding",
		Domains: []people.OrgDomainInput{
			{Domain: "zzz.example"},
			{Domain: "alma.example", IsPrimary: true},
		},
	})
	alma := seedAccount(t, e, people.CreateOrganizationInput{
		DisplayName: "Alma Werke",
		Domains:     []people.OrgDomainInput{{Domain: "zeta.example", IsPrimary: true}},
	})
	// No domain at all: nothing to print and nothing to order by, so it sits in
	// the tail rather than ahead of every named host.
	nameless := e.SeedOrg(t, "Mercator", &e.Rep1)

	assertIDOrder(t, orgIDsIn(ctx, t, e, "website_url"), []ids.UUID{zeta, alma, nameless},
		"website ascending — alma.example before zeta.example, no host last")

	// And the order agrees with what the rows print, which is the claim that
	// makes the expression the same derivation rather than a second one.
	rows := orgsIn(ctx, t, e, "website_url")
	if len(rows) != 3 || rows[0].WebsiteUrl == nil || *rows[0].WebsiteUrl != "https://alma.example" {
		t.Fatalf("the first row prints %v, and the page was ordered by the host it comes from", rows[0].WebsiteUrl)
	}
}

// The Contacts header orders by the number it prints — counted per row, under
// the same scope the page's own count applies.
func TestOrderingAccountsByContactsAgreesWithTheCountTheyShow(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	busy := e.SeedOrg(t, "Alma Werke", &e.Rep1)
	quiet := e.SeedOrg(t, "Zeta Holding", &e.Rep1)
	for _, name := range []string{"One", "Two", "Three"} {
		employPerson(t, e, e.SeedPerson(t, name+" at Alma", &e.Rep1), busy, nil)
	}
	employPerson(t, e, e.SeedPerson(t, "Only at Zeta", &e.Rep1), quiet, nil)

	// Ascending puts the quieter account first, which is the reverse of the
	// name order the list already had.
	assertIDOrder(t, orgIDsIn(ctx, t, e, "contact_count"), []ids.UUID{quiet, busy},
		"contacts ascending")

	rows := orgsIn(ctx, t, e, "-contact_count")
	if len(rows) != 2 {
		t.Fatalf("listed %d accounts, want 2", len(rows))
	}
	// The ordering and the printed number are the same number.
	if rows[0].ContactCount == nil || *rows[0].ContactCount != 3 {
		t.Errorf("the account the page put first prints %v contacts, not the 3 it was ordered by",
			rows[0].ContactCount)
	}
	if rows[1].ContactCount == nil || *rows[1].ContactCount != 1 {
		t.Errorf("the second account prints %v contacts, not 1", rows[1].ContactCount)
	}
}

// A count this reader is not shown orders the page by NOTHING.
//
// The open-pipeline figure is withheld from a role without computed_field:read
// (STATE-4), and a page ordered by a number they are refused would disclose it
// through the order — read it both ways and the largest pipeline is at whichever
// end. Every such row sits in the tail instead.
func TestAWithheldCountOrdersTheAccountsListByNothing(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)

	big := e.SeedOrg(t, "Alma Werke", &e.Rep1)
	small := e.SeedOrg(t, "Zeta Holding", &e.Rep1)
	for i := range 3 {
		seedDealForCompany(t, e, "Alma deal "+string(rune('A'+i)), pipeline, open, big)
	}
	seedDealForCompany(t, e, "Zeta deal", pipeline, open, small)

	// Admitted first: a reader who IS shown the count is ordered by it, so the
	// silence below is the grant's doing and not a sort that never worked. The
	// admin holds computed_field:read; AccountRepPerms deliberately does not.
	assertIDOrder(t, orgIDsIn(e.Admin(), t, e, "-open_deal_count"), []ids.UUID{big, small},
		"open deals descending, for a reader who may see them")

	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	rows := orgsIn(blind, t, e, "-open_deal_count")
	if len(rows) != 2 {
		t.Fatalf("listed %d accounts, want 2", len(rows))
	}
	for _, o := range rows {
		if o.OpenDealCount != nil {
			t.Fatalf("the reader without computed_field:read was shown a count of %d", *o.OpenDealCount)
		}
	}
	// Both rows in the tail, so the page falls back to its tie-breaker and the
	// order says nothing about the pipelines behind it. Newest first is what
	// that tie-breaker gives, and Zeta was seeded last.
	assertIDOrder(t, []ids.UUID{ids.UUID(rows[0].Id), ids.UUID(rows[1].Id)},
		[]ids.UUID{small, big}, "open deals descending, for a reader shown none")
}

// seedAccount creates one account through the real writer, owned by Rep1.
func seedAccount(t *testing.T, e *Env, in people.CreateOrganizationInput) ids.UUID {
	t.Helper()
	in.OwnerID, in.Source = userIDPtr(&e.Rep1), "manual"
	org, err := e.People.CreateOrganization(e.Admin(), in)
	if err != nil {
		t.Fatalf("seeding %q: %v", in.DisplayName, err)
	}
	return ids.UUID(org.Id)
}

// setLifecycle moves an account's stage through the real update path, so the
// column the sort reads holds a value a writer really put there.
func setLifecycle(t *testing.T, e *Env, org ids.UUID, to string) {
	t.Helper()
	if _, err := e.People.UpdateOrganization(e.Admin(), ids.From[ids.OrganizationKind](org),
		people.UpdateOrganizationInput{Lifecycle: &to}); err != nil {
		t.Fatalf("setting the lifecycle to %q: %v", to, err)
	}
}
