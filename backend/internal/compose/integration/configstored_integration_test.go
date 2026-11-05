// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A stored setting over the real settings store. A boot source answers first
// and locks the row; without one, the row and then the default decide.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	lockCapKey = "test.config_lock_cap"
	lockCapEnv = "X_TEST_CONFIG_LOCK_CAP"
)

var lockCap = config.DefineStored(config.Spec[int]{
	Key: lockCapKey, Env: lockCapEnv, Type: config.Int,
	Default: config.Some(100), Min: config.Some(1), Doc: "d",
},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update"})

// storedFixture binds a principal holding the setting's object and a real store
// whose registry is the one the config derives.
func storedFixture(t *testing.T, environ ...string) (context.Context, *Env, *config.Config) {
	t.Helper()
	e := Setup(t)
	reg, err := config.NewRegistry([]config.Any{lockCap})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Args:    []string{"--config", filepath.Join(t.TempDir(), "margince.yaml")},
		Environ: environ,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := settings.New(e.Pool, settings.NewRegistry(cfg.StoredDefinitions()...))
	t.Cleanup(func() { e.WsExec(t, `DELETE FROM setting WHERE key = $1`, lockCapKey) })
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	return ctx, e, cfg.WithDatabase(store)
}

// unlocked is the same registry with no boot source. Through it the test
// writes and reads the row that a locked config never touches.
func unlocked(t *testing.T, locked *config.Config, e *Env) *config.Config {
	t.Helper()
	cfg, err := config.Load(locked.Registry(), config.Sources{
		Args: []string{"--config", filepath.Join(t.TempDir(), "margince.yaml")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg.WithDatabase(settings.New(e.Pool, settings.NewRegistry(cfg.StoredDefinitions()...)))
}

func entryFor(ctx context.Context, t *testing.T, cfg *config.Config) config.Entry {
	t.Helper()
	entries, err := cfg.StoredEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range entries {
		if en.Key == lockCapKey && en.Value != nil {
			return en
		}
	}
	t.Fatalf("StoredEntries has no value for %s: %+v", lockCapKey, entries)
	return config.Entry{}
}

func TestConfigStored_ABootSourceWinsOverTheRowAndLocksIt(t *testing.T) {
	ctx, e, cfg := storedFixture(t, lockCapEnv+"=900")
	open := unlocked(t, cfg, e)
	if err := config.Save(ctx, open, lockCap, 500); err != nil {
		t.Fatalf("planting the row: %v", err)
	}

	v, origin, err := config.StoredValWithSource(ctx, cfg, lockCap)
	if err != nil || v != 900 || origin.Source != config.SourceEnv {
		t.Fatalf("got %d %+v %v", v, origin, err)
	}
	err = config.Save(ctx, cfg, lockCap, 901)
	if !errors.Is(err, apperrors.ErrConflict) || err.Error() != lockCapKey+" is set by env "+lockCapEnv {
		t.Fatalf("save of a different value: %v", err)
	}
	if err := config.Save(ctx, cfg, lockCap, 900); err != nil {
		t.Fatalf("save of the boot value is a no-op: %v", err)
	}
	if v, o, err := config.StoredValWithSource(ctx, open, lockCap); err != nil || v != 500 || o.Source != config.SourceDatabase {
		t.Fatalf("the row must stay untouched while locked, got %d %+v %v", v, o, err)
	}
	if en := entryFor(ctx, t, cfg); *en.Value != "900" || en.Origin.Source != config.SourceEnv {
		t.Fatalf("the overview of a locked setting shows %s from %+v", *en.Value, en.Origin)
	}
}

func TestConfigStored_WithoutABootSourceTheRowThenTheDefaultDecide(t *testing.T) {
	ctx, e, cfg := storedFixture(t)
	if v, o, err := config.StoredValWithSource(ctx, cfg, lockCap); err != nil || v != 100 || o.Source != config.SourceDefault {
		t.Fatalf("default: %d %+v %v", v, o, err)
	}
	if err := config.Save(ctx, cfg, lockCap, 250); err != nil {
		t.Fatal(err)
	}
	if v, o, err := config.StoredValWithSource(ctx, cfg, lockCap); err != nil || v != 250 || o.Source != config.SourceDatabase {
		t.Fatalf("row: %d %+v %v", v, o, err)
	}
	if en := entryFor(ctx, t, cfg); *en.Value != "250" || en.Origin.Source != config.SourceDatabase {
		t.Fatalf("the overview shows %s from %+v", *en.Value, en.Origin)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'installation_settings' AND after ? $1`, lockCapKey); n != 1 {
		t.Fatalf("the save wrote %d audit rows, want 1", n)
	}
}

func TestConfigStored_TheGrantDecidesBeforeTheLock(t *testing.T) {
	for _, environ := range [][]string{nil, {lockCapEnv + "=900"}} {
		_, e, cfg := storedFixture(t, environ...)
		none := e.As(e.AdminUser, nil, principal.Permissions{RowScope: principal.RowScopeAll})
		if _, err := cfg.StoredEntries(none); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("StoredEntries without the object grant (%v) = %v, want ErrPermissionDenied", environ, err)
		}
		if _, err := config.StoredVal(none, cfg, lockCap); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("StoredVal without the object grant (%v) = %v, want ErrPermissionDenied", environ, err)
		}
		if err := config.Save(none, cfg, lockCap, 901); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("Save without the object grant (%v) = %v, want ErrPermissionDenied", environ, err)
		}
	}
}

// Seed skips a boot-locked key, and a data reset still removes its row: first
// boot never seeds one, so the reset matches first boot.
func TestConfigStored_SeedSkipsABootLockedKeyAndResetClearsIt(t *testing.T) {
	ctx, e, cfg := storedFixture(t, lockCapEnv+"=900")
	def := cfg.StoredDefinitions()[0]
	inTx := func(fn func(tx pgx.Tx) error) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, e.Pool, fn); err != nil {
			t.Fatal(err)
		}
	}
	inTx(func(tx pgx.Tx) error {
		stored, err := settings.Seed(ctx, tx, def, json.RawMessage(`7`), cfg.BootLocked())
		if stored {
			t.Error("seed wrote a row under a boot-locked key")
		}
		return err
	})
	if n := e.WsCount(t, `SELECT count(*) FROM setting WHERE key = $1`, lockCapKey); n != 0 {
		t.Fatalf("a skipped seed left %d rows", n)
	}

	if err := config.Save(ctx, unlocked(t, cfg, e), lockCap, 500); err != nil {
		t.Fatal(err)
	}
	inTx(func(tx pgx.Tx) error { return settings.ResetConfig(ctx, tx, settings.NewRegistry(def)) })
	if n := e.WsCount(t, `SELECT count(*) FROM setting WHERE key = $1`, lockCapKey); n != 0 {
		t.Fatalf("a reset kept the boot-locked key's row")
	}
}
