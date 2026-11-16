// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannels

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAConnectionStartsOff(t *testing.T) {
	e := setupChannels(t)
	c, err := e.store.Connection(e.as(e.rep, repGrant))
	if err != nil {
		t.Fatalf("reading the connection: %v", err)
	}
	if c.Enabled || c.RepliesWaiting != 0 || c.LastPullAt != nil {
		t.Fatalf("a new connection = %+v, want off with nothing waiting", c)
	}
}

func TestTurningOnIsAuditedOnceAndRepeatsAreNoOps(t *testing.T) {
	e := setupChannels(t)
	for range 2 {
		c, err := e.store.SetEnabled(e.as(e.rep, repGrant), true)
		if err != nil || !c.Enabled {
			t.Fatalf("turning on = %+v, %v", c, err)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND action = 'update'`, connectionEntity); n != 1 {
		t.Fatalf("audit rows = %d, want 1 for one real change", n)
	}
}

func TestOneMembersConnectionIsNotAnothers(t *testing.T) {
	e := setupChannels(t)
	if _, err := e.store.SetEnabled(e.as(e.rep, repGrant), true); err != nil {
		t.Fatal(err)
	}
	c, err := e.store.Connection(e.as(e.other, repGrant))
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled {
		t.Fatal("the other member's connection reads as on because the rep turned theirs on")
	}
}

func TestARoleWithoutThePermissionIsRefused(t *testing.T) {
	e := setupChannels(t)
	_, err := e.store.Connection(e.as(e.rep, principal.ObjectGrant{}))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want permission denied", err)
	}
}

func TestRequireEnabledRefusesAConnectionThatIsOff(t *testing.T) {
	e := setupChannels(t)
	ctx := e.as(e.rep, repGrant)
	if _, err := e.store.Connection(ctx); err != nil {
		t.Fatal(err)
	}
	err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.RequireEnabled(ctx, tx, e.rep)
		return err
	})
	if !errors.Is(err, ErrOff) {
		t.Fatalf("err = %v, want ErrOff", err)
	}
}
