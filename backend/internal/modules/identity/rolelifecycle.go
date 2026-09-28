// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A role's life apart from its grant map: made by copying another, renamed,
// its row scope moved, archived and restored. roles.go edits the grant map.
//
// None of these writes emits an event. The closed catalog's one role verb,
// role.changed, describes a MEMBER's assignment, and nobody's assignment moves
// here. The audit row carries each change with its before and after, which is
// what a permission investigation reads.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

var (
	errRoleNameTaken = fmt.Errorf("%w: a live role already has this name", apperrors.ErrConflict)
	errSystemRole    = fmt.Errorf("%w: a role the product ships cannot be archived", apperrors.ErrConflict)
	errRoleInUse     = fmt.Errorf("%w: a member who can sign in holds this role", apperrors.ErrConflict)
)

// maxRoleName bounds a role's name, as the contract does.
const maxRoleName = 120

var rowScopes = []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll}

// validRoleName trims and bounds a role name.
func validRoleName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxRoleName {
		return "", &values.ParseError{
			Field: roleFieldName, Code: "invalid_role_name",
			Message: fmt.Sprintf("a role name is 1 to %d characters", maxRoleName),
		}
	}
	return name, nil
}

// customRoleKeyPrefix keeps a made role's key apart from every seeded key.
const customRoleKeyPrefix = "custom_"

// The audit entity and image keys the role writers share.
const (
	roleEntity        = "role"
	roleFieldName     = "name"
	roleFieldRowScope = "row_scope"
	roleFieldArchived = "archived"
)

// roleKeyUnsafe matches a run of characters a key does not carry.
var roleKeyUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// CreateRole makes a role as a copy of a live one, under a new name and a key
// generated from it. Admin only: a new role is authority nobody held before.
//
// The stored document is copied as it is, not through policy.Parse: Parse
// drops a grant on an object this installation cannot name, so a copy made
// while an extension is removed would lose that grant for good. The source's
// field masks come with it in the same transaction.
func (s *Service) CreateRole(ctx context.Context, actor Identity, copyFrom, name string) (roleRow, error) {
	ctx, err := admit(ctx, actor, objectRoleAdmin, principal.ActionCreate)
	if err != nil {
		return roleRow{}, err
	}
	if err := refuseWideningUnlessAdmin(actor, true); err != nil {
		return roleRow{}, err
	}
	name, err = validRoleName(name)
	if err != nil {
		return roleRow{}, err
	}
	var created roleRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var sourceID ids.UUID
		err := tx.QueryRow(ctx,
			`SELECT id FROM role WHERE key = $1 AND archived_at IS NULL`,
			copyFrom).Scan(&sourceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errUnknownRole
		}
		if err != nil {
			return err
		}
		if err := refuseTakenRoleName(ctx, tx, name, ""); err != nil {
			return err
		}
		key, err := freeRoleKey(ctx, tx, name)
		if err != nil {
			return err
		}
		created, err = insertRoleCopy(ctx, tx, sourceID, copyFrom, key, name)
		return err
	})
	if err != nil {
		return roleRow{}, err
	}
	return created, nil
}

// insertRoleCopy writes the new role row, copies the source's field masks and
// audits the creation.
func insertRoleCopy(ctx context.Context, tx pgx.Tx, sourceID ids.UUID, sourceKey, key, name string) (roleRow, error) {
	var roleID ids.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO role (key, name, is_system, permissions)
		 SELECT $2, $3, false, permissions FROM role WHERE id = $1
		 RETURNING id`, sourceID, key, name).Scan(&roleID); err != nil {
		return roleRow{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO field_mask (role_key, object, field, condition)
		 SELECT $2, object, field, condition FROM field_mask WHERE role_key = $1`,
		sourceKey, key); err != nil {
		return roleRow{}, err
	}
	created, err := scanRoleRow(tx.QueryRow(ctx, `SELECT `+roleColumns+` FROM role WHERE id = $1`, roleID))
	if err != nil {
		return roleRow{}, err
	}
	_, err = storekit.Audit(ctx, tx, "create", roleEntity, roleID, nil, map[string]any{
		"key": key, roleFieldName: name, "copied_from": sourceKey, roleFieldRowScope: created.RowScope,
	})
	if err != nil {
		return roleRow{}, err
	}
	return created, nil
}

// RoleChange is a sparse edit of a role's name and row scope; nil leaves it.
type RoleChange struct {
	Name     *string
	RowScope *principal.RowScope
}

