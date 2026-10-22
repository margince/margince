// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureSettingsStore is the capture-settings store a program composes after
// it loads environ, read and saved through pool.
func captureSettingsStore(t *testing.T, pool *pgxpool.Pool, environ ...string) (*capture.SettingsStore, *config.Config) {
	t.Helper()
	store, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings(), environ...)
	return capture.NewSettings(store, cfg), cfg
}

// captureAdmin may read and change the capture settings of ws.
func captureAdmin(ctx context.Context, ws ids.WorkspaceID) context.Context {
	admin := principal.WithWorkspaceID(ctx, ws.UUID)
	admin = principal.WithCorrelationID(admin, ids.NewV7())
	return principal.WithActor(admin, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A sync interval the environment sets paces the sync, which reads it through
// the process binding, and the settings store refuses any other value. Not
// parallel: the binding is the process's own.
func TestABootSetSyncIntervalPacesTheSyncAndRefusesAChange(t *testing.T) {
	ctx, reg, _, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	store, cfg := captureSettingsStore(t, pool, "MARGINCE_CAPTURE_MAIL_SYNC_INTERVAL_SECONDS=1800")
	configtest.Bind(t, cfg)

	other := 600
	if _, err := store.Update(captureAdmin(ctx, ws), capture.SettingsPatch{MailSyncIntervalSeconds: &other}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a change of the boot-set interval gave %v, want a conflict", err)
	}
	same, on := 1800, true
	got, err := store.Update(captureAdmin(ctx, ws), capture.SettingsPatch{MailSyncIntervalSeconds: &same, AutoEnrich: &on})
	if err != nil {
		t.Fatalf("resending the boot-set interval beside another field: %v", err)
	}
	if got.MailSyncIntervalSeconds != 1800 {
		t.Fatalf("the store reads interval %d, want the 1800 the environment sets", got.MailSyncIntervalSeconds)
	}

	if err := syncOnceOf(ctx, t, reg, "gmail"); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	pacedWithin(ctx, t, "gmail", 25*time.Minute, 30*time.Minute)
}

// A pass cap the environment sets is the cap the pass reads, and the settings
// store refuses a different one. With no boot source the pass reads what an
// admin saved. Not parallel: the binding is the process's own.
func TestTheConfidentialityPassReadsTheBootSetCapBeforeASavedOne(t *testing.T) {
	ctx, _, _, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	threads := capture.NewThreadVerdictStore(database.BindTo(pool, ws))

	store, cfg := captureSettingsStore(t, pool)
	configtest.Bind(t, cfg)
	saved := 90
	if _, err := store.Update(captureAdmin(ctx, ws), capture.SettingsPatch{ConfidentialityPassCap: &saved}); err != nil {
		t.Fatalf("saving the cap: %v", err)
	}
	if got, err := threads.PassCap(ctx); err != nil || got != saved {
		t.Fatalf("the pass reads %d (err %v), want the %d an admin saved", got, err, saved)
	}

	bootStore, bootCfg := captureSettingsStore(t, pool, "MARGINCE_CAPTURE_CONFIDENTIALITY_PASS_CAP=7")
	configtest.Bind(t, bootCfg)
	if got, err := threads.PassCap(ctx); err != nil || got != 7 {
		t.Fatalf("the pass reads %d (err %v), want the 7 the environment sets", got, err)
	}
	other := 8
	if _, err := bootStore.Update(captureAdmin(ctx, ws), capture.SettingsPatch{ConfidentialityPassCap: &other}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a change of the boot-set cap gave %v, want a conflict", err)
	}
}
