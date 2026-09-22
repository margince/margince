// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A partner's margin tier is what the programme pays, and a seat can read the
// partner register without reading it. Two things have to be true at once, and
// this file keeps them true together: every path that PRINTS the tier withholds
// it, while the accrual that PRICES with it reads the real value. A mask that
// reached the pricing would post the wrong money instead of disclosing none.

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// marginMaskedSeat reads partners and commissions and is withheld the tier.
// RowScopeTeam and not All: a principal reading every row is unbounded, and
// masks do not apply to one, so a fixture on All would assert nothing.
func marginMaskedSeat() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"partner": {Read: true}, "company": {Read: true},
			"commission": {Read: true}, "deal": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
		FieldMasks: []principal.FieldMask{
			{Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways},
		},
	}
}

// assertTierWithheld checks the tier came back BOTH null and named: a null
// nothing names reads as a partner nobody has tiered yet, and a name beside a
// value still on the wire is worse than either half alone.
func assertTierWithheld(t *testing.T, p crmcontracts.Partner) {
	t.Helper()
	if p.MarginTier != nil {
		t.Errorf("margin_tier = %v, want it withheld", *p.MarginTier)
	}
	if p.MaskedFields == nil || !slices.Contains(*p.MaskedFields, "margin_tier") {
		t.Errorf("masked_fields = %v, want margin_tier named", p.MaskedFields)
	}
}

func TestAMaskedSeatReadsThePartnerRegisterWithoutTheTier(t *testing.T) {
	e := Setup(t)
	tier := "tier2_20"
	company := ids.From[ids.CompanyKind](e.SeedPartnerCompany(t, "Northgate Partners", &tier, nil))
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, marginMaskedSeat())

	got, err := e.Contacts.GetPartner(ctx, company)
	if err != nil {
		t.Fatalf("reading the partner: %v", err)
	}
	assertTierWithheld(t, got)

	// The list is where a withheld value is cheapest to harvest in bulk, so it
	// owes the single read's answer.
	page, _, err := e.Contacts.ListPartners(ctx, contacts.ListPartnersInput{})
	if err != nil {
		t.Fatalf("listing partners: %v", err)
	}
	if len(page) == 0 {
		t.Fatal("the partner list came back empty; the mask would then be proving nothing")
	}
	for _, p := range page {
		assertTierWithheld(t, p)
	}

	// The engine's own read, under the SAME masked seat. It is an input to
	// pricing and never meets the wire, so it answers the real tier — this is
	// the assertion that stops a later tidy-up from making the two consistent
	// by masking the accrual into paying the wrong money.
	priced, err := e.Contacts.MarginTierOf(ctx, company)
	if err != nil {
		t.Fatalf("the accrual reading the tier it prices from: %v", err)
	}
	if priced == nil || *priced != tier {
		t.Errorf("MarginTierOf = %v, want %q: masking the pricing input mis-accrues rather than withholds", priced, tier)
	}
}