// UpdateRole renames a live role or moves its row scope.
//
// Renaming and narrowing are open to every role_admin.update holder: neither
// gives anybody anything. Widening the row scope reaches every holder's teams
// at once, so only an admin does it.
func (s *Service) UpdateRole(ctx context.Context, actor Identity, key string, change RoleChange, ifVersion *int64) (roleRow, error) {
	ctx, err := admit(ctx, actor, objectRoleAdmin, principal.ActionUpdate)
	if err != nil {
		return roleRow{}, err
	}
	if change.Name != nil {
		name, err := validRoleName(*change.Name)
		if err != nil {
			return roleRow{}, err
		}
		change.Name = &name
	}
	if change.RowScope != nil && !slices.Contains(rowScopes, *change.RowScope) {
		return roleRow{}, &values.ParseError{
			Field: roleFieldRowScope, Code: "invalid_row_scope",
			Message: "a row scope is own, team or all",
		}
	}
	var updated roleRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var err error
		updated, err = applyRoleChange(ctx, tx, actor, key, change, ifVersion)
		return err
	})
	if err != nil {
		return roleRow{}, err
	}
	return updated, nil
}

// applyRoleChange locks the live role, checks the caller's If-Match and the
// widening rule under the lock, then writes the changed fields and audits
// exactly those. The row scope is a targeted jsonb_set, so the rest of the
// document stays as stored.
func applyRoleChange(ctx context.Context, tx pgx.Tx, actor Identity, key string, change RoleChange, ifVersion *int64) (roleRow, error) {
	roleID, err := roleIDByKey(ctx, tx, key)
	if err != nil {
		return roleRow{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, roleEntity, roleID, storekit.LiveOnly); err != nil {
		return roleRow{}, unknownRoleIfMissing(err)
	}
	before, err := readRole(ctx, tx, roleID)
	if err != nil {
		return roleRow{}, err
	}
	if ifVersion != nil && *ifVersion != before.Version {
		return roleRow{}, apperrors.ErrVersionSkew
	}
	widens := change.RowScope != nil && change.RowScope.Wider(before.RowScope)
	if err := refuseWideningUnlessAdmin(actor, widens); err != nil {
		return roleRow{}, err
	}
	if change.Name != nil && !strings.EqualFold(*change.Name, before.Name) {
		if err := refuseTakenRoleName(ctx, tx, *change.Name, key); err != nil {
			return roleRow{}, err
		}
	}
	after, beforeImage, afterImage := roleChangeImages(before, change)
	if len(afterImage) == 0 {
		return before, nil
	}
	updated, err := scanRoleRow(tx.QueryRow(ctx,
		`UPDATE role SET name = $2, permissions = jsonb_set(permissions, '{row_scope}', to_jsonb($3::text), true)
		  WHERE id = $1 AND archived_at IS NULL RETURNING `+roleColumns, roleID, after.Name, string(after.RowScope)))
	if err != nil {
		return roleRow{}, err
	}
	if _, err := storekit.Audit(ctx, tx, "update", roleEntity, roleID, beforeImage, afterImage); err != nil {
		return roleRow{}, err
	}
	return updated, nil
}

// roleChangeImages applies a change to a role and names the fields it moved,
// before and after, for the audit row. Empty images mean nothing moved.
func roleChangeImages(before roleRow, change RoleChange) (after roleRow, beforeImage, afterImage map[string]any) {
	after, beforeImage, afterImage = before, map[string]any{}, map[string]any{}
	if change.Name != nil && *change.Name != before.Name {
		after.Name = *change.Name
		beforeImage[roleFieldName], afterImage[roleFieldName] = before.Name, after.Name
	}
	if change.RowScope != nil && *change.RowScope != before.RowScope {
		after.RowScope = *change.RowScope
		beforeImage[roleFieldRowScope], afterImage[roleFieldRowScope] = before.RowScope, after.RowScope
	}
	return after, beforeImage, afterImage
}

// ArchiveRole takes a custom role out of use. It grants nothing while archived
// and cannot be assigned; its grants and field masks stay for RestoreRole.
// Archiving an archived role changes nothing.
func (s *Service) ArchiveRole(ctx context.Context, actor Identity, key string) (roleRow, error) {
	ctx, err := admit(ctx, actor, objectRoleAdmin, principal.ActionDelete)
	if err != nil {
		return roleRow{}, err
	}
	var out roleRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var err error
		out, err = archiveRoleTx(ctx, tx, key)
		return err
	})
	if err != nil {
		return roleRow{}, err
	}
	return out, nil
}

