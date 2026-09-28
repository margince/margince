// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Every refusal the role lifecycle can answer, one per case: who may not call
// it at all, what input it rejects, and which roles it cannot find.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A seat holding no role_admin is refused every lifecycle verb before any row
// is read.
func TestTheRoleLifecycleRefusesASeatWithoutTheRoleEditor(t *testing.T) {
	e := setupRevocationEnv(t, "role-noadmin")
	key := e.customRole(t, "Seasonal", "rep", nil)
	rep := e.seat(t, "plain", "rep")
	ctx := e.wsCtx(rep)
	name := "Renamed"
	for verb, err := range map[string]error{
		"create":          func() error { _, err := e.svc.CreateRole(ctx, rep, "rep", "Copy"); return err }(),
		"update":          func() error { _, err := e.svc.UpdateRole(ctx, rep, key, RoleChange{Name: &name}, nil); return err }(),
		"archive":         func() error { _, err := e.svc.ArchiveRole(ctx, rep, key); return err }(),
		"restore":         func() error { _, err := e.svc.RestoreRole(ctx, rep, key); return err }(),
		"list assignable": func() error { _, err := e.svc.ListAssignableRoles(ctx, rep); return err }(),
	} {
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a rep tried to %s: %v, want permission denied", verb, err)
		}
	}
}

// A name must be 1 to 120 characters after trimming, and a row scope one of
// the three the policy reads.
func TestTheRoleLifecycleRejectsANameOrScopeItCannotStore(t *testing.T) {
	e := setupRevocationEnv(t, "role-input")
	ctx := e.wsCtx(e.admin)
	key := e.customRole(t, "Seasonal", "rep", nil)
	blank, long := "   ", strings.Repeat("n", maxRoleName+1)
	galaxy := principal.RowScope("galaxy")

	for name, err := range map[string]error{
		"create with a blank name": func() error { _, err := e.svc.CreateRole(ctx, e.admin, "rep", blank); return err }(),
		"create with a long name":  func() error { _, err := e.svc.CreateRole(ctx, e.admin, "rep", long); return err }(),
		"rename to a blank name": func() error {
			_, err := e.svc.UpdateRole(ctx, e.admin, key, RoleChange{Name: &blank}, nil)
			return err
		}(),
		"rescope to an unknown scope": func() error {
			_, err := e.svc.UpdateRole(ctx, e.admin, key, RoleChange{RowScope: &galaxy}, nil)
			return err
		}(),
	} {
		var parse *values.ParseError
		if !errors.As(err, &parse) {
			t.Errorf("%s: %v, want a field refusal", name, err)
		}
	}
}

// A key nobody defines, and a role that is archived, are not roles the editor
// can change.
func TestTheRoleLifecycleAnswersUnknownForARoleItCannotEdit(t *testing.T) {
	e := setupRevocationEnv(t, "role-unknown")
	ctx := e.wsCtx(e.admin)
	archived := e.customRole(t, "Retired", "rep", nil)
	if _, err := e.svc.ArchiveRole(ctx, e.admin, archived); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	for verb, err := range map[string]error{
		"update an unknown key": func() error {
			_, err := e.svc.UpdateRole(ctx, e.admin, "custom_nobody", RoleChange{Name: &name}, nil)
			return err
		}(),
		"update an archived role": func() error {
			_, err := e.svc.UpdateRole(ctx, e.admin, archived, RoleChange{Name: &name}, nil)
			return err
		}(),
		"archive an unknown key": func() error { _, err := e.svc.ArchiveRole(ctx, e.admin, "custom_nobody"); return err }(),
		"restore an unknown key": func() error { _, err := e.svc.RestoreRole(ctx, e.admin, "custom_nobody"); return err }(),
	} {
		if !errors.Is(err, errUnknownRole) {
			t.Errorf("%s: %v, want errUnknownRole", verb, err)
		}
	}
}

// Changing nothing writes nothing, and restoring a live role is a no-op.
func TestARoleEditThatChangesNothingWritesNothing(t *testing.T) {
	e := setupRevocationEnv(t, "role-noop")
	ctx := e.wsCtx(e.admin)
	key := e.customRole(t, "Seasonal", "rep", nil)
	same, own := "Seasonal", principal.RowScopeOwn
	before, err := e.svc.UpdateRole(ctx, e.admin, key, RoleChange{Name: &same, RowScope: &own}, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := e.svc.RestoreRole(ctx, e.admin, key)
	if err != nil || restored.ArchivedAt != nil || restored.Version != before.Version {
		t.Errorf("restoring a live role: %+v %v, want it unchanged", restored, err)
	}
	if got := e.auditActions(t, e.roleID(t, key)); len(got) != 1 {
		t.Errorf("audit actions = %v, want only the create", got)
	}
}

// A restore refuses a name a live role took while this one was archived.
func TestARestoreRefusesANameTakenMeanwhile(t *testing.T) {
	e := setupRevocationEnv(t, "role-restore-name")
	ctx := e.wsCtx(e.admin)
	first := e.customRole(t, "Seasonal", "rep", nil)
	if _, err := e.svc.ArchiveRole(ctx, e.admin, first); err != nil {
		t.Fatal(err)
	}
	e.customRole(t, "Seasonal", "rep", nil)
	if _, err := e.svc.RestoreRole(ctx, e.admin, first); !errors.Is(err, errRoleNameTaken) {
		t.Errorf("restoring onto a taken name: %v, want errRoleNameTaken", err)
	}
}

// A name with nothing a key can carry still gets a key, and a long one is cut
// at 40 characters of slug, mid-word if need be, so the key fits the
// contract's 64.
func TestAKeyIsDerivedFromAnyName(t *testing.T) {
	e := setupRevocationEnv(t, "role-keys")
	ctx := e.wsCtx(e.admin)
	for name, want := range map[string]string{
		"Đội bán hàng":                  "custom_i_b_n_h_ng",
		"日本":                            "custom_role",
		strings.Repeat("Regional ", 10): "custom_regional_regional_regional_regional_regi",
	} {
		row, err := e.svc.CreateRole(ctx, e.admin, "rep", name)
		if err != nil {
			t.Fatalf("creating %q: %v", name, err)
		}
		if row.Key != want {
			t.Errorf("%q got key %q, want %q", name, row.Key, want)
		}
	}
}

// A member administrator acting on a member who is not there, or on an admin
// they may not reactivate, is refused before anything changes.
func TestADelegateActingOnAnUnreachableTargetIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "delegate-target")
	delegate := e.seat(t, "delegate", e.customRole(t, "Member admin", "rep", map[string]storedGrant{objectUserAdmin: fullGrant}))
	ctx := e.wsCtx(delegate)
	if err := e.svc.DeactivateUser(ctx, delegate, DeactivateUserInput{UserID: ids.New[ids.UserKind]()}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("deactivating nobody: %v, want not found", err)
	}
	second := e.seat(t, "second-admin", roleAdmin)
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: second.UserID}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ReactivateUser(ctx, delegate, second.UserID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a delegate reactivating an admin: %v, want permission denied", err)
	}
}
