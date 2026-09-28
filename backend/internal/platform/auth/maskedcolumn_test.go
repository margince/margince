// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RowScopeTeam and not All, deliberately: auth.Unbounded reads row_scope=all as
// "every row" and skips masks outright, so a fixture on All would assert
// nothing about masking. A masked seat is a row-scoped seat by construction.
func maskedActor(masks ...principal.FieldMask) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects:    map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: masks,
		},
	})
}

// No mask: the column goes out as itself, so an unmasked installation pays
// nothing for the guard being there.
func TestMaskedColumnSQLIsTheBareColumnWithoutAMask(t *testing.T) {
	t.Parallel()
	got, err := auth.MaskedColumnSQL(maskedActor(), "deal", "amount_minor", "d", "amount_minor", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedColumnSQL: %v", err)
	}
	if got != "d.amount_minor" {
		t.Errorf("MaskedColumnSQL = %q, want the bare column", got)
	}
}

// An always-mask nulls the column on every row, and the expression still
// carries a type — a bare NULL is what Postgres refuses to infer one for in a
// UNION leg or an aggregate, which is where these renderings land.
func TestMaskedColumnSQLNullsEveryRowUnderAnAlwaysMask(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways,
	})
	got, err := auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedColumnSQL: %v", err)
	}
	if !strings.HasPrefix(got, "CASE WHEN FALSE THEN d.amount_minor") {
		t.Errorf("MaskedColumnSQL = %q, want a CASE that can never reach the column and still types from it", got)
	}
	if !strings.Contains(got, "ELSE NULL END") {
		t.Errorf("MaskedColumnSQL = %q, want the masked arm to be NULL", got)
	}
}

// A write-authority mask nulls it only where the caller could not change the
// row, which is the same decision the deal list makes per row.
func TestMaskedColumnSQLNullsOnlyTheUnwritableRowsUnderAConditionedMask(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority,
	})
	got, err := auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedColumnSQL: %v", err)
	}
	if strings.HasPrefix(got, "CASE WHEN FALSE") {
		t.Errorf("MaskedColumnSQL = %q, want the write-authority predicate rather than a blanket refusal", got)
	}
	if !strings.HasPrefix(got, "CASE WHEN ") || !strings.HasSuffix(got, "ELSE NULL END") {
		t.Errorf("MaskedColumnSQL = %q, want a CASE nulling the column outside write authority", got)
	}
}

// Losing the object's update verb must NARROW, never widen: a caller with no
// write authority anywhere holds it on no row, so a conditioned mask withholds
// the column everywhere. This is the shape of the review finding #1917 fixed at
// the primitive, restated here for the rendering that reads it.
func TestMaskedColumnSQLWithholdsEverywhereWhenTheUpdateVerbIsGone(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope: principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{{
				Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority,
			}},
		},
	})
	got, err := auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedColumnSQL: %v", err)
	}
	if !strings.HasPrefix(got, "CASE WHEN FALSE THEN") {
		t.Errorf("MaskedColumnSQL = %q — a role that lost deal.update must read LESS, not more", got)
	}
}

// The field a mask names and the column a caller renders are not always the
// same: a deal's money is stored twice, and only amount_minor is a mask's
// subject. The base column has to inherit that mask, or summing in base
// currency becomes the way around it.
func TestMaskedColumnSQLMasksTheBaseColumnUnderTheFieldsMask(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways,
	})
	got, err := auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor_base",
		func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedColumnSQL: %v", err)
	}
	if !strings.Contains(got, "d.amount_minor_base") {
		t.Errorf("MaskedColumnSQL = %q, want the BASE column rendered", got)
	}
	if !strings.HasPrefix(got, "CASE WHEN FALSE") {
		t.Errorf("MaskedColumnSQL = %q — the base column must inherit the field's mask, "+
			"or a sum in base currency reads what the row withholds", got)
	}
}

// The expression form guards a value the caller rendered, and the ALIAS still
// reaches the predicate: write authority is a question about the row, so a
// projection that does arithmetic on the money must still be judged by whose
// deal it is. The rendering is otherwise the column form's, which is the point
// of their sharing a body.
func TestMaskedExpressionSQLGuardsARenderedValueByItsRow(t *testing.T) {
	t.Parallel()
	const fold = "CASE WHEN d.currency = 'EUR' THEN d.amount_minor ELSE d.amount_minor_base END"
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority,
	})
	got, err := auth.MaskedExpressionSQL(ctx, "deal", "amount_minor", "d", fold, func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedExpressionSQL: %v", err)
	}
	if !strings.Contains(got, fold) {
		t.Errorf("MaskedExpressionSQL = %q, want the caller's expression inside the guard", got)
	}
	if !strings.HasPrefix(got, "CASE WHEN ") || !strings.HasSuffix(got, "ELSE NULL END") {
		t.Errorf("MaskedExpressionSQL = %q, want a CASE nulling the value outside write authority", got)
	}
	if !strings.Contains(got, "d.") {
		t.Errorf("MaskedExpressionSQL = %q, want the predicate to name the aliased row", got)
	}
}

// No mask: the expression goes out untouched, unqualified and unwrapped — the
// caller already rendered every name in it, and there is nothing here to add.
func TestMaskedExpressionSQLIsTheBareExpressionWithoutAMask(t *testing.T) {
	t.Parallel()
	const fold = "sum(d.amount_minor_base)"
	got, err := auth.MaskedExpressionSQL(maskedActor(), "deal", "amount_minor", "d", fold, func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedExpressionSQL: %v", err)
	}
	if got != fold {
		t.Errorf("MaskedExpressionSQL = %q, want the expression unchanged", got)
	}
}

