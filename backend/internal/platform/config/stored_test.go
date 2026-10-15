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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The database-backed half is in internal/compose/integration; these cases
// need none, because a boot source answers before the database is asked.

var storedCap = config.DefineStored(config.Spec[int]{Key: "test.cap", Env: "X_TEST_CAP", Type: config.Int,
	Default: config.Some(100), Min: config.Some(1), Doc: "d"},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update"})

var appliedCap = config.DefineStored(config.Spec[int]{Key: "test.applied", Env: "X_TEST_APPLIED", Type: config.Int,
	Default: config.Some(1), Doc: "d"},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update", MachineryApplied: true})

func asHolding(grant principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects:  map[string]principal.ObjectGrant{"installation_settings": grant},
		},
	})
}

var (
	reader = asHolding(principal.ObjectGrant{Read: true})
	writer = asHolding(principal.ObjectGrant{Read: true, Update: true})
	nobody = asHolding(principal.ObjectGrant{})
)

func lockedCap(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := load(t, fixture{env: []string{"X_TEST_CAP=900"}}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestABootSourceAnswersAStoredSettingWithoutTheDatabase(t *testing.T) {
	cfg := lockedCap(t)
	v, origin, err := config.StoredValWithSource(reader, cfg, storedCap)
	if err != nil || v != 900 || origin.Source != config.SourceEnv || origin.Name != "X_TEST_CAP" {
		t.Fatalf("got %d %+v %v", v, origin, err)
	}
	if o, locked := cfg.Locked("test.cap"); !locked || o.Name != "X_TEST_CAP" {
		t.Fatalf("Locked = %+v %v", o, locked)
	}
}

// A lock is not a grant: a caller without read learns neither the value nor
// that a boot source sets it.
func TestALockedSettingKeepsItsReadGate(t *testing.T) {
	cfg := lockedCap(t)
	if _, _, err := config.StoredValWithSource(nobody, cfg, storedCap); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a locked read without the grant = %v, want ErrPermissionDenied", err)
	}
	if _, err := cfg.StoredEntries(nobody); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an overview of locked settings without the grant = %v, want ErrPermissionDenied", err)
	}
	entries, err := cfg.StoredEntries(reader)
	if err != nil || len(entries) != 1 || entries[0].Value == nil || *entries[0].Value != "900" {
		t.Fatalf("a reader's overview of a locked setting = %+v %v", entries, err)
	}
}

func TestSavingOverABootSourceIsAConflictThatNamesTheSource(t *testing.T) {
	cfg := lockedCap(t)
	err := config.Save(writer, cfg, storedCap, 901)
	var locked config.LockedError
	if !errors.Is(err, apperrors.ErrConflict) || !errors.As(err, &locked) || locked.Key != "test.cap" {
		t.Fatalf("save of a different value: %v", err)
	}
	if !strings.Contains(err.Error(), "X_TEST_CAP") || strings.Contains(err.Error(), "901") {
		t.Fatalf("the refusal must name the source and not the value: %v", err)
	}
	// No database is attached, so reaching the store would fail rather than pass.
	if err := config.Save(writer, cfg, storedCap, 900); err != nil {
		t.Fatalf("saving the boot value itself is a no-op: %v", err)
	}
}

func TestSavingALockedSettingWithoutTheGrantIsRefusedNotAConflict(t *testing.T) {
	cfg := lockedCap(t)
	for _, v := range []int{900, 901} {
		if err := config.Save(reader, cfg, storedCap, v); !errors.Is(err, apperrors.ErrPermissionDenied) ||
			errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("save of %d without update = %v, want ErrPermissionDenied", v, err)
		}
	}
}

// The machinery read mirrors settings.ApplyTx: no read gate, and only for an
// entry declared MachineryApplied. A nil transaction proves the boot value
// answers without the database.
func TestTheMachineryReadTakesTheBootValueWithoutAGate(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"X_TEST_CAP=900", "X_TEST_APPLIED=7"}}, storedCap, appliedCap)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := config.StoredApplyTx(nobody, nil, cfg, appliedCap); err != nil || v != 7 {
		t.Fatalf("machinery read of a locked setting = %d %v", v, err)
	}
	if _, err := config.StoredApplyTx(nobody, nil, cfg, storedCap); err == nil {
		t.Fatal("a setting not declared MachineryApplied was read ungated")
	}
}

func TestAnUnlockedStoredSettingNeedsTheDatabase(t *testing.T) {
	cfg, err := load(t, fixture{}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.StoredVal(reader, cfg, storedCap); err == nil ||
		!strings.Contains(err.Error(), "WithDatabase") {
		t.Fatalf("a read with no database must say so: %v", err)
	}
	if err := config.Save(writer, cfg, storedCap, 5); err == nil {
		t.Fatal("a save with no database reported success")
	}
	if _, locked := cfg.Locked("test.cap"); locked || len(cfg.BootLocked()) != 0 {
		t.Fatalf("nothing sets test.cap at boot, yet it reads as locked: %v", cfg.BootLocked())
	}
}

func TestABootOnlySettingIsNotReadAsStored(t *testing.T) {
	cfg, err := load(t, fixture{}, level)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.StoredVal(reader, cfg, level); err == nil {
		t.Fatal("a boot-only setting was read as a stored one")
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
