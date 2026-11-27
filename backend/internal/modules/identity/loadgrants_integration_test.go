// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// authorMasks writes field masks on a role, and removes them when the test ends.
func (e *revocationEnv) authorMasks(t *testing.T, key string, masks ...principal.FieldMask) {
	t.Helper()
	for _, m := range masks {
		if _, err := e.owner.Exec(context.Background(),
			`INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, $2, $3, $4)`,
			key, m.Object, m.Field, m.Condition); err != nil {
			t.Fatalf("masking %s on %s: %v", m.Field, key, err)
		}
	}
}

// A seat resolved with others in one call gets the union of its roles' masks,
// distinct and ordered. That is what loadFieldMasks answers for those roles.
func TestASeatWithTwoRolesHoldsTheUnionOfTheirMasks(t *testing.T) {
	t.Parallel()
	e := setupRevocationEnv(t, "mask-union")
	first := e.customRole(t, "Masked first", "rep", nil)
	second := e.customRole(t, "Masked second", "rep", nil)
	amount := principal.FieldMask{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}
	currency := principal.FieldMask{Object: "deal", Field: "currency", Condition: principal.MaskAlways}
	tier := principal.FieldMask{Object: "partner", Field: "margin_tier", Condition: principal.MaskOutsideWriteAuthority}
	e.authorMasks(t, first, amount, currency)
	e.authorMasks(t, second, amount, tier)
	// An archived role grants nothing, so its masks must not count either.
	archived := e.customRole(t, "Masked archived", "rep", nil)
	e.authorMasks(t, archived, principal.FieldMask{Object: "deal", Field: "project_id", Condition: principal.MaskAlways})
	if _, err := e.svc.ArchiveRole(e.wsCtx(e.admin), e.admin, archived); err != nil {
		t.Fatal(err)
	}

	both := e.seat(t, "both", first).UserID
	for _, extra := range []string{second, archived} {
		if _, err := e.owner.Exec(context.Background(),
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`, e.roleID(t, extra), both); err != nil {
			t.Fatal(err)
		}
	}
	onlySecond := e.seat(t, "only-second", second).UserID

	var got map[ids.UUID]seatGrants
	var wantBoth, wantSecond []principal.FieldMask
	err := e.svc.db.Tx(e.wsCtx(e.admin), func(tx pgx.Tx) error {
		var err error
		if got, err = loadGrantsFor(context.Background(), tx, e.svc.rbac, []ids.UUID{both.UUID, onlySecond.UUID, e.member.UserID.UUID}); err != nil {
			return err
		}
		if wantBoth, err = loadFieldMasks(context.Background(), tx, []string{first, second}); err != nil {
			return err
		}
		wantSecond, err = loadFieldMasks(context.Background(), tx, []string{second})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if union := []principal.FieldMask{amount, currency, tier}; !slices.Equal(wantBoth, union) {
		t.Fatalf("loadFieldMasks for both roles = %+v, want %+v", wantBoth, union)
	}
	for _, c := range []struct {
		seat ids.UserID
		want []principal.FieldMask
	}{{both, wantBoth}, {onlySecond, wantSecond}, {e.member.UserID, nil}} {
		if masks := got[c.seat.UUID].perms.FieldMasks; !slices.Equal(masks, c.want) {
			t.Errorf("seat %s holds masks %+v, want %+v", c.seat, masks, c.want)
		}
	}
}
