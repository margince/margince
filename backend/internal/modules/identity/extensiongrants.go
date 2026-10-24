// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// GrantExtensionObjects gives the seeded admin role full access to each given
// object and returns the objects it granted. Boot runs it as the system actor
// with no signed-in admin, so it takes no RBAC gate.
//
// An object the role already has an entry for is left alone, so an admin's
// narrowing survives the next upgrade. Each grant goes through the role
// editor's own writer, so it leaves the same audit row.
func (s *Service) GrantExtensionObjects(ctx context.Context, objects []string) ([]string, error) {
	installer := Identity{Roles: []string{roleAdmin}}
	full := storedGrant{Create: true, Read: true, Update: true, Delete: true}
	var granted []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRow(ctx,
			`SELECT permissions FROM role WHERE key = $1 AND archived_at IS NULL`, roleAdmin).Scan(&raw); err != nil {
			return fmt.Errorf("identity: reading the admin role: %w", err)
		}
		held, err := decodeRoleObjects(raw)
		if err != nil {
			return err
		}
		for _, object := range slices.Sorted(slices.Values(objects)) {
			if _, has := held[object]; has {
				continue
			}
			if _, err := applyRoleObjectGrant(ctx, tx, installer, roleAdmin, object, full, nil); err != nil {
				return err
			}
			granted = append(granted, object)
		}
		return nil
	})
	return granted, err
}
