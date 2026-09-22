// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
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

// Write authority is answerable only where rows carry an owner and can be
// shared. A product has no owner, so "outside the rows you may write" names no
// row at all on one — and an unanswerable condition withholds rather than
// erroring the read, which is the direction that cannot leak.
func TestMaskedColumnSQLWithholdsWhereWriteAuthorityCannotBeAnswered(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"product": {Read: true, Update: true}},
			RowScope: principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{{
				Object: "product", Field: "unit_price_minor", Condition: principal.MaskOutsideWriteAuthority,
			}},
		},
	})
	got, err := auth.MaskedColumnSQL(ctx, "product", "unit_price_minor", "p", "unit_price_minor",
		func(any) int { return 1 })
	if err != nil {
		t.Fatalf("MaskedColumnSQL: %v", err)
	}
	if !strings.HasPrefix(got, "CASE WHEN FALSE THEN") {
		t.Errorf("MaskedColumnSQL = %q, want the column withheld on every row", got)
	}
}
