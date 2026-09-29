// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The partner list declared `sort` and ordered by company_id regardless, so a
// client offering a sort control showed an order the server never applied
// (#827). It honours it now, and these are the three things that had to become
// true: the order, the page that continues it, and the refusal of a field this
// list cannot order by.
//
// Against a real database because all three are SQL — the ORDER BY, the keyset
// tuple, and the typed cast the next page's key binds through.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// partnerSortReader holds the two grants the list asks for.
func partnerSortReader() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"partner": {Read: true}, "company": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}
}

// seedPartnersWithTiers puts three partners on the workspace, each with a
// margin tier that orders differently from the company ids they would
// otherwise be listed by.
func seedPartnersWithTiers(t *testing.T, e *Env) {
	t.Helper()
	for _, tier := range []string{"tier2_20", "tier1_15", "tier3_25"} {
		tier := tier
		e.SeedPartnerCompany(t, "Partner "+tier, &tier, nil)
	}
}

func partnerTiers(ctx context.Context, t *testing.T, store *contacts.Store, in contacts.ListPartnersInput) ([]string, storekit.Page) {
	t.Helper()
	rows, page, err := store.ListPartners(ctx, in)
	if err != nil {
		t.Fatalf("listing partners: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.MarginTier != nil {
			out = append(out, string(*r.MarginTier))
		}
	}
	return out, page
}

func TestThePartnerListIsOrderedByTheSortItIsAsked(t *testing.T) {
	e := Setup(t)
	seedPartnersWithTiers(t, e)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.AdminUser, nil, partnerSortReader())

	spec := "margin_tier"
	ascending, _ := partnerTiers(ctx, t, store, contacts.ListPartnersInput{Sort: &spec})
	if len(ascending) != 3 {
		t.Fatalf("listed %d partner(s) with a tier, want the 3 seeded", len(ascending))
	}
	if !slices.IsSorted(ascending) {
		t.Errorf("sort=margin_tier came back %v", ascending)
	}

	descSpec := "-margin_tier"
	descending, _ := partnerTiers(ctx, t, store, contacts.ListPartnersInput{Sort: &descSpec})
	slices.Reverse(descending)
	if !slices.Equal(ascending, descending) {
		t.Errorf("sort=-margin_tier is not the reverse of sort=margin_tier: %v against %v", descending, ascending)
	}
}

func TestASortedPartnerPageContinuesInItsOwnOrder(t *testing.T) {
	e := Setup(t)
	seedPartnersWithTiers(t, e)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.AdminUser, nil, partnerSortReader())

	spec := "margin_tier"
	two := 2
	first, page := partnerTiers(ctx, t, store, contacts.ListPartnersInput{Sort: &spec, Limit: &two})
	if len(first) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first page = %v, has_more=%v — the limit was not honoured", first, page.HasMore)
	}

	next, _ := partnerTiers(ctx, t, store,
		contacts.ListPartnersInput{Sort: &spec, Cursor: page.NextCursor})
	if len(next) == 0 || next[0] <= first[1] {
		t.Errorf("the second page starts at %v, which does not continue %v", next, first)
	}

	// The same token under a different sort names an axis this page is not
	// ordered by; resuming it would hand back the wrong rows in silence.
	otherSpec := "-margin_tier"
	if _, _, err := store.ListPartners(ctx,
		contacts.ListPartnersInput{Sort: &otherSpec, Cursor: page.NextCursor}); err == nil {
		t.Error("a cursor minted under one sort resumed another")
	}
}

// The company's own name is the Sort component's example of what this server
// cannot order by, because the key would be a joined value the cursor has no
// column for. Refusing it is what keeps the dial honest.
func TestAPartnerSortOnAJoinedValueIsRefused(t *testing.T) {
	e := Setup(t)
	store := contacts.NewStore(e.DB())
	ctx := e.As(e.AdminUser, nil, partnerSortReader())

	spec := "display_name"
	_, _, err := store.ListPartners(ctx, contacts.ListPartnersInput{Sort: &spec})
	var refusal *storekit.SortError
	if !errors.As(err, &refusal) {
		t.Fatalf("sort=display_name returned %v, want a typed sort refusal", err)
	}
	if refusal.Code != storekit.CodeSortFieldNotAllowed {
		t.Errorf("code = %q, want %q", refusal.Code, storekit.CodeSortFieldNotAllowed)
	}
}
