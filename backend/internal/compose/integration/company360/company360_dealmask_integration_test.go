// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// A field mask withholds a deal's amount on the deal list. Everything here is a
// SECOND way to the same number: an account page prints the deal's figure in
// its pipeline band and folds every won deal into a lifetime total.
//
// A unit test cannot fail any of it. The mask is rendered INTO the statement —
// a CASE that nulls the column on the rows the caller could not write — so what
// has to be true is that Postgres agrees, and that is only knowable against a
// real database.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const maskedDealAmount = int64(250_000)

// maskedRep is a seat holding everything the account page needs, with the
// amount masked outside its own write authority.
//
// RowScopeTeam and not All: auth.Unbounded reads row_scope=all as "every row"
// and skips masks outright, so a fixture on All would assert nothing about
// masking at all.
func maskedRep() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"company": {Read: true}, "deal": {Read: true, Update: true},
			"pipeline": {Read: true}, "contact": {Read: true},
			"relationship": {Read: true}, "activity": {Read: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
		FieldMasks: []principal.FieldMask{
			{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority},
		},
	}
}

// seedTwoPricedDeals puts one deal the reader owns and one another team's on
// the same account, both priced the same, so any difference in what comes back
// is the mask and not the money.
func seedTwoPricedDeals(t *testing.T, e *integration.Env, status string) (company ids.UUID, mine, theirs ids.UUID) {
	t.Helper()
	pipelineID, stage, _ := integration.DealFixture(t, e)
	company = e.SeedCompany(t, "Brandt GmbH", nil)
	mine = e.SeedDeal(t, "Mine", pipelineID, stage, &e.Rep1)
	theirs = e.SeedDeal(t, "Theirs", pipelineID, stage, &e.Rep3)
	for _, id := range []ids.UUID{mine, theirs} {
		// fx_rate_to_base with the close, because deal_closed_fx refuses a
		// priced deal that closed without the rate it was converted at — the
		// frozen rate is what makes a lifetime total re-derivable later.
		e.WsExec(t, `UPDATE deal SET company_id = $2, amount_minor = $3, amount_minor_base = $3,
			currency = 'EUR', status = $4,
			closed_at = CASE WHEN $4 = 'won' THEN now() ELSE NULL END,
			fx_rate_to_base = CASE WHEN $4 = 'won' THEN 1 ELSE NULL END
			WHERE id = $1`, id, company, maskedDealAmount, status)
	}
	return company, mine, theirs
}

// The lifetime total folds every won deal on the account. A masked deal must
// contribute nothing — a total is the one place a withheld figure reappears
// whole, and it reappears without anything on screen saying it did.
func TestTheAccountsWonLifetimeExcludesAMaskedDeal(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, _, _ := seedTwoPricedDeals(t, e, "won")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, maskedRep())
	view, err := svc.Assemble(rep, ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if view.Deals == nil {
		t.Fatalf("the deals band was withheld entirely (sections_omitted=%v) — the assertion "+
			"below would not have run", view.SectionsOmitted)
	}
	got := view.Deals.WonLifetime.AmountMinor
	if got == nil {
		t.Fatal("won_lifetime carried no figure at all; want the reader's own deal counted")
	}
	// Exactly one of the two deals: the reader's own. Asserting the VALUE and
	// not merely "less than both" is what catches a mask that zeroed the whole
	// total, which would hide the reader's own money and read as a fix.
	if *got != maskedDealAmount {
		t.Errorf("won_lifetime = %d, want %d — one masked deal and one the reader owns, so a total "+
			"of %d means the masked deal was summed and %d means the reader's own was dropped",
			*got, maskedDealAmount, 2*maskedDealAmount, 0)
	}
}

// And the band that lists the deals: the reader's own keeps its figure, the
// other team's keeps its NAME and loses the number. Withholding the deal
// entirely would say the account has no business rather than that the size is
// not this reader's to see.
func TestTheAccountsPipelineBandNullsAMaskedAmount(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company, mine, theirs := seedTwoPricedDeals(t, e, "open")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, maskedRep())
	view, err := svc.Assemble(rep, ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if view.Deals == nil {
		t.Fatalf("the deals band was withheld entirely (sections_omitted=%v)", view.SectionsOmitted)
	}
	seen := map[ids.UUID]*int64{}
	for _, d := range view.Deals.Data {
		var amount *int64
		if d.Amount != nil {
			amount = d.Amount.AmountMinor
		}
		seen[ids.UUID(d.DealId)] = amount
	}
	own, listed := seen[mine]
	if !listed {
		t.Fatal("the reader's own open deal was not listed")
	}
	if own == nil || *own != maskedDealAmount {
		t.Errorf("the reader's own deal came back worth %v, want %d — the mask swallowed money "+
			"inside its own write authority", own, maskedDealAmount)
	}
	other, listed := seen[theirs]
	if !listed {
		t.Fatal("another team's deal vanished from the band — a deal a rep may READ should still name itself")
	}
	if other != nil {
		t.Errorf("another team's amount reached the account page as %d", *other)
	}
}
