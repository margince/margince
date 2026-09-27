// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A custom role's life through the role editor: made by copying, renamed, its
// row scope moved, archived and restored. Every role and member here is made
// through the product's own writers, so what is proved is what an operator
// can reach.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var fullGrant = storedGrant{Create: true, Read: true, Update: true, Delete: true}

// customRole makes a role the way an admin does: a copy of `from`, then each
// grant set through the editor. Returns its key.
func (e *revocationEnv) customRole(t *testing.T, name, from string, grants map[string]storedGrant) string {
	t.Helper()
	ctx := e.wsCtx(e.admin)
	row, err := e.svc.CreateRole(ctx, e.admin, from, name)
	if err != nil {
		t.Fatalf("creating %q from %s: %v", name, from, err)
	}
	for object, grant := range grants {
		if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, row.Key, object, grant, nil); err != nil {
			t.Fatalf("granting %s on %s: %v", object, row.Key, err)
		}
	}
	return row.Key
}

// scopeRole moves a role's row scope as an admin.
func (e *revocationEnv) scopeRole(t *testing.T, key string, scope principal.RowScope) {
	t.Helper()
	if _, err := e.svc.UpdateRole(e.wsCtx(e.admin), e.admin, key, RoleChange{RowScope: &scope}, nil); err != nil {
		t.Fatalf("scoping %s to %s: %v", key, scope, err)
	}
}

// seat invites a member holding role, in teams, and resolves them the way
// every request does.
func (e *revocationEnv) seat(t *testing.T, handle, role string, teams ...ids.UUID) Identity {
	t.Helper()
	userID, _, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: handle + "@" + e.slug + ".test", DisplayName: handle, Role: role, TeamIDs: teams,
	})
	if err != nil {
		t.Fatalf("inviting %s as %s: %v", handle, role, err)
	}
	return e.resolve(t, userID)
}

// resolve reads a member's roles, teams and grants through loadGrants, the
// loader authentication runs.
func (e *revocationEnv) resolve(t *testing.T, userID ids.UserID) Identity {
	t.Helper()
	id := Identity{UserID: userID, WorkspaceID: e.admin.WorkspaceID, SeatType: string(principal.SeatFull)}
	err := e.svc.db.Tx(e.wsCtx(e.admin), func(tx pgx.Tx) error {
		var err error
		id.Roles, id.Teams, id.Permissions, err = loadGrants(context.Background(), tx, userID)
		return err
	})
	if err != nil {
		t.Fatalf("resolving %s: %v", userID, err)
	}
	return id
}

// maskRole authors one field mask on a role. The product has no mask editor,
// so SQL is how an operator writes one today.
func (e *revocationEnv) maskRole(t *testing.T, key string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, 'deal', 'amount_minor', 'always')`,
		key); err != nil {
		t.Fatalf("masking %s: %v", key, err)
	}
	t.Cleanup(func() {
		if _, err := e.owner.Exec(context.Background(), `DELETE FROM field_mask WHERE role_key = $1`, key); err != nil {
			t.Errorf("removing %s's mask: %v", key, err)
		}
	})
}

func (e *revocationEnv) auditActions(t *testing.T, entity ids.UUID) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT action FROM audit_log WHERE entity_type = 'role' AND entity_id = $1 ORDER BY occurred_at, id`, entity)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return actions
}

