// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAGrantWithAWriteAndNoReadIsRefusedUnlessTheObjectOffersNoRead(t *testing.T) {
	e := setupRevocationEnv(t, "role-write-needs-read")
	ctx := e.wsCtx(e.admin)
	custom := e.customRole(t, "Contact editors", "rep", nil)

	for _, g := range []storedGrant{{Create: true}, {Update: true}, {Delete: true}} {
		if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, custom, "contact", g, nil); !errors.Is(err, errWriteNeedsRead) {
			t.Errorf("grant %+v without read: err = %v, want errWriteNeedsRead", g, err)
		}
	}
	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, custom, "contact", storedGrant{Read: true, Update: true}, nil); err != nil {
		t.Errorf("update with read: %v", err)
	}
	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, custom, "system_reset", storedGrant{Delete: true}, nil); err != nil {
		t.Errorf("delete on an object that offers no read: %v", err)
	}
}

func TestRestoringShippedGrantsReturnsASystemRoleToItsSeed(t *testing.T) {
	e := setupRevocationEnv(t, "role-restore")
	registerExtObject(t)
	ctx := e.wsCtx(e.admin)

	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, "rep", "contact", storedGrant{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, "rep", extObject, storedGrant{Read: true}, nil); err != nil {
		t.Fatal(err)
	}

	restored, err := e.svc.RestoreRoleGrants(ctx, e.admin, "rep", nil)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	shipped, _ := policy.ShippedGrants("rep")
	if got := restored.Objects["contact"]; got != storedGrant(shipped["contact"]) {
		t.Errorf("contact after restore = %+v, want the seeded %+v", got, shipped["contact"])
	}
	if _, kept := restored.Objects[extObject]; kept {
		t.Error("an extension object with no shipped default survived the restore")
	}
	want := shippedObjects("rep")
	if !reflect.DeepEqual(restored.Objects, want) {
		t.Errorf("restored objects differ from the shipped set")
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'role' AND action = 'update'
		    AND after::text LIKE '%contact%'`).Scan(&rows); err != nil || rows == 0 {
		t.Errorf("no audit row names the restore (rows=%d, err=%v)", rows, err)
	}
}

func TestRestoringShippedGrantsRefusesACustomRoleAndAWideningNonAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "role-restore-refusals")
	ctx := e.wsCtx(e.admin)
	custom := e.customRole(t, "Field sales", "rep", nil)
	if _, err := e.svc.RestoreRoleGrants(ctx, e.admin, custom, nil); !errors.Is(err, errNotSystemRole) {
		t.Errorf("custom role: err = %v, want errNotSystemRole", err)
	}
	if _, err := e.svc.RestoreRoleGrants(ctx, e.admin, "no_such_role", nil); !errors.Is(err, errUnknownRole) {
		t.Errorf("unknown role: err = %v, want errUnknownRole", err)
	}
	stale := int64(0)
	if _, err := e.svc.RestoreRoleGrants(ctx, e.admin, "rep", &stale); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("stale version: err = %v, want version skew", err)
	}

	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, "rep", "contact", storedGrant{}, nil); err != nil {
		t.Fatal(err)
	}
	editor := e.seat(t, "editor", e.customRole(t, "Role editor", "rep", map[string]storedGrant{objectRoleAdmin: fullGrant}))
	if _, err := e.svc.RestoreRoleGrants(e.wsCtx(editor), editor, "rep", nil); !errors.Is(err, errWideningRequiresAdmin) {
		t.Errorf("widening restore by a non-admin: err = %v, want errWideningRequiresAdmin", err)
	}
}
