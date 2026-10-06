// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Additive role grants from the corporate directory (OidcGroupRoleMap): at
// federated sign-in the ID token's `groups` claim is intersected with the
// admin's group→role map, and every mapped role the member does not yet hold
// is granted. GRANT-ONLY by design: nothing here deletes or replaces an
// assignment — leaving an IdP group revokes nothing, and revocation stays a
// deliberate admin action (ChangeUserRole, DeactivateUser). No accounts are
// created either: the grant runs strictly after resolveFederatedUser has
// admitted an already-invited member, so an email nobody invited is refused
// exactly as before, whatever groups its token carries.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// roleAuditKey names the before/after role set in the assign audit — the same
// key ChangeUserRole's audit uses, so a reader of the ledger sees one shape
// whether a role was granted at sign-in or set by an admin.
const roleAuditKey = "roles"

// WithGroupRoleMap injects the group→role grant map reader, read fresh per
// federated sign-in so an admin's change takes effect without a restart — the
// same wiring shape as WithRequireSSO. Unset grants nothing, exactly like an
// empty map. The reader takes the login's own transaction: granting from a map
// read outside it would let an entry the admin retires mid-login still hand out
// the role after its removal committed.
func (s *Service) WithGroupRoleMap(fn func(ctx context.Context, tx pgx.Tx) (map[string]string, error)) *Service {
	s.groupRoleMap = fn
	return s
}

// mappedRoleKeys answers which role keys this token's groups grant: the map is
// read only when the token actually carried groups, so the common groupless
// sign-in reads nothing. Keys are deduplicated in the groups' own order — two
// groups mapping onto one role are one grant.
//
// A map read that fails propagates and fails the login rather than reading as
// "no grants", for the reason enforcedSSO gives: a policy outage is neither on
// nor off, and signing a member in without a role an admin deliberately mapped
// would be answering it as off.
func (s *Service) mappedRoleKeys(ctx context.Context, tx pgx.Tx, groups []string) ([]string, error) {
	if len(groups) == 0 || s.groupRoleMap == nil {
		return nil, nil
	}
	roleMap, err := s.groupRoleMap(ctx, tx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(groups))
	var keys []string
	for _, group := range groups {
		roleKey, mapped := roleMap[group]
		if !mapped || seen[roleKey] {
			continue
		}
		seen[roleKey] = true
		keys = append(keys, roleKey)
	}
	return keys, nil
}

// grantMappedRoles inserts every mapped role the member does not hold, inside
// the login's own transaction, and records the change only when a row was
// actually inserted — an unchanged login stays a login and writes no ledger
// entry. A mapped role key the installation no longer defines is skipped with
// a warning naming the stale map entry: the member did nothing wrong, so their
// sign-in must not fail on an admin's leftover.
func (s *Service) grantMappedRoles(ctx context.Context, tx pgx.Tx, userID ids.UserID, roleKeys []string) error {
	if len(roleKeys) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx,
		`SELECT r.key FROM role_assignment ra JOIN role r ON r.id = ra.role_id WHERE ra.user_id = $1`,
		userID)
	if err != nil {
		return fmt.Errorf("identity: read held roles: %w", err)
	}
	before, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("identity: read held roles: %w", err)
	}
	held := make(map[string]bool, len(before))
	for _, key := range before {
		held[key] = true
	}
	var granted []string
	for _, key := range roleKeys {
		if held[key] {
			continue
		}
		roleID, err := roleIDByKey(ctx, tx, key)
		if errors.Is(err, errUnknownRole) {
			slog.WarnContext(ctx, "the group-role map names a role this installation no longer defines; retire the stale entry",
				"role", key)
			continue
		}
		if err != nil {
			return err
		}
		// ON CONFLICT DO NOTHING against uq_role_assignment — NOT the plain
		// INSERT invite and ChangeUserRole use. Those two write into a state
		// they just established (a member created one statement earlier; the
		// assignments deleted one statement earlier), while two concurrent
		// sign-ins can race this insert, and the grant already existing is this
		// path's success, not a conflict to surface.
		tag, err := tx.Exec(ctx,
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			roleID, userID)
		if err != nil {
			return fmt.Errorf("identity: grant mapped role: %w", err)
		}
		if tag.RowsAffected() == 1 {
			granted = append(granted, key)
		}
	}
	if len(granted) == 0 {
		return nil
	}
	return auditMappedGrants(ctx, tx, userID, before, granted)
}

// auditMappedGrants mirrors ChangeUserRole's ledger shape: ONE audit row
// carrying the full before/after role sets, and one role.changed event per
// granted role — the payload names a single to_role, so a sign-in that granted
// two roles is two assignment moves as far as a permission cache is concerned.
// from_role stays absent on every one: nothing was removed, so there is no
// "from", the same absence a multi-role history gets in ChangeUserRole.
//
// The actor is the member whose sign-in triggered the grant — selfActorCtx,
// the same self-attribution the reset cascade uses: there is no admin in the
// room, and the correlation id rides in from the request's own scope.
func auditMappedGrants(ctx context.Context, tx pgx.Tx, userID ids.UserID, before, granted []string) error {
	after := make([]string, 0, len(before)+len(granted))
	after = append(append(after, before...), granted...)
	actorCtx := selfActorCtx(ctx, userID)
	auditID, err := storekit.Audit(actorCtx, tx, "assign", "user", userID.UUID,
		map[string]any{roleAuditKey: before}, map[string]any{roleAuditKey: after})
	if err != nil {
		return err
	}
	for _, key := range granted {
		if err := storekit.EmitEvent(actorCtx, tx, auditID, userID.UUID,
			roleChangedPayload(userID, key, userID, nil)); err != nil {
			return err
		}
	}
	return nil
}
