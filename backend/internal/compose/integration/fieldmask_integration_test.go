// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// dealMoneyFields is the money on a deal: one fact in three columns, so a mask
// on any of them withholds all three. A currency left standing beside a
// withheld amount reads as a priced deal with its figure missing, and an ARR
// left standing discloses the size of the deal the mask was meant to hide.
var dealMoneyFields = []string{"amount_minor", "expected_arr_minor", "currency"}

// assertMoneyWithheld checks the money came back BOTH null and named. The two
// halves are one fact: a null nothing names is indistinguishable from a field
// nobody filled in, and a name beside a value still on the wire is worse than
// either half alone — which is why they are asserted in one place rather than
// left to drift apart per call site.
func assertMoneyWithheld(t *testing.T, d crmcontracts.Deal) {
	t.Helper()
	if d.AmountMinor != nil || d.ExpectedArrMinor != nil || d.Currency != nil {
		t.Errorf("the money read back as amount %v arr %v currency %v, want all three withheld",
			d.AmountMinor, d.ExpectedArrMinor, d.Currency)
	}
	if d.MaskedFields == nil {
		t.Fatalf("masked_fields is absent, want %v named — a withheld null must say it was withheld", dealMoneyFields)
	}
	for _, field := range dealMoneyFields {
		if !slices.Contains(*d.MaskedFields, field) {
			t.Errorf("masked_fields = %v, want it to name %s", *d.MaskedFields, field)
		}
	}
	if len(*d.MaskedFields) != len(dealMoneyFields) {
		t.Errorf("masked_fields = %v, want exactly %v", *d.MaskedFields, dealMoneyFields)
	}
}

// A field mask withholds one column of a readable row, and the money on a deal
// is one fact in three columns, so a mask on the amount takes the ARR and the
// currency with it. A rep reads every deal in the workspace; another team's
// money comes back null and every withheld field is named in masked_fields,
// their own team's money is theirs whole, and a sort by the masked column is
// refused — ordering by a value is reading it. The mask is a row property the
// admin does not carry.
func TestARepReadsEveryDealButNotAnotherTeamsMoney(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	mine := e.SeedDeal(t, "Mine", pipeline, open, &e.Rep1)
	theirs := e.SeedDeal(t, "Theirs", pipeline, open, &e.Rep3)
	amount, arr := int64(250000), int64(90000)
	for _, id := range []ids.UUID{mine, theirs} {
		// The ARR is seeded alongside the amount because a column nobody wrote
		// reads back null whether or not it is withheld: only a populated one
		// can prove the mask reached it.
		e.WsExec(t, `UPDATE deal SET amount_minor = $2, expected_arr_minor = $3, currency = 'EUR' WHERE id = $1`,
			id, amount, arr)
	}

	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}, "pipeline": {Read: true}}
	perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	got, err := e.Deals.GetDeal(rep, ids.From[ids.DealKind](theirs), 0)
	if err != nil {
		t.Fatalf("a rep reading another team's deal: %v", err)
	}
	assertMoneyWithheld(t, got)
	own, err := e.Deals.GetDeal(rep, ids.From[ids.DealKind](mine), 0)
	if err != nil || own.AmountMinor == nil || *own.AmountMinor != amount ||
		own.ExpectedArrMinor == nil || *own.ExpectedArrMinor != arr || own.Currency == nil || own.MaskedFields != nil {
		t.Errorf("the rep's own deal = amount %v arr %v currency %v masked %v (%v), want the whole money surface and no mask",
			own.AmountMinor, own.ExpectedArrMinor, own.Currency, own.MaskedFields, err)
	}

	page, _, err := e.Deals.ListDeals(rep, deals.ListDealsInput{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ids.UUID]bool{}
	for _, d := range page {
		seen[ids.UUID(d.Id)] = true
		switch ids.UUID(d.Id) {
		case mine:
			if d.AmountMinor == nil {
				t.Error("the list withheld the rep's own amount")
			}
		case theirs:
			// The page owes the same answer as the single read: a list is where
			// a withheld figure is cheapest to harvest in bulk.
			assertMoneyWithheld(t, d)
		}
	}
	if !seen[mine] || !seen[theirs] {
		t.Errorf("the list shows %v, want both deals", seen)
	}
	sort := "-amount_minor"
	var refused *values.ParseError
	if _, _, err := e.Deals.ListDeals(rep, deals.ListDealsInput{Sort: &sort}); !errors.As(err, &refused) || refused.Code != "field_masked" {
		t.Errorf("sorting by a masked column → %v, want the field_masked refusal", err)
	}

	// The admin carries no mask.
	full, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](theirs), 0)
	if err != nil || full.AmountMinor == nil || full.MaskedFields != nil {
		t.Errorf("the admin's read = amount %v masked %v (%v), want the amount", full.AmountMinor, full.MaskedFields, err)
	}
}

