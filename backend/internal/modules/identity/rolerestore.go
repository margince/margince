// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var errNotSystemRole = fmt.Errorf("%w: only a role the product ships has shipped grants", apperrors.ErrConflict)

// shippedObjects is the grant map a system role starts with on a new
// installation. It holds the seeded core grants and each registered extension
// default for that role. The seed writes the same set, so a restore ends where
// a new installation begins.
func shippedObjects(roleKey string) map[string]storedGrant {
	core, _ := policy.ShippedGrants(roleKey)
	out := make(map[string]storedGrant, len(core))
	for object, g := range core {
		out[object] = storedGrant(g)
	}
	for _, d := range registeredRbacDefaults() {
		if d.Role != roleKey {
			continue
		}
		// Validated when registered, so the grant always spells.
		if g, err := d.grant(); err == nil {
			out[string(d.Object)] = g
		}
	}
	return out
}

// RestoreRoleGrants puts a system role's grants back to the ones the product
// ships, and returns the role as stored afterwards. Its row scope, name and
// field masks stay as they are.
//
// Turning rights on reaches every holder at once, so a restore that widens the
// role is admin only, like any other widening.
func (s *Service) RestoreRoleGrants(ctx context.Context, actor Identity, roleKey string, ifVersion *int64) (roleRow, error) {
	ctx, err := admit(ctx, actor, objectRoleAdmin, principal.ActionUpdate)
	if err != nil {
		return roleRow{}, err
	}
	var restored roleRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var err error
		if restored, err = restoreShippedGrants(ctx, tx, actor, roleKey, ifVersion); err != nil {
			return err
		}
		return s.recheckRoleHoldersTx(ctx, tx, roleKey)
	})
	if err != nil {
		return roleRow{}, err
	}
	return restored, nil
}

// restoreShippedGrants locks the role, checks If-Match and the caller's
// authority, replaces the whole grant map and audits what moved. Objects the
// installation cannot name are replaced too: they are not part of what shipped.
func restoreShippedGrants(ctx context.Context, tx pgx.Tx, actor Identity, roleKey string, ifVersion *int64) (roleRow, error) {
	var roleID ids.UUID
	var version int64
	var isSystem bool
	var rawBefore []byte
	err := tx.QueryRow(ctx,
		`SELECT id, version, is_system, permissions FROM role WHERE key = $1 AND archived_at IS NULL FOR UPDATE`,
		roleKey).Scan(&roleID, &version, &isSystem, &rawBefore)
	if errors.Is(err, pgx.ErrNoRows) {
		return roleRow{}, errUnknownRole
	}
	if err != nil {
		return roleRow{}, err
	}
	if !isSystem || !isSystemRoleKey(roleKey) {
		return roleRow{}, errNotSystemRole
	}
	if ifVersion != nil && *ifVersion != version {
		return roleRow{}, apperrors.ErrVersionSkew
	}
	before, err := decodeRoleObjects(rawBefore)
	if err != nil {
		return roleRow{}, err
	}
	shipped := shippedObjects(roleKey)
	widens := false
	for object, g := range shipped {
		widens = widens || grantAdded(before[object], g) != storedGrant{}
	}
	if err := refuseWideningUnlessAdmin(actor, widens); err != nil {
		return roleRow{}, err
	}
	encoded, err := json.Marshal(shipped)
	if err != nil {
		return roleRow{}, err
	}
	restored, err := scanRoleRow(tx.QueryRow(ctx,
		`UPDATE role
		    SET permissions = jsonb_set(permissions, '{objects}', $2::jsonb, true)
		  WHERE id = $1
		  RETURNING `+roleColumns, roleID, encoded))
	if err != nil {
		return roleRow{}, err
	}
	was, now := changedGrants(before, shipped)
	if _, err := storekit.Audit(ctx, tx, "update", roleEntity, roleID,
		roleObjectsImage(was), roleObjectsImage(now)); err != nil {
		return roleRow{}, err
	}
	return restored, nil
}

// changedGrants names only the objects whose grant differs, so the audit row
// shows what the restore moved and not the whole document. An object the
// restore drops reads as the zero grant afterwards.
func changedGrants(before, after map[string]storedGrant) (was, now map[string]storedGrant) {
	was, now = map[string]storedGrant{}, map[string]storedGrant{}
	for _, object := range slicesUnion(before, after) {
		if before[object] != after[object] {
			was[object], now[object] = before[object], after[object]
		}
	}
	return was, now
}

func slicesUnion(a, b map[string]storedGrant) []string {
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	return slices.Sorted(maps.Keys(keys))
}
