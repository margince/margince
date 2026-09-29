// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// A standard field the reader's role masks answers no filter at all: not the
// comparison, not its negation, and not a why. Any of the three would let a
// reader bisect the withheld value out of which records come back.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	collectionsmod "github.com/margince/margince/backend/internal/modules/collections"
	contactsmod "github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countFor is the engine's count of one leaf, as the given reader.
//
//craft:ignore naked-any value is a predicate leaf's operand, which spans every scalar and array shape the filter DSL accepts
func (f fixture) countFor(ctx context.Context, t *testing.T, field, op string, value any) int {
	t.Helper()
	engine, _, err := f.lists.SegmentEngine(ctx, "contact")
	if err != nil {
		t.Fatal(err)
	}
	pred, err := collectionsmod.PredicateFromDefinition(map[string]any{"field": field, "op": op, "value": value})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		n, err = engine.CountMatching(ctx, tx, pred)
		return err
	}); err != nil {
		t.Fatalf("count %s %s: %v", field, op, err)
	}
	return n
}

func TestAMaskedStandardFieldSelectsNothingEitherWay(t *testing.T) {
	f := setupFixture(t)
	target, err := f.contacts.CreateContact(f.ctx, contactsmod.CreateContactInput{
		FullName: "Masked Mira", Source: "manual",
		Emails: []contactsmod.ContactEmailInput{{Email: "mira@masked.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.contacts.CreateContact(f.ctx, contactsmod.CreateContactInput{FullName: "Open Olaf", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	masked := testPerms
	masked.RowScope = principal.RowScopeTeam
	masked.FieldMasks = []principal.FieldMask{
		{Object: "contact", Field: "email", Condition: principal.MaskAlways},
		{Object: "contact", Field: "name", Condition: principal.MaskAlways},
	}
	reader := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, masked)

	for _, c := range []struct {
		field, op string
		value     any
		unmasked  int
	}{
		{"email", "eq", "mira@masked.example", 1},
		{"email", "neq", "mira@masked.example", 1},
		{"email", "exists", true, 1},
		{"name", "contains", "mira", 1},
		{"name", "neq", "Masked Mira", 1},
	} {
		if got := f.countFor(f.ctx, t, c.field, c.op, c.value); got != c.unmasked {
			t.Fatalf("unmasked %s %s counts %d, want %d — the case proves nothing", c.field, c.op, got, c.unmasked)
		}
		if got := f.countFor(reader, t, c.field, c.op, c.value); got != 0 {
			t.Errorf("a reader masked on %s counts %d for %s %v, want 0", c.field, got, c.op, c.value)
		}
	}

	list := f.liveList(t, "By address", map[string]any{"field": "email", "op": "eq", "value": "MIRA@masked.example"})
	why, err := f.lists.ExplainMember(reader, list, ids.UUID(target.Id))
	if err != nil {
		t.Fatal(err)
	}
	if why.Member || !why.Clauses.Hidden || why.Clauses.Value != nil {
		t.Errorf("a masked reader's why = member %v, %+v; want no member and the address hidden", why.Member, why.Clauses)
	}
	openWhy, err := f.lists.ExplainMember(f.ctx, list, ids.UUID(target.Id))
	if err != nil {
		t.Fatal(err)
	}
	if !openWhy.Member || openWhy.Clauses.Value == nil || *openWhy.Clauses.Value != "mira@masked.example" {
		t.Errorf("an unmasked reader's why = member %v, %+v; want the member and its address", openWhy.Member, openWhy.Clauses)
	}
}
