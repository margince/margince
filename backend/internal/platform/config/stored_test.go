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

var storedCap = config.DefineStored(config.Spec[int]{
	Key: "test.cap", Env: "X_TEST_CAP", Type: config.Int,
	Default: config.Some(100), Min: config.Some(1), Doc: "d",
},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update"})

var appliedCap = config.DefineStored(config.Spec[int]{
	Key: "test.applied", Env: "X_TEST_APPLIED", Type: config.Int,
	Default: config.Some(1), Doc: "d",
},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update", MachineryApplied: true})

var appliedFloor = config.DefineStored(config.Spec[int]{
	Key: "test.applied_floor", Env: "X_TEST_APPLIED_FLOOR", Type: config.Int,
	Default: config.Some(1), Doc: "d",
},
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
	if code, message := locked.MessageFault(); code != "setting_locked_by_boot_source" || !strings.Contains(message, "X_TEST_CAP") {
		t.Fatalf("the refusal answers %q %q, want the lock's code and the source", code, message)
	}
	fromBase := config.LockedError{Key: "test.cap", Origin: config.Origin{Source: config.SourceBaseFile, Name: "test.cap", File: "base.yaml"}}
	if _, message := fromBase.MessageFault(); !strings.Contains(message, "config_file test.cap base.yaml") {
		t.Fatalf("a base-file lock reads %q, want the config_file the lock reports", message)
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

// The paired read refuses either setting the ungated readers may not read,
// before it reads any row.
func TestThePairedMachineryReadRefusesAnUndeclaredSetting(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"X_TEST_CAP=900", "X_TEST_APPLIED=7"}}, storedCap, appliedCap)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := config.StoredApplyPairTx(nobody, nil, cfg, appliedCap, storedCap); err == nil {
		t.Fatal("a pair holding a setting not declared MachineryApplied was read ungated")
	}
	if _, _, err := config.StoredApplyPairTx(nobody, nil, cfg, storedCap, appliedCap); err == nil {
		t.Fatal("a pair leading with a setting not declared MachineryApplied was read ungated")
	}
}

// A pair a boot source sets in full answers without the database, as a single
// machinery read does. A nil transaction proves no row is read.
func TestThePairedMachineryReadOfTwoBootValuesReadsNoRow(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"X_TEST_APPLIED=7", "X_TEST_APPLIED_FLOOR=3"}}, appliedCap, appliedFloor)
	if err != nil {
		t.Fatal(err)
	}
	a, b, err := config.StoredApplyPairTx(nobody, nil, cfg, appliedCap, appliedFloor)
	if err != nil || a != 7 || b != 3 {
		t.Fatalf("a paired read of two boot values = %d, %d, %v; want 7, 3 with no row read", a, b, err)
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

// A stored secret's Validate guards the save too, so its reason must be
// dropped there as it is at boot. The refusal comes before the value is sealed
// or the database is asked.
func TestAStoredSecretsRefusalDoesNotQuoteIt(t *testing.T) {
	echo := config.DefineStored(config.Spec[string]{
		Key: "test.key", Env: "X_TEST_KEY", Type: config.NonEmpty, Secret: true,
		Validate: func(s string) error { return errors.New("bad " + s) }, Doc: "d",
	},
		config.Store{RBACObject: "installation_settings", AuditVerb: "update", SecretReference: true})
	cfg, err := load(t, fixture{}, echo)
	if err != nil {
		t.Fatal(err)
	}
	err = config.Save(writer, cfg, echo, "hunter2")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("got %v", err)
	}
}

// A nil transaction proves the boot value answers without the database, and
// the read gate still comes first.
func TestTheTransactionReadTakesTheBootValueBehindItsGate(t *testing.T) {
	cfg := lockedCap(t)
	if v, err := config.StoredValTx(reader, nil, cfg, storedCap); err != nil || v != 900 {
		t.Fatalf("a locked read in a transaction = %d %v", v, err)
	}
	if _, err := config.StoredValTx(nobody, nil, cfg, storedCap); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a locked read without the grant = %v, want ErrPermissionDenied", err)
	}
}

func TestValidateJSONHoldsTheSpecBounds(t *testing.T) {
	if err := storedCap.ValidateJSON([]byte(`0`)); err == nil || !strings.Contains(err.Error(), "at least 1") {
		t.Fatalf("0 is below the minimum: %v", err)
	}
	if err := storedCap.ValidateJSON([]byte(`1`)); err != nil {
		t.Fatalf("1 is the minimum itself: %v", err)
	}
	if err := level.ValidateJSON([]byte(`"debug"`)); err == nil {
		t.Fatal("a boot-only setting has no row to validate against")
	}
}

