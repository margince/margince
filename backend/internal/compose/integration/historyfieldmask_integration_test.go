// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Hiding a value and hiding its past is one motion. A role that withholds a
// deal's money reads that record's trail with the same fields gone — from BOTH
// sides of every diff, because an old_value discloses a figure as completely as
// a new one — and a filter naming a withheld field is refused rather than
// answered with the empty page a reader would take for "it never changed".

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The money a priced deal is seeded and repriced with. Named so the assertions
// can look for the FIGURES on either side of a diff, which is the half of the
// leak a field name alone does not cover.
const (
	seededAmountMinor   = int64(250_000)
	repricedAmountMinor = int64(400_000)
	seededArrMinor      = int64(90_000)
	repricedArrMinor    = int64(150_000)
)

// seedRepricedDeal creates a priced deal through the real writer, raises its
// amount, then raises its ARR. The second edit is what gives the GROUP teeth:
// its images name a field no mask below configures, and the amount's mask has
// to reach it anyway or the deal's size is readable off the recurring half.
func seedRepricedDeal(t *testing.T, e *Env, name string,
	pipeline ids.PipelineID, stage ids.StageID, owner ids.UUID,
) ids.UUID {
	t.Helper()
	currency := "EUR"
	amount, arr := seededAmountMinor, seededArrMinor
	d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage, Source: "manual",
		OwnerID: userIDPtr(&owner), AmountMinor: &amount, ExpectedArrMinor: &arr, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("seeding %s through the real writer: %v", name, err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(d.Id))
	raised, raisedArr := repricedAmountMinor, repricedArrMinor
	if _, err := e.Deals.UpdateDeal(e.Admin(), dealID,
		deals.UpdateDealInput{AmountMinor: &raised}); err != nil {
		t.Fatalf("repricing %s: %v", name, err)
	}
	if _, err := e.Deals.UpdateDeal(e.Admin(), dealID,
		deals.UpdateDealInput{ExpectedArrMinor: &raisedArr}); err != nil {
		t.Fatalf("raising %s's ARR: %v", name, err)
	}
	return ids.UUID(d.Id)
}

// maskedMoneyReader seeds one deal the reader could change and one they could
// not, and binds a rep whose role masks the amount outside their write
// authority — so one reader answers both arms of a conditioned mask.
func maskedMoneyReader(t *testing.T, e *Env) (reader context.Context, mine, theirs ids.UUID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	mine = seedRepricedDeal(t, e, "Mine", pipeline, open, e.Rep1)
	theirs = seedRepricedDeal(t, e, "Theirs", pipeline, open, e.Rep3)

	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{
		objDeal: {Read: true, Update: true}, objPipeline: {Read: true},
	}
	perms.FieldMasks = []principal.FieldMask{
		{Object: objDeal, Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority},
	}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, perms), mine, theirs
}

// seededFigures are the money values as a diff side renders them.
func seededFigures() []string {
	return []string{
		strconv.FormatInt(seededAmountMinor, 10),
		strconv.FormatInt(repricedAmountMinor, 10),
		strconv.FormatInt(seededArrMinor, 10),
		strconv.FormatInt(repricedArrMinor, 10),
	}
}

func TestAMaskedAmountIsGoneFromADealsFieldHistory(t *testing.T) {
	e := Setup(t)
	reader, mine, theirs := maskedMoneyReader(t, e)

	page, err := privacy.ListFieldHistory(reader, e.DB(),
		privacy.FieldHistoryFilter{EntityType: objDeal, EntityID: theirs})
	if err != nil {
		t.Fatalf("reading another team's deal history: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the history is empty, so nothing below proves a mask — the seed wrote no projectable rows")
	}
	figures := seededFigures()
	for _, entry := range page.Entries {
		if slices.Contains(dealMoneyFields, entry.Field) {
			t.Errorf("the history names %s, want the whole money group withheld", entry.Field)
		}
		for _, side := range []*string{entry.OldValue, entry.NewValue} {
			if side != nil && slices.Contains(figures, *side) {
				t.Errorf("%s carries %q — a withheld figure reached the reader through a diff side", entry.Field, *side)
			}
		}
	}

	// The same reader on a deal they could change: the mask lifts exactly where
	// the live read lifts it, so the history is whole.
	amountField := "amount_minor"
	own, err := privacy.ListFieldHistory(reader, e.DB(),
		privacy.FieldHistoryFilter{EntityType: objDeal, EntityID: mine, Field: &amountField})
	if err != nil {
		t.Fatalf("reading their own deal's amount history: %v", err)
	}
	if len(own.Entries) == 0 {
		t.Error("their own deal withheld its amount history, want it whole where they could write the value")
	}

	var refused *values.ParseError
	_, err = privacy.ListFieldHistory(reader, e.DB(),
		privacy.FieldHistoryFilter{EntityType: objDeal, EntityID: theirs, Field: &amountField})
	if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
		t.Errorf("filtering by a withheld field → %v, want the %s refusal, never an empty page", err, auth.CodeFieldMasked)
	}
}