// MaskedFields answers what the mask WITHHOLDS, not how it is configured.
//
// Four surfaces ask this question and then decide what to null or leave out —
// the record read, a filtered export, its preview, and a field's history. Each
// used to expand the configured name for itself, and only the record read
// expanded it at all, so a mask on the amount left the ARR and the currency
// exportable, previewable and readable through an audit diff. Expanding here,
// where they all ask, is what makes the four agree by construction.
func TestMaskedFieldsAnswersWithTheWholeGroupAMaskWithholds(t *testing.T) {
	t.Parallel()

	ctx := maskedActor(principal.FieldMask{Object: fieldmask.Deal, Field: "amount_minor"})
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the fixture built no principal")
	}

	got := auth.MaskedFields(p, fieldmask.Deal, false)
	for _, field := range []string{"amount_minor", "expected_arr_minor", "currency"} {
		if !slices.Contains(got, field) {
			t.Errorf("a mask on the amount answers %v, missing %q — a reader withholding exactly "+
				"what it is told sends the value", got, field)
		}
	}
	// And the question a sort, a filter or a search predicate asks.
	masked, err := auth.MasksAnyRowOf(ctx, fieldmask.Deal, "expected_arr_minor")
	if err != nil {
		t.Fatal(err)
	}
	if !masked {
		t.Error("ordering or filtering by the ARR is allowed under a mask on the amount it " +
			"is withheld with, and the order alone discloses it")
	}
}

// An object this build groups nothing for still withholds what it is told to.
//
// The grouping table is not an allowlist: read as one it would answer "nothing
// is withheld" for every object absent from it, which is every object but the
// deal — a mask that withholds nothing, and indistinguishable from no mask.
func TestAMaskOnAnUngroupedObjectStillWithholdsItsField(t *testing.T) {
	t.Parallel()

	ctx := maskedActor(principal.FieldMask{Object: "company", Field: "legal_name"})
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the fixture built no principal")
	}
	if got := auth.MaskedFields(p, "company", false); !slices.Equal(got, []string{"legal_name"}) {
		t.Errorf("a mask on an ungrouped object answered %v, want the field it names", got)
	}
}

// The SQL arms match the group too, not the spelling.
//
// MaskedColumnSQL, MaskedExpressionSQL and MaskExcludedClause all ask
// MaskExcludedClause which masks reach one column, and every caller in the tree
// asks about the amount. Matched by name, a mask configured on the ARR beside
// it left an aggregate summing figures the reader may not read and a column
// rendering them — the group closed at the wire and stayed open in SQL.
func TestTheSQLArmsMatchTheGroupAMaskWithholds(t *testing.T) {
	t.Parallel()

	ctx := maskedActor(principal.FieldMask{Object: fieldmask.Deal, Field: "expected_arr_minor"})
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	_, masked, err := auth.MaskExcludedClause(ctx, fieldmask.Deal, "amount_minor", "d", arg)
	if err != nil {
		t.Fatal(err)
	}
	if !masked {
		t.Error("a mask on the ARR left the amount unexcluded — an aggregate over it is taken " +
			"over figures this reader may not read")
	}
}

// Who reads every column, and when a conditioned mask lifts.
//
// Both arms are easy to assert backwards. An unbounded principal reads
// everything BY DESIGN, so a fixture that leaves row_scope at `all` and hangs
// masks off it asserts nothing and passes — which is how a masking test comes
// out green while covering the opposite of what it claims. The conditioned arm
// is the mirror: it lifts exactly where the caller could write, and a test
// asking only the unwritable side never sees it lift at all.
func TestWhoReadsEveryColumnAndWhenAConditionedMaskLifts(t *testing.T) {
	t.Parallel()

	masks := []principal.FieldMask{{
		Object: fieldmask.Deal, Field: "amount_minor",
		Condition: principal.MaskOutsideWriteAuthority,
	}}

	bounded, ok := principal.Actor(maskedActor(masks...))
	if !ok {
		t.Fatal("the fixture built no principal")
	}
	if got := auth.MaskedFields(bounded, fieldmask.Deal, false); len(got) == 0 {
		t.Error("a row the caller cannot write reported no mask — the condition withholds there")
	}
	if got := auth.MaskedFields(bounded, fieldmask.Deal, true); len(got) != 0 {
		t.Errorf("a row the caller COULD write still reported %v — the condition is what lifts it, "+
			"and a mask that never lifts is an always-mask wearing the other name", got)
	}

	unbounded := bounded
	unbounded.Permissions.RowScope = principal.RowScopeAll
	if got := auth.MaskedFields(unbounded, fieldmask.Deal, false); len(got) != 0 {
		t.Errorf("a principal reading every row reported %v; reading every row is reading every "+
			"column, and every masking fixture built on one asserts nothing", got)
	}
}

// With nobody bound to the context, the mask question ERRORS.
//
// Every caller of this uses the answer to decide what to withhold, so the one
// reply it must never give unasked is "no". Returning false with no actor would
// read as "nothing is masked" at each of them, and a sort, a filter or a search
// predicate would compile over a column nobody established the caller may read.
func TestTheMaskQuestionRefusesWhenNobodyIsAsking(t *testing.T) {
	t.Parallel()

	if _, err := auth.MasksAnyRowOf(context.Background(), fieldmask.Deal, "amount_minor"); err == nil {
		t.Error("the mask question answered a context with no actor — every caller reads that " +
			"answer as \"nothing is withheld\"")
	}
}