func (e *revocationEnv) roleID(t *testing.T, key string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT id FROM role WHERE key = $1`, key).Scan(&id); err != nil {
		t.Fatalf("reading %s's id: %v", key, err)
	}
	return id
}

// A copy keeps what its source stores, including a grant on an extension this
// installation no longer composes, and the source's field masks.
func TestACopiedRoleKeepsItsSourcesGrantsAndMasks(t *testing.T) {
	e := setupRevocationEnv(t, "role-copy")
	registerExtObject(t)
	source := e.customRole(t, "Field sales", "rep", map[string]storedGrant{extObject: {Read: true}})
	e.maskRole(t, source)
	ResetRbacObjectsForTest()

	copied, err := e.svc.CreateRole(e.wsCtx(e.admin), e.admin, source, "  Field sales DACH ")
	if err != nil {
		t.Fatalf("copying: %v", err)
	}
	if copied.Key != "custom_field_sales_dach" || copied.Name != "Field sales DACH" || copied.IsSystem {
		t.Errorf("copy = key %q name %q system %v, want custom_field_sales_dach, the trimmed name, not system",
			copied.Key, copied.Name, copied.IsSystem)
	}
	if !copied.Objects[extObject].Read {
		t.Errorf("the copy lost the grant on %s, an extension that is not composed now", extObject)
	}
	var masks int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM field_mask WHERE role_key = $1`, copied.Key).Scan(&masks); err != nil || masks != 1 {
		t.Errorf("the copy carries %d field masks (%v), want the source's 1", masks, err)
	}
	if got := e.auditActions(t, e.roleID(t, copied.Key)); len(got) != 1 || got[0] != "create" {
		t.Errorf("audit actions = %v, want one create", got)
	}
	// A second copy under the same name is refused; under a name that slugs
	// alike it gets its own key.
	if _, err := e.svc.CreateRole(e.wsCtx(e.admin), e.admin, source, "field sales dach"); !errors.Is(err, errRoleNameTaken) {
		t.Errorf("a second role with the same name, other case: %v, want errRoleNameTaken", err)
	}
	second, err := e.svc.CreateRole(e.wsCtx(e.admin), e.admin, source, "Field-Sales DACH!")
	if err != nil || second.Key != "custom_field_sales_dach_2" {
		t.Errorf("a name slugging to a used key got %q (%v), want custom_field_sales_dach_2", second.Key, err)
	}
	if _, err := e.svc.CreateRole(e.wsCtx(e.admin), e.admin, "no_such_role", "Ghost"); !errors.Is(err, errUnknownRole) {
		t.Errorf("copying an unknown role: %v, want errUnknownRole", err)
	}
}

// Rename and row scope go through If-Match, and each changed field is audited.
func TestARoleIsRenamedAndRescopedUnderIfMatch(t *testing.T) {
	e := setupRevocationEnv(t, "role-update")
	ctx := e.wsCtx(e.admin)
	key := e.customRole(t, "Inside sales", "rep", nil)
	taken := e.customRole(t, "Partner desk", "rep", nil)
	before, err := e.svc.ListRoles(ctx, e.admin, false)
	if err != nil {
		t.Fatal(err)
	}
	var version int64
	for _, row := range before {
		if row.Key == key {
			version = row.Version
		}
	}
	name, scope := "Inside sales EMEA", principal.RowScopeTeam
	updated, err := e.svc.UpdateRole(ctx, e.admin, key, RoleChange{Name: &name, RowScope: &scope}, &version)
	if err != nil {
		t.Fatalf("updating: %v", err)
	}
	if updated.Name != name || updated.RowScope != principal.RowScopeTeam || updated.Key != key {
		t.Errorf("updated = %+v, want the new name and team scope under the same key", updated)
	}
	if _, err := e.svc.UpdateRole(ctx, e.admin, key, RoleChange{Name: &name}, &version); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("a write from the stale version: %v, want ErrVersionSkew", err)
	}
	if _, err := e.svc.UpdateRole(ctx, e.admin, taken, RoleChange{Name: &name}, nil); !errors.Is(err, errRoleNameTaken) {
		t.Errorf("renaming onto a live role's name: %v, want errRoleNameTaken", err)
	}
	if got := e.auditActions(t, e.roleID(t, key)); len(got) != 2 || got[1] != "update" {
		t.Errorf("audit actions = %v, want create then update", got)
	}
}