// A writer that judges sameness itself gets the lock Save takes. There is no
// lock without a boot source. A value it calls the same is a no-op, and any
// other is a conflict that names the source.
func TestLockedChangeJudgesTheBootValueByTheCallersRule(t *testing.T) {
	unlocked, err := load(t, fixture{}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	if locked, err := config.LockedChange(unlocked, storedCap, func(int) bool { return false }); locked || err != nil {
		t.Fatalf("no boot source: locked %v, %v", locked, err)
	}
	cfg := lockedCap(t)
	if locked, err := config.LockedChange(cfg, storedCap, func(boot int) bool { return boot%100 == 0 }); !locked || err != nil {
		t.Fatalf("the same value by the caller's rule: locked %v, %v", locked, err)
	}
	locked, err := config.LockedChange(cfg, storedCap, func(int) bool { return false })
	if !locked || !errors.Is(err, apperrors.ErrConflict) || !strings.Contains(err.Error(), "X_TEST_CAP") {
		t.Fatalf("another value: locked %v, %v", locked, err)
	}
}

// A refusal of the value sent for a boot-set key reads as the lock. A locked
// field then answers 409 whether or not the value is valid. An unlocked key
// keeps its own refusal.
func TestLockFirstAnswersTheLockInPlaceOfARefusal(t *testing.T) {
	refused := errors.New("must be at least 1")
	var locked config.LockedError
	if err := lockedCap(t).LockFirst("test.cap", refused); !errors.As(err, &locked) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a refusal of a locked key = %v, want the LockedError", err)
	}
	unlocked, err := load(t, fixture{}, storedCap)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlocked.LockFirst("test.cap", refused); !errors.Is(err, refused) {
		t.Fatalf("a refusal of an unlocked key = %v, want it unchanged", err)
	}
	if err := lockedCap(t).LockFirst("test.cap", nil); err != nil {
		t.Fatalf("no refusal = %v, want nil", err)
	}
}

var sealedKey = config.DefineStored(config.Spec[string]{
	Key: "test.sealed", Env: "X_TEST_SEALED", Type: config.NonEmpty, Secret: true, Doc: "d",
},
	config.Store{RBACObject: "installation_settings", AuditVerb: "update", SecretReference: true})

func lockedSecret(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := load(t, fixture{env: []string{"X_TEST_SEALED=sk-boot"}}, sealedKey)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// A boot-set secret refuses even its own value: a no-op answer would confirm
// a caller's guess of it. No database is attached, so the lock is what answers.
func TestSavingABootSetSecretIsAConflictEvenForItsOwnValue(t *testing.T) {
	cfg := lockedSecret(t)
	for _, v := range []string{"sk-boot", "sk-other"} {
		err := config.Save(writer, cfg, sealedKey, v)
		if !errors.Is(err, apperrors.ErrConflict) || strings.Contains(err.Error(), v) {
			t.Fatalf("save of a secret over its boot value = %v, want a conflict that quotes nothing", err)
		}
	}
}

func TestRemovingABootSetSecretIsAConflict(t *testing.T) {
	if err := config.RemoveSecret(writer, lockedSecret(t), sealedKey); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("removing a boot-set secret = %v, want a conflict", err)
	}
	if err := config.RemoveSecret(reader, lockedSecret(t), sealedKey); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("removing without the grant = %v, want ErrPermissionDenied before the lock", err)
	}
}

// A sealed setting's row holds a ref. The readers and writers that treat the
// row as the value refuse it, rather than hand out the ref.
func TestARowReaderRefusesASealedSetting(t *testing.T) {
	cfg := lockedSecret(t)
	if _, err := config.StoredRequireTx(reader, nil, cfg, sealedKey); err == nil {
		t.Error("StoredRequireTx read a sealed setting's row")
	}
	if err := config.SaveTx(writer, cfg, nil, sealedKey, "sk-x"); err == nil {
		t.Error("SaveTx wrote a sealed setting's row in the clear")
	}
	if err := sealedKey.ValidateJSON([]byte(`"sk-x"`)); err == nil {
		t.Error("ValidateJSON judged a secret as its row")
	}
}

// The boot value answers a sealed setting without a vault or a database.
func TestABootSetSecretIsReadWithoutTheVault(t *testing.T) {
	v, origin, err := config.StoredValWithSource(reader, lockedSecret(t), sealedKey)
	if err != nil || v != "sk-boot" || origin.Name != "X_TEST_SEALED" {
		t.Fatalf("read %q %+v %v, want the boot value from X_TEST_SEALED", v, origin, err)
	}
}
