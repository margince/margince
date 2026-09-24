// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
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
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways,
	})
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
		Object: "deal", Field: "currency", Condition: principal.MaskAlways,
	})
	if len(masksWithholding(p, "deal", "amount_minor")) != 0 {
		t.Error("a mask on the currency alone withheld the amount")
	}
}

// Cross-object: the tier on a commission entry is the same fact, and a mask
// that leaves it standing protects nothing.
func TestAPartnerMarginMaskReachesTheCommissionThatRepublishesIt(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways,
	})
	if len(masksWithholding(p, "commission", "margin_tier_at_accrual")) == 0 {
		t.Error("commission.margin_tier_at_accrual survived a partner margin mask")
	}
}

// The precondition for the strictest deciding: masksWithholding hands back
// EVERY mask reaching the field, never the first one it matched, because a
// caller given one mask cannot find the stricter one behind it. Which mask wins
// is settled below, where the resolution actually happens.
func TestMasksWithholdingReturnsEveryMaskReachingTheField(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(
		principal.FieldMask{
			Object: "deal", Field: "amount_minor",
			Condition: principal.MaskOutsideWriteAuthority,
		},
		principal.FieldMask{
			Object: "deal", Field: "expected_arr_minor",
			Condition: principal.MaskAlways,
		})
	// expected_arr_minor is named by both: its own always-mask and the amount's
	// conditioned group.
	got := masksWithholding(p, "deal", "expected_arr_minor")
	if !slices.ContainsFunc(got, func(m principal.FieldMask) bool {
		return m.Condition == principal.MaskAlways
	}) {
		t.Errorf("masksWithholding = %v; the always-mask must be among them", got)
	}
}

// A field readable on some rows through one mask and on no rows through another
// is readable on none — and BOTH renderings have to say so, or the statement
// withholds the figure while the record hands it over.
//
// Both orderings of the same pair, because each rendering returns early on the
// always-mask: an answer that depended on which mask came first would be the
// order of stored rows deciding what a role reads.
func TestTheStrictestMaskDecidesWhenTwoNameOneField(t *testing.T) {
	t.Parallel()
	conditioned := principal.FieldMask{
		Object: "deal", Field: "amount_minor",
		Condition: principal.MaskOutsideWriteAuthority,
	}
	always := principal.FieldMask{
		Object: "deal", Field: "expected_arr_minor",
		Condition: principal.MaskAlways,
	}
	for _, order := range [][]principal.FieldMask{{conditioned, always}, {always, conditioned}} {
		p := principalWithMasks(order...)
		// The update verb, so the conditioned mask ALONE would render a real
		// predicate: without it the update-verb arm answers "no row" already and
		// the always-mask could be ignored with nothing failing.
		p.Permissions.Objects = map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}}

		// The wire rendering, asked about a row the caller COULD change: the
		// conditioned mask lifts there, and the always-mask's group still takes
		// the amount with the ARR.
		if got := MaskedFields(p, "deal", true); !slices.Contains(got, "amount_minor") {
			t.Errorf("MaskedFields = %v under %v, want the amount withheld on a writable row too",
				got, order)
		}
		// The statement rendering, asked about every row at once.
		ctx := principal.WithActor(context.Background(), p)
		clause, masked, err := MaskExcludedClause(ctx, "deal", "amount_minor", "d", func(any) int { return 1 })
		if err != nil {
			t.Fatalf("MaskExcludedClause: %v", err)
		}
		if !masked || clause != sqlNoRow {
			t.Errorf("MaskExcludedClause = %q (masked %v) under %v, want no row admitted",
				clause, masked, order)
		}
	}
}

// The twin of the arm MaskExcludedClause carries: write authority is answerable
// only where rows carry an owner and a grant, so a mask conditioned on it
// elsewhere withholds on every row — and the two renderings must agree, or the
// SQL withholds the price while the record ships it.
func TestAConditionedMaskWithholdsWhereWriteAuthorityCannotBeAnswered(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "product", Field: "unit_price_minor", Condition: principal.MaskOutsideWriteAuthority,
	})
	p.Permissions.Objects = map[string]principal.ObjectGrant{"product": {Read: true, Update: true}}
	for _, writable := range []bool{false, true} {
		if got := MaskedFields(p, "product", writable); !slices.Contains(got, "unit_price_minor") {
			t.Errorf("MaskedFields(product, writable=%v) = %v, want the price withheld: a product "+
				"carries no owner, so the condition names no row it could lift on", writable, got)
		}
	}
	ctx := principal.WithActor(context.Background(), p)
	clause, masked, err := MaskExcludedClause(ctx, "product", "unit_price_minor", "p", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskExcludedClause: %v", err)
	}
	if !masked || clause != sqlNoRow {
		t.Errorf("MaskExcludedClause = %q (masked %v), want no row admitted", clause, masked)
	}
}

// The closure is the one place three renderings now ask, so a principal that
// reads every row must fall out of it rather than out of each caller.
func TestAReaderOfEveryRowIsWithheldNothing(t *testing.T) {
	t.Parallel()
	p := principalWithMasks(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways,
	})
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
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways,
	})
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
		Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways,
	})
	if got := MaskedFields(p, "commission", false); !slices.Contains(got, "margin_tier_at_accrual") {
		t.Errorf("MaskedFields(commission) = %v, want the tier the partner mask reaches", got)
	}
	if got := MaskedFields(p, "partner", false); !slices.Contains(got, "margin_tier") {
		t.Errorf("MaskedFields(partner) = %v, want the configured field itself", got)
	}
}
