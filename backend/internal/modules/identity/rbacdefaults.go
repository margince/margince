// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The starting grant a unit's objects carry on the system roles.
//
// A unit cannot write a role document, so its objects start with no grant and
// nobody can use the unit until an admin ticks them. The unit declares a
// default per system role instead. It is written once, where the role document
// has no entry for the object. An existing entry is never touched, so an
// admin's revocation holds. Custom roles are never filled.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RbacDefault is one system role's starting grant on one extension object.
// Actions are the four object verbs, spelled as the role document spells them.
type RbacDefault struct {
	Object  RbacObject
	Role    string
	Actions []string
}

// SystemRoleKeys lists the seeded role keys in seed order. The extension
// package mirrors them, and a gate holds the two equal.
func SystemRoleKeys() []string {
	keys := make([]string, 0, len(systemRoles))
	for _, r := range systemRoles {
		keys = append(keys, r.key)
	}
	return keys
}

// grant spells the default as the document does. A verb outside the four is
// refused: it would store a grant nothing evaluates, which reads as configured
// and denies.
func (d RbacDefault) grant() (storedGrant, error) {
	var g storedGrant
	for _, a := range d.Actions {
		switch principal.Action(a) {
		case principal.ActionCreate:
			g.Create = true
		case principal.ActionRead:
			g.Read = true
		case principal.ActionUpdate:
			g.Update = true
		case principal.ActionDelete:
			g.Delete = true
		default:
			return storedGrant{}, fmt.Errorf("identity: default grant for role %q on %q names action %q, which is not one of create, read, update, delete", d.Role, d.Object, a)
		}
	}
	return g, nil
}

// ValidateRbacDefaults checks each default for an extension object name, a
// system role key and the four verbs only. It is pure, so the boot runs it
// before applying anything.
func ValidateRbacDefaults(defaults ...RbacDefault) error {
	type pair struct{ object, role string }
	seen := map[pair]bool{}
	for _, d := range defaults {
		if err := d.Object.Validate(); err != nil {
			return err
		}
		if !isSystemRoleKey(d.Role) {
			return fmt.Errorf("identity: default grant on %q names role %q, which is not a system role (a custom role starts with no grant)", d.Object, d.Role)
		}
		if _, err := d.grant(); err != nil {
			return err
		}
		k := pair{string(d.Object), d.Role}
		if seen[k] {
			return fmt.Errorf("identity: default grant on %q lists role %q twice", d.Object, d.Role)
		}
		seen[k] = true
	}
	return nil
}

// registeredDefaults holds the defaults this process registered. Two writers
// read it: the seed of a new installation and the boot fill of an old one.
var registeredDefaults struct {
	mu   sync.RWMutex
	list []RbacDefault
}

// RegisterRbacDefaults records the defaults the composed set declares. The boot
// calls it once after RegisterRbacObjects, and it applies all or none. Each
// object must already be registered, because a role document drops a grant on
// an object it cannot name.
func RegisterRbacDefaults(defaults ...RbacDefault) error {
	if err := ValidateRbacDefaults(defaults...); err != nil {
		return err
	}
	for _, d := range defaults {
		if !policy.IsRegisteredObject(string(d.Object)) {
			return fmt.Errorf("identity: default grant names %q, which no unit registered", d.Object)
		}
	}
	registeredDefaults.mu.Lock()
	defer registeredDefaults.mu.Unlock()
	registeredDefaults.list = slices.Clone(defaults)
	return nil
}

func registeredRbacDefaults() []RbacDefault {
	registeredDefaults.mu.RLock()
	defer registeredDefaults.mu.RUnlock()
	return slices.Clone(registeredDefaults.list)
}

// FillRbacDefaults writes each registered default into the system role that has
// no entry for the object. Each write gets one audit row, and the count of
// writes is returned. A second boot finds every entry present and writes
// nothing. The caller binds the workspace and a system actor.
//
// It takes the lock every authorization write takes, so it cannot interleave
// with an admin editing the document, and two booting processes cannot both write.
func (s *Service) FillRbacDefaults(ctx context.Context) (int, error) {
	// Boot's alone. A user edits a grant through SetRoleObjectGrant, which is
	// gated, and this writes under no user's authority.
	if p, ok := principal.Actor(ctx); !ok || p.Type != principal.PrincipalSystem {
		return 0, apperrors.ErrPermissionDenied
	}
	defaults := registeredRbacDefaults()
	if len(defaults) == 0 {
		return 0, nil
	}
	filled := 0
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		filled = 0
		for _, d := range defaults {
			roleID, wrote, err := fillRoleObjectDefault(ctx, tx, d)
			if err != nil {
				return err
			}
			if !wrote {
				continue
			}
			g, err := d.grant()
			if err != nil {
				return err
			}
			if _, err := storekit.Audit(ctx, tx, "update", roleEntity, roleID,
				roleObjectsImage(nil), roleObjectsImage(map[string]storedGrant{string(d.Object): g})); err != nil {
				return err
			}
			filled++
		}
		return nil
	})
	return filled, err
}

// roleObjectsImage is the audit image of the grants a fill changed. The empty
// before-image is a non-nil map, because an update must carry one.
func roleObjectsImage(grants map[string]storedGrant) map[string]any {
	if grants == nil {
		grants = map[string]storedGrant{}
	}
	return map[string]any{"objects": grants}
}

// fillRoleObjectDefault writes the default into the role's document when it has
// no entry for the object, and says whether it wrote. It uses the grant
// editor's targeted write (roleObjectGrantExpr), so unknown grants stay.
//
// The entry's presence is the whole test. A grant an admin turned off is still
// an entry, because all four verbs are stored, so a revocation is not refilled.
func fillRoleObjectDefault(ctx context.Context, tx pgx.Tx, d RbacDefault) (roleID ids.UUID, wrote bool, err error) {
	g, err := d.grant()
	if err != nil {
		return ids.Nil, false, err
	}
	encoded, err := json.Marshal(g)
	if err != nil {
		return ids.Nil, false, err
	}
	// Read first and write only what is missing. Any UPDATE on role fires a
	// statement trigger that bumps the report fence, even when no row matches.
	// A boot that fills nothing must therefore send none.
	var version int64
	err = tx.QueryRow(ctx,
		`SELECT id, version FROM role
		  WHERE key = $1 AND is_system AND archived_at IS NULL
		    AND NOT COALESCE(jsonb_exists(permissions->'objects', $2::text), false)`,
		d.Role, string(d.Object)).Scan(&roleID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE role SET permissions = `+roleObjectGrantExpr+` WHERE id = $1 AND version = $4`,
		roleID, string(d.Object), encoded, version)
	if err != nil {
		return ids.Nil, false, err
	}
	if tag.RowsAffected() != 1 {
		return ids.Nil, false, apperrors.ErrVersionSkew
	}
	return roleID, true, nil
}

// fillRegisteredDefaults gives a freshly seeded role set the registered
// defaults, so a new installation starts with them. It writes no audit row,
// because the seed creates the role documents unaudited and these entries are
// part of that creation.
func fillRegisteredDefaults(ctx context.Context, tx pgx.Tx) error {
	for _, d := range registeredRbacDefaults() {
		if _, _, err := fillRoleObjectDefault(ctx, tx, d); err != nil {
			return err
		}
	}
	return nil
}