// The list surface's twin of the report case: a rep whose role lost
// deal.update owns write authority over NO row, so the mask holds on their
// own deal too — revoking a verb must never widen what a role reads.
func TestARevokedUpdateVerbMasksTheRepsOwnMoney(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	mine := e.SeedDeal(t, "Mine", pipeline, open, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET amount_minor = $2, expected_arr_minor = $3, currency = 'EUR' WHERE id = $1`,
		mine, int64(250000), int64(90000))

	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{"deal": {Read: true}, "pipeline": {Read: true}}
	perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	got, err := e.Deals.GetDeal(rep, ids.From[ids.DealKind](mine), 0)
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyWithheld(t, got)
}

// maskedDealSeat is a rep whose role withholds one deal column on every row.
// Row scope stays team: a reader of every row is withheld nothing, so a fixture
// on that scope would assert nothing.
func maskedDealSeat(t *testing.T, e *Env, field string) context.Context {
	t.Helper()
	perms := AccountRepPerms
	perms.FieldMasks = []principal.FieldMask{
		{Object: "deal", Field: field, Condition: principal.MaskAlways},
	}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
}

// seedPartneredDeal is one deal with a partner the reader can open, so what
// withholds the pair is the ROLE and never the partner's own visibility.
func seedPartneredDeal(t *testing.T, e *Env) (ids.DealID, ids.CompanyID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1))
	partner := companyIDOf(e.SeedPartnerCompany(t, "Northgate Partners", nil, nil))
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{PartnerCompanyID: &partner}); err != nil {
		t.Fatalf("linking the deal to its partner: %v", err)
	}
	return deal, partner
}

// A role mask on the partner withholds what the partner DID as well, and names
// it: "sourced" beside a null partner tells the reader that some partner
// brought the deal, which is the fact the mask was set to withhold.
func TestAMaskedPartnerTakesTheAttributionWithIt(t *testing.T) {
	e := Setup(t)
	deal, _ := seedPartneredDeal(t, e)
	rep := maskedDealSeat(t, e, "partner_company_id")

	got, err := e.Deals.GetDeal(rep, deal, 0)
	if err != nil {
		t.Fatalf("a rep reading a deal whose partner their role withholds: %v", err)
	}
	if got.PartnerCompanyId != nil || got.PartnerAttribution != nil {
		t.Errorf("the deal named partner %v attribution %v, want both withheld",
			got.PartnerCompanyId, got.PartnerAttribution)
	}
	assertMaskNames(t, got, "partner_company_id", "partner_attribution")

	// The other direction: an unmasked seat reads the pair whole, or the
	// assertion above would hold against a read that withholds it from everybody.
	full, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil || full.PartnerCompanyId == nil || full.PartnerAttribution == nil {
		t.Errorf("the admin's read = partner %v attribution %v (%v), want the pair",
			full.PartnerCompanyId, full.PartnerAttribution, err)
	}
}

// Narrowing by a column is reading it. The target here is one this reader can
// open, so every other arm of the filter answers, and what comes back is the
// binding the projection just declined to name — so the list refuses instead.
// An empty page would be just as safe and would teach the caller the value.
func TestTheDealListRefusesAFilterOverAColumnTheRoleWithholds(t *testing.T) {
	e := Setup(t)
	deal, partner := seedPartneredDeal(t, e)
	rep := maskedDealSeat(t, e, "partner_company_id")

	var refused *values.ParseError
	_, _, err := e.Deals.ListDeals(rep, deals.ListDealsInput{PartnerCompanyID: &partner})
	if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
		t.Errorf("filtering by a withheld column → %v, want the %s refusal, never a page", err, auth.CodeFieldMasked)
	}
	// The attribution is refused with the partner it describes: it is withheld
	// as a consequence, and a filter over it recovers the same fact.
	sourced := "sourced"
	_, _, err = e.Deals.ListDeals(rep, deals.ListDealsInput{PartnerAttribution: &sourced})
	if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
		t.Errorf("filtering by what the withheld partner did → %v, want the %s refusal", err, auth.CodeFieldMasked)
	}
	// Attribution PRESENCE asks the same question under a name of its own, and
	// the page's membership answers it: these are the deals that name a partner.
	partnered := true
	_, _, err = e.Deals.ListDeals(rep, deals.ListDealsInput{PartnerSourced: &partnered})
	if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
		t.Errorf("filtering by whether a withheld partner is named → %v, want the %s refusal",
			err, auth.CodeFieldMasked)
	}
	// The same filter for a seat whose role withholds nothing still narrows, or
	// the refusal above would have closed the oracle by breaking the feature.
	page, _, err := e.Deals.ListDeals(e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms),
		deals.ListDealsInput{PartnerCompanyID: &partner})
	if err != nil {
		t.Fatalf("an unmasked seat filtering by the same partner: %v", err)
	}
	if len(page) != 1 || ids.UUID(page[0].Id) != deal.UUID {
		t.Errorf("the unmasked filter returned %d deal(s), want the one on that partner", len(page))
	}
}
