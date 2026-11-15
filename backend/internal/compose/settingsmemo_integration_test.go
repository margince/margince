// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A transaction reads a setting's row once, however many readers ask, and a
// write through the settings store makes the next read see the new value.
func TestASettingIsReadOncePerTransactionAndAgainAfterItsWriter(t *testing.T) {
	e := integration.Setup(t)
	store := settings.New(e.Pool, settingsRegistry())
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	ctx, meter := database.WithStatementMeter(ctx)
	zone := func(ctx context.Context, tx pgx.Tx) string {
		t.Helper()
		name, err := identity.TimezoneAppliedTx(ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
		return name
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		before := meter.Cost()
		first, second := zone(ctx, tx), zone(ctx, tx)
		if read := meter.Cost().Minus(before).Statements; read != 1 || first != second {
			t.Errorf("two readers sent %d statements and read %q and %q, want one statement and one zone", read, first, second)
		}
		if err := settings.SetTx(ctx, store, tx, identity.Timezone, "Pacific/Auckland"); err != nil {
			return err
		}
		if after := zone(ctx, tx); after != "Pacific/Auckland" {
			t.Errorf("after the write the zone reads %q, want Pacific/Auckland", after)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A writer reads the row again after its lock. A value its transaction
// remembered from before another writer's commit is not what it writes over.
func TestASettingWriteIsNotFooledByWhatItsTransactionReadBeforeTheLock(t *testing.T) {
	e := integration.Setup(t)
	store := settings.New(e.Pool, settingsRegistry())
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	write := func(name string) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return settings.SetTx(ctx, store, tx, identity.Timezone, name)
		}); err != nil {
			t.Fatal(err)
		}
	}
	read := func() string {
		t.Helper()
		var name string
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			name, err = identity.TimezoneAppliedTx(ctx, tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return name
	}
	write("Europe/Lisbon")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		seen, err := identity.TimezoneAppliedTx(ctx, tx)
		if err != nil || seen != "Europe/Lisbon" {
			t.Fatalf("the transaction first read %q (%v), want Europe/Lisbon", seen, err)
		}
		write("Pacific/Auckland")
		return settings.SetTx(ctx, store, tx, identity.Timezone, "Europe/Lisbon")
	}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "Europe/Lisbon" {
		t.Errorf("after the write the zone is %q, want Europe/Lisbon", got)
	}
}
