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
