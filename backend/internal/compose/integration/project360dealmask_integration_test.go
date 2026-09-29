// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The project page's header folds the project's deals into open money and won
// money. A field mask withholds a deal's amount on the deal list, and a total
// is where a withheld figure reappears whole: a project carrying one deal
// makes its header exactly that deal's amount, so a header that summed it
// hands back the number GET /deals refused.
//
// A unit test cannot fail any of this. The mask is a predicate rendered INTO
// the statement, so what has to be true is that Postgres agrees, and that is
// only knowable against a real database.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const maskedProjectDealAmount = int64(250_000)

// projectMaskedRepPerms reads every section of the page with the deal's amount
// withheld outside its own write authority.
//
// Update on the deal is what leaves that condition answerable: a role carrying
// no update verb holds authority over no row at all, which withholds the
// column everywhere and zeroes both figures whatever the sum does with them.
var projectMaskedRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"project":               {Read: true},
		"company":               {Read: true},
		"contact":               {Read: true},
		"deal":                  {Read: true, Update: true},
		"activity":              {Read: true},
		"relationship":          {Read: true},
		"contract":              {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
	FieldMasks: []principal.FieldMask{
		{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority},
	},
}

// seedPricedProjectDeals files two equally priced deals under one project, one
// the reader owns and one another team's, so a header differing from twice the
// amount differs by the mask rather than by the money.
func seedPricedProjectDeals(t *testing.T, e *Env, status string) ids.ProjectID {
	t.Helper()
	pipeline, stage, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Brandt GmbH", &e.Rep1)
	project := seedProject(e.Admin(), t, e, "ERP rollout", company, &e.Rep1).ID
	for _, owner := range []ids.UUID{e.Rep1, e.Rep3} {
		deal := e.SeedDeal(t, "ERP licences", pipeline, stage, &owner)
		// fx_rate_to_base travels with the close, because deal_closed_fx refuses
		// a priced deal that closed without the rate it was converted at.
		e.WsExec(t, `UPDATE deal SET project_id = $2, company_id = $3, amount_minor = $4::bigint,
			amount_minor_base = CASE WHEN $5::text = 'won' THEN $4::bigint END,
			currency = 'EUR', status = $5::text,
			closed_at = CASE WHEN $5::text = 'won' THEN now() END,
			fx_rate_to_base = CASE WHEN $5::text = 'won' THEN 1 END
			WHERE id = $1`, deal, project.UUID, company, maskedProjectDealAmount, status)
	}
	return project
}

// projectHeader assembles the page as the given seat and answers its header. A
// mask narrows a figure there and never withholds the section: a header that
// vanished would say the project has no deals and no work.
func projectHeader(t *testing.T, e *Env, project ids.ProjectID,
	perms principal.Permissions,
) crmcontracts.Project360Rollups {
	t.Helper()
	page, err := project360Service(e, time.Now().UTC()).
		Assemble(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), project)
	if err != nil {
		t.Fatalf("assemble the project page: %v", err)
	}
	if page.Rollups == nil {
		t.Fatalf("the header was withheld entirely (sections_omitted=%v) — a mask takes a FIGURE, "+
			"never the section that would have carried it", page.SectionsOmitted)
	}
	return *page.Rollups
}

// headerAmount is one header figure. Project360Rollups requires both Money
// fields, so an absent amount is a broken contract rather than a narrow answer.
func headerAmount(t *testing.T, figure string, money crmcontracts.Money) int64 {
	t.Helper()
	if money.AmountMinor == nil {
		t.Fatalf("%s carried no amount at all; a narrowed total is still a figure", figure)
	}
	return *money.AmountMinor
}

// Exactly one of the two deals, the reader's own. Asserting the VALUE rather
// than "less than both" is what catches a mask that zeroed the whole header,
// which would hide the reader's own money and read as a fix.
func assertOneDealCounted(t *testing.T, figure string, money crmcontracts.Money) {
	t.Helper()
	got := headerAmount(t, figure, money)
	if got != maskedProjectDealAmount {
		t.Errorf("%s = %d, want %d — one masked deal and one the reader owns, so %d means the "+
			"masked deal was summed and %d means the reader's own was dropped",
			figure, got, maskedProjectDealAmount, 2*maskedProjectDealAmount, 0)
	}
}

func TestTheProjectHeadersWonMoneyExcludesAMaskedDeal(t *testing.T) {
	e := Setup(t)
	project := seedPricedProjectDeals(t, e, "won")
	header := projectHeader(t, e, project, projectMaskedRepPerms)
	assertOneDealCounted(t, "won_deal_value", header.WonDealValue)
}

// The open half folds through a different expression — each deal's own amount
// while it is quoted in the base currency — so it takes its own assertion. The
// two must move together: a mask reaching one total and not the other puts the
// same figure back on the same page.
func TestTheProjectHeadersOpenMoneyExcludesAMaskedDeal(t *testing.T) {
	e := Setup(t)
	project := seedPricedProjectDeals(t, e, "open")
	header := projectHeader(t, e, project, projectMaskedRepPerms)
	assertOneDealCounted(t, "open_deal_value", header.OpenDealValue)
}

// A mask on every row leaves the reader no deal to price, and the header then
// reads zero in the installation's own currency — not a refusal, and not the
// figure. Zero is the honest answer this surface can give: Project360Rollups
// requires both Money fields and carries no excluded_by_permission beside them,
// so a reader is not told the total was narrowed. The report engine says so and
// the page cannot, which is a difference in the CONTRACT rather than in the
// rule the two apply.
func TestTheProjectHeaderReadsZeroWhereEveryDealsMoneyIsMasked(t *testing.T) {
	e := Setup(t)
	project := seedPricedProjectDeals(t, e, "won")
	perms := projectMaskedRepPerms
	perms.FieldMasks = []principal.FieldMask{
		{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways},
	}
	won := projectHeader(t, e, project, perms).WonDealValue
	if got := headerAmount(t, "won_deal_value", won); got != 0 {
		t.Errorf("won_deal_value = %d, want 0 — this reader may price no deal on the project", got)
	}
	if won.Currency == nil || *won.Currency == "" {
		t.Error("a zero total came back with no currency; the figure is in the installation's base")
	}
}
