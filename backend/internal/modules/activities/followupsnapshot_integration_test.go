// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestFollowUpSourcesShareOnlyTheirOwnTransactionsWindow(t *testing.T) {
	e := setupLoad(t)
	e.restoreFollowUpWindow(t)
	cfg := configtest.StoredOver(t, e.pool, ConfigSettings())
	configtest.Bind(t, cfg)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	asOf := clocktest.Now(t)
	err := database.WithWorkspaceSnapshot(e.as(), e.pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
			ctx, meter := database.WithStatementMeter(ctx)
			read := func() error {
				if _, days, err := store.AwaitingReplies(ctx, asOf); err != nil || days != 2 {
					t.Fatalf("awaiting replies window = %d, %v; want 2", days, err)
				}
				if _, err := store.MeetingsAwaitingFollowUp(ctx, asOf); err != nil {
					return err
				}
				_, err := store.NextMeetingFollowUpsByDealTx(ctx, tx, asOf, nil)
				return err
			}
			if err := read(); err != nil {
				return err
			}
			first := meter.Cost()
			if err := read(); err != nil {
				return err
			}
			second := meter.Cost().Minus(first)
			if first.Statements-second.Statements != 1 {
				t.Errorf("first pair used %d statements, repeated pair %d; want one shared setting read", first.Statements, second.Statements)
			}
			if err := config.Save(e.settingsAdmin(true), cfg, FollowUpAfterDays, 5); err != nil {
				return err
			}
			detached := database.Detached(ctx)
			return database.WithWorkspaceSnapshot(detached, e.pool, func(fresh context.Context) error {
				_, days, err := store.AwaitingReplies(fresh, asOf)
				if err == nil && days != 5 {
					t.Errorf("a detached snapshot inherited the old %d-day window, want 5", days)
				}
				return err
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFollowUpMemoFollowsSettingWritesAndSavepointRollback(t *testing.T) {
	e := setupLoad(t)
	e.restoreFollowUpWindow(t)
	settingsStore, cfg := configtest.StoredOverWith(t, e.pool, nil, ConfigSettings())
	configtest.Bind(t, cfg)
	admin := e.settingsAdmin(true)
	err := database.WithWorkspaceSnapshot(admin, e.pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
			check := func(want int) {
				t.Helper()
				if days, err := followUpAfterDays(ctx, tx); err != nil || days != want {
					t.Fatalf("follow-up window = %d, %v; want %d", days, err, want)
				}
			}
			check(2)
			if err := config.SaveTx(ctx, cfg, tx, FollowUpAfterDays, 5); err != nil {
				return err
			}
			check(5)
			if _, err := settingsStore.DeleteRawTx(ctx, tx, FollowUpAfterDays.Key()); err != nil {
				return err
			}
			check(2)
			definitions := cfg.StoredDefinitions()
			if _, err := settings.Seed(ctx, tx, definitions[0], json.RawMessage("7"), nil); err != nil {
				return err
			}
			check(7)
			if err := settings.ResetConfig(ctx, tx, settings.NewRegistry(definitions...)); err != nil {
				return err
			}
			check(2)
			undone := errors.New("undo the setting change")
			write := func(context.Context) error {
				if err := config.SaveTx(ctx, cfg, tx, FollowUpAfterDays, 9); err != nil {
					return err
				}
				check(9)
				return undone
			}
			for _, rollback := range []func() error{
				func() error { return database.WithSavepoint(ctx, write) },
				func() error { return database.WithDegradableRead(ctx, time.Second, write) },
			} {
				if err := rollback(); !errors.Is(err, undone) {
					t.Fatalf("savepoint returned %v, want the lane's error", err)
				}
				check(2)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}
