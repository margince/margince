// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The database-backed half is in internal/compose/integration; these cases
// need none, because a boot source answers before the database is asked.

var storedCap = config.DefineStored(config.Spec[int]{Key: "test.cap", Env: "X_TEST_CAP", Type: config.Int,
	Default: config.Some(100), Min: config.Some(1), Doc: "d"},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update"})

func TestABootSourceAnswersAStoredSettingWithoutTheDatabase(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"X_TEST_CAP=900"}}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	v, origin, err := config.StoredValWithSource(context.Background(), cfg, storedCap)
	if err != nil || v != 900 || origin.Source != config.SourceEnv || origin.Name != "X_TEST_CAP" {
		t.Fatalf("got %d %+v %v", v, origin, err)
	}
	if o, locked := cfg.Locked("test.cap"); !locked || o.Name != "X_TEST_CAP" {
		t.Fatalf("Locked = %+v %v", o, locked)
	}
}

func TestSavingOverABootSourceIsAConflictThatNamesTheSource(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"X_TEST_CAP=900"}}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	err = config.Save(context.Background(), cfg, storedCap, 901)
	var locked config.LockedError
	if !errors.Is(err, apperrors.ErrConflict) || !errors.As(err, &locked) || locked.Key != "test.cap" {
		t.Fatalf("save of a different value: %v", err)
	}
	if !strings.Contains(err.Error(), "X_TEST_CAP") || strings.Contains(err.Error(), "901") {
		t.Fatalf("the refusal must name the source and not the value: %v", err)
	}
	// No database is attached, so reaching the store would fail rather than pass.
	if err := config.Save(context.Background(), cfg, storedCap, 900); err != nil {
		t.Fatalf("saving the boot value itself is a no-op: %v", err)
	}
}

func TestAnUnlockedStoredSettingNeedsTheDatabase(t *testing.T) {
	cfg, err := load(t, fixture{}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.StoredVal(context.Background(), cfg, storedCap); err == nil ||
		!strings.Contains(err.Error(), "WithDatabase") {
		t.Fatalf("a read with no database must say so: %v", err)
	}
	if err := config.Save(context.Background(), cfg, storedCap, 5); err == nil {
		t.Fatal("a save with no database reported success")
	}
	if _, locked := cfg.Locked("test.cap"); locked || len(cfg.BootLocked()) != 0 {
		t.Fatalf("nothing sets test.cap at boot, yet it reads as locked: %v", cfg.BootLocked())
	}
}

// BootLocked feeds the seed's skip set, so a boot-only setting set by a flag
// must not appear in it, and StoredDefinitions must not register one.
func TestOnlyStoredSettingsAreLockedAndRegistered(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"X_TEST_CAP=900", "MARGINCE_LOG_LEVEL=debug"}}, storedCap, level)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.BootLocked(); len(got) != 1 || !got["test.cap"] {
		t.Fatalf("BootLocked = %v, want only test.cap", got)
	}
	defs := cfg.StoredDefinitions()
	if len(defs) != 1 || defs[0].Key() != "test.cap" || defs[0].Object() != "installation_settings" {
		t.Fatalf("StoredDefinitions = %v", defs)
	}
}