// Widening passes through the caller's own authority; narrowing does not.
func TestWideningARoleNeedsTheCallersOwnAuthority(t *testing.T) {
	e := setupRevocationEnv(t, "role-widen")
	editorRole := e.customRole(t, "Role editor", "rep", map[string]storedGrant{objectRoleAdmin: fullGrant})
	editor := e.seat(t, "editor", editorRole)
	other := e.customRole(t, "Viewer", "read_only", nil)
	ctx := e.wsCtx(editor)

	if _, err := e.svc.SetRoleObjectGrant(ctx, editor, other, "deal", storedGrant{Create: true, Read: true}, nil); err != nil {
		t.Errorf("turning on a verb the editor holds, on a role they do not hold: %v", err)
	}
	if _, err := e.svc.SetRoleObjectGrant(ctx, editor, other, "deal", storedGrant{Read: true, Delete: true}, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("turning on delete, which the editor lacks: %v, want permission denied", err)
	}
	if _, err := e.svc.SetRoleObjectGrant(e.wsCtx(e.admin), e.admin, other, "deal", fullGrant, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetRoleObjectGrant(ctx, editor, other, "deal", storedGrant{Read: true, Delete: true}, nil); err != nil {
		t.Errorf("narrowing a role while keeping a verb the editor lacks: %v, want allowed", err)
	}
	all := principal.RowScopeAll
	if _, err := e.svc.UpdateRole(ctx, editor, other, RoleChange{RowScope: &all}, nil); err != nil {
		t.Errorf("keeping the viewer at all: %v", err)
	}
	team := principal.RowScopeTeam
	if _, err := e.svc.UpdateRole(ctx, editor, other, RoleChange{RowScope: &team}, nil); err != nil {
		t.Errorf("narrowing the viewer's row scope: %v, want allowed", err)
	}
	if _, err := e.svc.UpdateRole(ctx, editor, other, RoleChange{RowScope: &all}, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("widening past the editor's own row scope: %v, want permission denied", err)
	}
	if _, err := e.svc.CreateRole(ctx, editor, "admin", "Second admin"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("copying admin, which is wider than the editor: %v, want permission denied", err)
	}
	if _, err := e.svc.CreateRole(ctx, editor, "rep", "Plain copy"); err != nil {
		t.Errorf("copying rep, which the editor contains: %v", err)
	}
}

// The admin role cannot lose its administration grants; an admin copy is not
// an admin to the last-admin guard.
func TestTheAdminRoleKeepsItsFloorAndACopyIsNotAnAdmin(t *testing.T) {
	e := setupRevocationEnv(t, "role-floor")
	ctx := e.wsCtx(e.admin)
	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, roleAdmin, objectRoleAdmin, storedGrant{Read: true}, nil); !errors.Is(err, errAdminRoleFloor) {
		t.Errorf("narrowing admin's role_admin: %v, want errAdminRoleFloor", err)
	}
	if _, err := e.svc.SetRoleObjectGrant(ctx, e.admin, roleAdmin, "deal", storedGrant{Read: true}, nil); err != nil {
		t.Errorf("narrowing admin's record grant: %v, want allowed", err)
	}
	adminCopy := e.customRole(t, "Admin copy", roleAdmin, nil)
	if err := e.svc.ChangeUserRole(ctx, e.admin, e.member.UserID, adminCopy); err != nil {
		t.Fatalf("giving the member the admin copy: %v", err)
	}
	if err := e.svc.DeactivateUser(ctx, e.admin, DeactivateUserInput{UserID: e.admin.UserID}); !errors.Is(err, errLastActiveAdmin) {
		t.Errorf("deactivating the only literal admin beside an admin copy: %v, want errLastActiveAdmin", err)
	}
}

