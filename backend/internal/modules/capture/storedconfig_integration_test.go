// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureSettingsStore is the capture-settings store a program composes after
// it loads environ, read and saved through pool.
func captureSettingsStore(t *testing.T, pool *pgxpool.Pool, environ ...string) (*capture.SettingsStore, *config.Config) {
	t.Helper()
	reg, err := config.NewRegistry(capture.ConfigSettings())
	if err != nil {
		t.Fatalf("capture's settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading capture's settings from %v: %v", environ, err)
	}
	defs := append(capture.Definitions(), cfg.StoredDefinitions()...)
	store := settings.New(pool, settings.NewRegistry(defs...))
	cfg = cfg.WithDatabase(store)
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
	before := config.Bound()
	config.Bind(cfg)
	t.Cleanup(func() { config.Bind(before) })

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