// archiveRoleTx locks the role, refuses a system role and one a member who can
// sign in holds, and archives it.
//
// Its caller holds lockAuthorization, as every assignment does, so an
// assignment racing this either commits first and is counted here, or waits
// and then finds no live role — a member cannot end up on an archived role
// through it.
func archiveRoleTx(ctx context.Context, tx pgx.Tx, key string) (roleRow, error) {
	roleID, err := roleIDByKey(ctx, tx, key)
	if err != nil {
		return roleRow{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, roleEntity, roleID, storekit.IncludeArchived); err != nil {
		return roleRow{}, unknownRoleIfMissing(err)
	}
	row, err := readRole(ctx, tx, roleID)
	if err != nil || row.ArchivedAt != nil {
		return row, err
	}
	if row.IsSystem {
		return roleRow{}, errSystemRole
	}
	var held bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM role_assignment ra JOIN app_user u ON u.id = ra.user_id
		  WHERE ra.role_id = $1 AND `+ActivatableMemberSQL("u")+`)`, roleID).Scan(&held); err != nil {
		return roleRow{}, err
	}
	if held {
		return roleRow{}, errRoleInUse
	}
	archived, err := scanRoleRow(tx.QueryRow(ctx,
		`UPDATE role SET archived_at = now() WHERE id = $1 AND archived_at IS NULL RETURNING `+roleColumns, roleID))
	if err != nil {
		return roleRow{}, err
	}
	if _, err := storekit.Audit(ctx, tx, "archive", roleEntity, roleID,
		map[string]any{roleFieldArchived: false}, map[string]any{roleFieldArchived: true}); err != nil {
		return roleRow{}, err
	}
	return archived, nil
}

// RestoreRole brings an archived role back with the grants and masks it had.
// Restoring a live role changes nothing. Admin only: a restored role hands its
// grants back to every member still holding it, with no assignment check.
func (s *Service) RestoreRole(ctx context.Context, actor Identity, key string) (roleRow, error) {
	ctx, err := admit(ctx, actor, objectRoleAdmin, principal.ActionDelete)
	if err != nil {
		return roleRow{}, err
	}
	if err := refuseWideningUnlessAdmin(actor, true); err != nil {
		return roleRow{}, err
	}
	var out roleRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var err error
		out, err = restoreRoleTx(ctx, tx, key)
		return err
	})
	if err != nil {
		return roleRow{}, err
	}
	return out, nil
}

// restoreRoleTx reaches an archived role on purpose, which is what restoring
// is, and refuses when a live role has taken its name meanwhile.
func restoreRoleTx(ctx context.Context, tx pgx.Tx, key string) (roleRow, error) {
	roleID, err := roleIDByKey(ctx, tx, key)
	if err != nil {
		return roleRow{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, roleEntity, roleID, storekit.IncludeArchived); err != nil {
		return roleRow{}, unknownRoleIfMissing(err)
	}
	row, err := readRole(ctx, tx, roleID)
	if err != nil || row.ArchivedAt == nil {
		return row, err
	}
	if err := refuseTakenRoleName(ctx, tx, row.Name, key); err != nil {
		return roleRow{}, err
	}
	restored, err := scanRoleRow(tx.QueryRow(ctx,
		`UPDATE role SET archived_at = NULL WHERE id = $1 RETURNING `+roleColumns, roleID))
	if err != nil {
		return roleRow{}, err
	}
	if _, err := storekit.Audit(ctx, tx, "restore", roleEntity, roleID,
		map[string]any{roleFieldArchived: true}, map[string]any{roleFieldArchived: false}); err != nil {
		return roleRow{}, err
	}
	return restored, nil
}

// roleIDByKey resolves a key to its row. A role's id and key never change, so
// a lock taken on the id afterwards holds the role the key named.
func roleIDByKey(ctx context.Context, tx pgx.Tx, key string) (ids.UUID, error) {
	var roleID ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM role WHERE key = $1`, key).Scan(&roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, errUnknownRole
	}
	return roleID, err
}

func readRole(ctx context.Context, tx pgx.Tx, roleID ids.UUID) (roleRow, error) {
	return scanRoleRow(tx.QueryRow(ctx, `SELECT `+roleColumns+` FROM role WHERE id = $1`, roleID))
}

// unknownRoleIfMissing names a role the lock did not find the way every other
// role read does.
func unknownRoleIfMissing(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return errUnknownRole
	}
	return err
}

// refuseTakenRoleName refuses a name another live role already carries,
// ignoring case: two roles a picker cannot tell apart are one mistake waiting.
func refuseTakenRoleName(ctx context.Context, tx pgx.Tx, name, exceptKey string) error {
	var taken bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM role WHERE lower(name) = lower($1) AND key <> $2 AND archived_at IS NULL)`,
		name, exceptKey).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return errRoleNameTaken
	}
	return nil
}

// freeRoleKey derives `custom_<slug>` from a name and numbers it past any key
// already used, archived roles included, since a key is never reused.
func freeRoleKey(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	slug := strings.Trim(roleKeyUnsafe.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if slug == "" {
		slug = roleEntity
	}
	const maxSlug = 40
	if len(slug) > maxSlug {
		slug = strings.TrimRight(slug[:maxSlug], "_")
	}
	base := customRoleKeyPrefix + slug
	for n := 1; ; n++ {
		key := base
		if n > 1 {
			key = base + "_" + strconv.Itoa(n)
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM role WHERE key = $1)`, key).Scan(&used); err != nil {
			return "", err
		}
		if !used {
			return key, nil
		}
	}
}