func TestAMaskedAmountIsGoneFromADealsRecordHistoryImages(t *testing.T) {
	e := Setup(t)
	reader, _, theirs := maskedMoneyReader(t, e)

	page, err := privacy.ListRecordHistory(reader, e.DB(),
		privacy.RecordHistoryFilter{EntityType: objDeal, EntityID: theirs})
	if err != nil {
		t.Fatalf("reading another team's record history: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the record history is empty, so nothing below proves a mask")
	}
	for _, entry := range page.Entries {
		for _, image := range []map[string]any{entry.Before, entry.After} {
			for _, field := range dealMoneyFields {
				if _, carried := image[field]; carried {
					t.Errorf("a %s image carries %s, want the money group withheld on both sides", entry.Action, field)
				}
			}
		}
	}
}

// The partner's tier as seeded and as raised. Both are assertion subjects: a
// tier is a closed vocabulary, so the VALUE is what a diff side discloses.
const (
	seededMarginTier = "tier1_15"
	raisedMarginTier = "tier2_20"
)

// The mask names these the way an administrator configures it, which is not
// the entity type the trail is filed under.
const (
	partnerObject   = "partner"
	marginTierField = "margin_tier"
)

// maskedTierReader seeds a company whose partner terms were tiered and then
// re-tiered through the real writer, and binds a reader whose role withholds
// the partner's margin tier.
//
// The partner is a FACET of the company: UpsertPartner audits its images onto
// ('company', company_id), so the tier and every past value of it live in the
// COMPANY's trail while the mask is configured on `partner`.
func maskedTierReader(t *testing.T, e *Env) (reader context.Context, company ids.UUID) {
	t.Helper()
	seeded := seededMarginTier
	company = e.SeedPartnerCompany(t, "Tiered", &seeded, &e.Rep1)
	raised := raisedMarginTier
	if _, err := e.Contacts.UpsertPartner(e.PartnerSeat(), contacts.UpsertPartnerInput{
		CompanyID: companyIDOf(company), PartnerRole: "consulting", MarginTier: &raised,
	}); err != nil {
		t.Fatalf("re-tiering the partner: %v", err)
	}

	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{objCompany: {Read: true}}
	perms.FieldMasks = []principal.FieldMask{
		{Object: partnerObject, Field: marginTierField, Condition: principal.MaskAlways},
	}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, perms), company
}

func TestAMaskedMarginTierIsGoneFromItsCompanysFieldHistory(t *testing.T) {
	e := Setup(t)
	reader, company := maskedTierReader(t, e)

	page, err := privacy.ListFieldHistory(reader, e.DB(),
		privacy.FieldHistoryFilter{EntityType: objCompany, EntityID: company})
	if err != nil {
		t.Fatalf("reading the company's field history: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the history is empty, so nothing below proves a mask — the seed wrote no projectable rows")
	}
	tiers := []string{seededMarginTier, raisedMarginTier}
	for _, entry := range page.Entries {
		if entry.Field == marginTierField {
			t.Errorf("the history names %s, want the partner's mask to reach the company's trail", entry.Field)
		}
		for _, side := range []*string{entry.OldValue, entry.NewValue} {
			if side != nil && slices.Contains(tiers, *side) {
				t.Errorf("%s carries %q — a withheld tier reached the reader through a diff side", entry.Field, *side)
			}
		}
	}

	var refused *values.ParseError
	field := marginTierField
	_, err = privacy.ListFieldHistory(reader, e.DB(),
		privacy.FieldHistoryFilter{EntityType: objCompany, EntityID: company, Field: &field})
	if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
		t.Errorf("filtering by a withheld field → %v, want the %s refusal, never an empty page", err, auth.CodeFieldMasked)
	}
}

func TestAMaskedMarginTierIsGoneFromItsCompanysRecordHistoryImages(t *testing.T) {
	e := Setup(t)
	reader, company := maskedTierReader(t, e)

	page, err := privacy.ListRecordHistory(reader, e.DB(),
		privacy.RecordHistoryFilter{EntityType: objCompany, EntityID: company})
	if err != nil {
		t.Fatalf("reading the company's record history: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the record history is empty, so nothing below proves a mask")
	}
	for _, entry := range page.Entries {
		for _, image := range []map[string]any{entry.Before, entry.After} {
			if _, carried := image[marginTierField]; carried {
				t.Errorf("a %s image carries %s, want the partner's tier withheld on both sides",
					entry.Action, marginTierField)
			}
		}
	}
}
