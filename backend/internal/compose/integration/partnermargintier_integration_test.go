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

// marginMaskedWriter is the seat the hazard needs: it may change a partner and
// may not read the tier on it, so every edit it sends carries the null it was
// handed in place of the stored tier.
func marginMaskedWriter() principal.Permissions {
	perms := marginMaskedSeat()
	perms.Objects["partner"] = principal.ObjectGrant{Read: true, Update: true}
	perms.Objects["company"] = principal.ObjectGrant{Read: true, Update: true}
	return perms
}

// A field a caller cannot READ is not theirs to clear. The upsert coalesces an
// absent tier onto the stored one, so an edit that does not name it leaves it —
// which is the only answer that works, since a masked writer can never name it
// and refusing would turn its every unrelated edit into a 422.
func TestAMaskedWriterEditsAPartnerWithoutWipingTheTier(t *testing.T) {
	e := Setup(t)
	tier := "tier2_20"
	// Owned by the writer, because write authority over the company is what the
	// promotion asks for and a team-scoped seat holds it on its own rows.
	company := ids.From[ids.CompanyKind](e.SeedPartnerCompany(t, "Northgate Partners", &tier, &e.Rep1))
	step := "renew the agreement"

	// The masked client's own shape: it prefills from what it read, and what it
	// read carries no tier.
	if _, err := e.Contacts.UpsertPartner(e.As(e.Rep1, []ids.UUID{e.Team1}, marginMaskedWriter()),
		contacts.UpsertPartnerInput{
			CompanyID: company, PartnerRole: "consulting", MarginTier: nil, NextStep: &step,
		}); err != nil {
		t.Fatalf("a masked seat editing the partner it may write: %v", err)
	}

	after, err := e.Contacts.GetPartner(e.As(e.AdminUser, nil, partnerReadingPerms()), company)
	if err != nil {
		t.Fatalf("reading the partner back: %v", err)
	}
	if after.MarginTier == nil || string(*after.MarginTier) != tier {
		t.Errorf("margin_tier = %v after an unrelated edit by a masked writer, want %q kept: "+
			"a seat that cannot read a field cannot be the one that clears it", after.MarginTier, tier)
	}
	if after.NextStep == nil || *after.NextStep != step {
		t.Errorf("next_step = %v, want the edit the writer actually made", after.NextStep)
	}
}

// The tier is not clearable through this endpoint by ANYONE, masked or not: the
// coalesce that preserves it above has no counterpart gesture, and the request
// schema's `null` member is inert for this field. Pinned rather than left for
// the next author to discover from a write that silently did nothing.
func TestAnExplicitNullTierDoesNotClearItForAnUnmaskedWriterEither(t *testing.T) {
	e := Setup(t)
	tier := "tier1_15"
	company := ids.From[ids.CompanyKind](e.SeedPartnerCompany(t, "Vestergaard", &tier, nil))
	cleared := ""

	if _, err := e.Contacts.UpsertPartner(e.As(e.AdminUser, nil, partnerWritingPerms()),
		contacts.UpsertPartnerInput{
			CompanyID: company, PartnerRole: "consulting", MarginTier: &cleared,
		}); err == nil {
		t.Fatal("an empty tier was accepted; the column's CHECK holds the vocabulary closed")
	}

	after, err := e.Contacts.GetPartner(e.As(e.AdminUser, nil, partnerReadingPerms()), company)
	if err != nil {
		t.Fatalf("reading the partner back: %v", err)
	}
	if after.MarginTier == nil || string(*after.MarginTier) != tier {
		t.Errorf("margin_tier = %v, want %q: this endpoint has no gesture that clears it", after.MarginTier, tier)
	}
}

// partnerReadingPerms holds the two grants a partner read asks for, unmasked.
func partnerReadingPerms() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"partner": {Read: true}, "company": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}
}

// partnerWritingPerms adds the write half, which promotion asks for on both.
func partnerWritingPerms() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"partner": {Read: true, Update: true}, "company": {Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	}
}
