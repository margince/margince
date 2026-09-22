// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RowScopeTeam and not All, deliberately: Unbounded reads row_scope=all as
// "every row" and skips masks outright, so a fixture on All would assert
// nothing about what a mask withholds. A masked seat is a row-scoped seat by
// construction.
func principalWithMasks(masks ...principal.FieldMask) principal.Principal {
	return principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeTeam,
			FieldMasks: masks,
		},
	}
}

func TestAMaskOnTheAmountWithholdsTheCurrencyBesideIt(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways})
	for _, field := range []string{"amount_minor", "expected_arr_minor", "currency"} {
		if len(masksWithholding(p, "deal", field)) == 0 {
			t.Errorf("deal.%s is readable beside a withheld amount; a currency beside a "+
				"withheld figure reads as a priced deal with its amount missing", field)
		}
	}
}

// The relation is directed: a withheld currency says nothing about the figure,
// so withholding the amount too would hide more than was asked.
func TestAMaskOnTheCurrencyDoesNotWithholdTheAmount(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "deal", Field: "currency", Condition: principal.MaskAlways})
	if len(masksWithholding(p, "deal", "amount_minor")) != 0 {
		t.Error("a mask on the currency alone withheld the amount")
	}
}

// Cross-object: the tier on a commission entry is the same fact, and a mask
// that leaves it standing protects nothing.
func TestAPartnerMarginMaskReachesTheCommissionThatRepublishesIt(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways})
	if len(masksWithholding(p, "commission", "margin_tier_at_accrual")) == 0 {
		t.Error("commission.margin_tier_at_accrual survived a partner margin mask")
	}
}

func TestTheStrictestMaskDecidesWhenTwoNameOneField(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(
		principal.FieldMask{Object: "deal", Field: "amount_minor",
			Condition: principal.MaskOutsideWriteAuthority},
		principal.FieldMask{Object: "deal", Field: "expected_arr_minor",
			Condition: principal.MaskAlways})
	// expected_arr_minor is named by both: its own always-mask and the amount's
	// conditioned group. Always wins — a field readable on some rows through one
	// mask and no rows through another is readable on none.
	got := masksWithholding(p, "deal", "expected_arr_minor")
	if !slices.ContainsFunc(got, func(m principal.FieldMask) bool {
		return m.Condition == principal.MaskAlways
	}) {
		t.Errorf("masksWithholding = %v; the always-mask must be among them", got)
	}
}

// The closure is the one place three renderings now ask, so a principal that
// reads every row must fall out of it rather than out of each caller.
func TestAReaderOfEveryRowIsWithheldNothing(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways})
	p.Permissions.RowScope = principal.RowScopeAll
	if len(masksWithholding(p, "deal", "amount_minor")) != 0 {
		t.Error("a mask withheld a column from a reader whose scope is every row")
	}
}

// What goes on the wire in masked_fields: a reader can only tell "withheld"
// from "empty" for a field the list NAMES, so every field the group takes has
// to be named, not merely nulled.
func TestMaskedFieldsNamesEveryFieldTheGroupTakes(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways})
	got := MaskedFields(p, "deal", false)
	for _, field := range []string{"amount_minor", "expected_arr_minor", "currency"} {
		if !slices.Contains(got, field) {
			t.Errorf("MaskedFields = %v, want it to name %s", got, field)
		}
	}
}

// A mask configured on one object reaches the fields of the object that
// republishes the fact, and names them under THAT object rather than its own.
func TestMaskedFieldsCrossesToTheObjectTheGroupNames(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways})
	if got := MaskedFields(p, "commission", false); !slices.Contains(got, "margin_tier_at_accrual") {
		t.Errorf("MaskedFields(commission) = %v, want the tier the partner mask reaches", got)
	}
	if got := MaskedFields(p, "partner", false); !slices.Contains(got, "margin_tier") {
		t.Errorf("MaskedFields(partner) = %v, want the configured field itself", got)
	}
}