// Archive refuses a system role and a role anybody who can sign in holds; an
// archived role grants nothing and cannot be assigned; restore brings it back
// with its masks.
func TestArchivingARoleTakesItOutOfUseAndRestoreBringsItBack(t *testing.T) {
	e := setupRevocationEnv(t, "role-archive")
	ctx := e.wsCtx(e.admin)
	key := e.customRole(t, "Seasonal", "rep", nil)
	e.maskRole(t, key)
	holder := e.seat(t, "seasonal", key)

	if _, err := e.svc.ArchiveRole(ctx, e.admin, "rep"); !errors.Is(err, errSystemRole) {
		t.Errorf("archiving rep: %v, want errSystemRole", err)
	}
	if _, err := e.svc.ArchiveRole(ctx, e.admin, key); !errors.Is(err, errRoleInUse) {
		t.Errorf("archiving a role an invited member holds: %v, want errRoleInUse", err)
	}
	if err := e.svc.DeactivateUser(ctx, e.admin, DeactivateUserInput{UserID: holder.UserID}); err != nil {
		t.Fatal(err)
	}
	archived, err := e.svc.ArchiveRole(ctx, e.admin, key)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archiving once only a deactivated member holds it: %+v %v", archived, err)
	}
	if again, err := e.svc.ArchiveRole(ctx, e.admin, key); err != nil || again.ArchivedAt == nil {
		t.Errorf("archiving again: %v, want a no-op", err)
	}

	if grants := e.resolve(t, holder.UserID); len(grants.Roles) != 0 || grants.Permissions.Allows("deal", principal.ActionRead) {
		t.Errorf("a holder of an archived role resolves to roles %v with deal.read %v, want nothing",
			grants.Roles, grants.Permissions.Allows("deal", principal.ActionRead))
	}
	access, err := e.svc.UserAccess(ctx, e.admin, holder.UserID)
	if err != nil || access.Permissions.Allows("deal", principal.ActionRead) {
		t.Errorf("the access read shows the archived role's grants (%v), want none", err)
	}
	if _, err := e.svc.PreviewAccess(ctx, e.admin, key, nil); !errors.Is(err, errUnknownRole) {
		t.Errorf("previewing an archived role: %v, want errUnknownRole", err)
	}
	if err := e.svc.ChangeUserRole(ctx, e.admin, e.member.UserID, key); !errors.Is(err, errUnknownRole) {
		t.Errorf("assigning an archived role: %v, want errUnknownRole", err)
	}
	live, err := e.svc.ListRoles(ctx, e.admin, false)
	if err != nil || containsRoleKey(live, key) {
		t.Errorf("the live list carries the archived role (%v)", err)
	}
	every, err := e.svc.ListRoles(ctx, e.admin, true)
	if err != nil || !containsRoleKey(every, key) {
		t.Errorf("the list with archived roles lacks it (%v)", err)
	}

	restored, err := e.svc.RestoreRole(ctx, e.admin, key)
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("restoring: %+v %v", restored, err)
	}
	if masks := e.resolve(t, holder.UserID).Permissions.FieldMasks; len(masks) != 1 {
		t.Errorf("the restored role carries %d masks, want the 1 it had", len(masks))
	}
	if got := e.auditActions(t, e.roleID(t, key)); len(got) != 3 || got[1] != "archive" || got[2] != "restore" {
		t.Errorf("audit actions = %v, want create, archive, restore", got)
	}
}

func containsRoleKey(rows []roleRow, key string) bool {
	for _, row := range rows {
		if row.Key == key {
			return true
		}
	}
	return false
}

// An assignment reaching a role while an archive of it is in flight waits for
// the archive and then finds no live role. Without the lock it would read the
// role as live and land a member on a role that grants nothing.
func TestAnAssignmentRacingAnArchiveFindsNoRole(t *testing.T) {
	e := setupRevocationEnv(t, "role-race")
	ctx := e.wsCtx(e.admin)
	key := e.customRole(t, "Temporary", "rep", nil)

	archiving := make(chan struct{})
	release := make(chan struct{})
	archived := make(chan error, 1)
	go func() {
		archived <- e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := archiveRoleTx(ctx, tx, key); err != nil {
				return err
			}
			close(archiving)
			<-release
			return nil
		})
	}()
	<-archiving

	assigned := make(chan error, 1)
	go func() { assigned <- e.svc.ChangeUserRole(ctx, e.admin, e.member.UserID, key) }()
	waitForALockWaiter(t, e)
	close(release)

	if err := <-archived; err != nil {
		t.Fatalf("the archive: %v", err)
	}
	if err := <-assigned; !errors.Is(err, errUnknownRole) {
		t.Errorf("an assignment that waited on the archive: %v, want errUnknownRole", err)
	}
}

// waitForALockWaiter returns once some backend is blocked on a row lock, which
// is the assignment reaching the role the archive holds.
func waitForALockWaiter(t *testing.T, e *revocationEnv) {
	t.Helper()
	for range 10_000 {
		var waiting bool
		if err := e.owner.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
	t.Fatal("the assignment never waited on the role lock — it read past the archive")
}
