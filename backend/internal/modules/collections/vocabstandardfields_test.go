// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// compileLeaf renders one leaf through the engine a caller resolves, and
// answers the SQL and how many values it bound.
//
//craft:ignore naked-any value is a predicate leaf's operand, which spans every scalar shape the filter DSL accepts
func compileLeaf(t *testing.T, ctx context.Context, store *Store, resource, field, op string, value any) (string, int) {
	t.Helper()
	engine, ok, err := store.SegmentEngine(ctx, resource)
	if err != nil || !ok {
		t.Fatalf("segment engine for %s: ok=%v err=%v", resource, ok, err)
	}
	var args []any
	sql, err := storekit.CompilePredicate(storekit.Predicate{Field: field, Op: op, Value: value}, engine.Fields,
		func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		t.Fatalf("compile %s.%s %s: %v", resource, field, op, err)
	}
	return sql, len(args)
}

// employerID is any well-formed company id; the leaf compiles the same for all.
const employerID = "0190a9b8-0000-7000-8000-000000000001"

func maskedCtx(masks ...principal.FieldMask) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}, "contact": {Read: true}},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: masks,
		},
	})
}

func TestAMaskOnTheDealsMoneyWithholdsTheAmountLeaf(t *testing.T) {
	store := (&Store{}).WithDealAmount("t.amount_minor")
	if sql, _ := compileLeaf(t, maskedCtx(), store, "deal", amountField, storekit.OpGt, 100.0); sql == "FALSE" {
		t.Fatal("an unmasked reader's amount leaf compiled to FALSE, so the masked case below proves nothing")
	}
	masked := maskedCtx(principal.FieldMask{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways})
	for _, op := range []string{storekit.OpGt, storekit.OpLte, storekit.OpNeq} {
		if sql, bound := compileLeaf(t, masked, store, "deal", amountField, op, 100.0); sql != "FALSE" || bound != 0 {
			t.Errorf("amount %s compiled to %q with %d values for a reader masked on amount_minor, want FALSE", op, sql, bound)
		}
	}
}

func TestAMaskOnAContactFieldWithholdsTheLeafOfTheSameName(t *testing.T) {
	masked := maskedCtx(principal.FieldMask{Object: "contact", Field: emailField, Condition: principal.MaskAlways})
	if sql, _ := compileLeaf(t, masked, &Store{}, "contact", emailField, storekit.OpNeq, "a@b.example"); sql != "FALSE" {
		t.Errorf("email neq compiled to %q for a reader masked on email, want FALSE", sql)
	}
	if sql, _ := compileLeaf(t, masked, &Store{}, "contact", titleField, storekit.OpEq, "CTO"); sql == "FALSE" {
		t.Error("a mask on email withheld the title leaf beside it")
	}
}

func TestADealAmountIsUnpricedUntilComposeInjectsTheRule(t *testing.T) {
	if sql, _ := compileLeaf(t, maskedCtx(), &Store{}, "deal", amountField, storekit.OpGt, 1.0); sql != "FALSE" {
		t.Errorf("an unwired store compiled the amount to %q, want FALSE", sql)
	}
	sql, bound := compileLeaf(t, maskedCtx(), (&Store{}).WithDealAmount("t.worth"), "deal", amountField, storekit.OpGt, 1.0)
	if sql != "t.worth > $1" || bound != 1 {
		t.Errorf("the injected amount compiled to %q with %d values, want the injected expression compared", sql, bound)
	}
}

func TestTheEmployerLeafTakesTheEdgeGate(t *testing.T) {
	without := maskedCtx()
	if sql, _ := compileLeaf(t, without, &Store{}, "contact", companyIDField, storekit.OpEq, employerID); sql != "FALSE" {
		t.Errorf("a reader refused relationship:read got %q, want the employer leaf withheld", sql)
	}
	with := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"contact": {Read: true}, "company": {Read: true}, "relationship": {Read: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
	sql, _ := compileLeaf(t, with, &Store{}, "contact", companyIDField, storekit.OpEq, employerID)
	if !strings.Contains(sql, "FROM relationship rel") || !strings.Contains(sql, "FROM company ep") {
		t.Errorf("the employer leaf compiled to %q, want the edge bounded by its endpoints' scope", sql)
	}
}

func TestAnEmailOperandIsFoldedToTheStoredCase(t *testing.T) {
	engine, _, err := (&Store{}).SegmentEngine(readerCtx(), "lead")
	if err != nil {
		t.Fatal(err)
	}
	var args []any
	if _, err := storekit.CompilePredicate(storekit.Predicate{Field: emailField, Op: storekit.OpIn, Value: []any{"Ann@X.Example"}},
		engine.Fields, func(v any) int { args = append(args, v); return len(args) }); err != nil {
		t.Fatal(err)
	}
	if got, ok := args[0].([]string); !ok || len(got) != 1 || got[0] != "ann@x.example" {
		t.Errorf("bound %#v, want the address lowercased as lead_email_norm stores it", args)
	}
}
