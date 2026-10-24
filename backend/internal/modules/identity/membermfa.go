// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The admin side of the second factor. A member who lost both their
// authenticator and their recovery codes cannot sign in, and only an admin can
// let them back in. Removing the factor leaves the password alone on guard. So
// the write takes the password link's ceiling, and its ledger names the admin.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditVerbMFARemoved is its own verb because no app_user column changes, and
// an auditor must find every weakened sign-in by it.
const auditVerbMFARemoved = "mfa_removed"

// errOwnFactor sends an admin to DisableMFA for their own factor, which asks
// for a current code: a borrowed admin session must not strip it here.
var errOwnFactor = fmt.Errorf("%w: remove your own second factor in your own settings", apperrors.ErrPermissionDenied)

// RemoveMemberMFA removes another member's confirmed second factor and ends
// every session they hold, in one transaction with its audit row and event.
// The sealed secret is erased after the commit. A member with no confirmed
// factor is a no-op.
func (s *Service) RemoveMemberMFA(ctx context.Context, actor Identity, target ids.UserID) error {
	if !principal.SeatType(actor.SeatType).CanMutate() {
		return apperrors.ErrSeatTierInsufficient
	}
	if target == actor.UserID {
		return errOwnFactor
	}
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionUpdate)
	if err != nil {
		return err
	}
	wsID, err := s.InstallationWorkspace(ctx)
	if err != nil {
		return err
	}
	var ref string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		if err := refuseUnlessCallerOutranksTarget(ctx, tx, actor, target, reachTakeover); err != nil {
			return err
		}
		if err := ensureUserExists(ctx, tx, target); err != nil {
			return err
		}
		if err := refuseWhileHoldingArchivedRole(ctx, tx, target); err != nil {
			return err
		}
		ref, err = removeEnrolment(ctx, tx, actor.UserID, target)
		return err
	})
	if err != nil {
		return fmt.Errorf("identity: remove member mfa: %w", err)
	}
	if ref != "" && s.vault != nil {
		// Detached for DisableMFA's reason: the removal committed, and a failed
		// erase of a blob nothing references can only be logged.
		keyvault.DeleteDetached(ctx, s.vault, slog.Default(), wsID.UUID, keyvault.Ref(ref), "mfa-removed-by-admin")
	}
	return nil
}

// removeEnrolment deletes the target's confirmed factor (its recovery codes
// cascade), ends their sessions, and writes the audit row and event. It
// returns the sealed secret's ref for the caller to erase after the commit, or
// "" when no confirmed factor guarded the account. A pending enrolment guards
// no sign-in, so it is left for the member to restart or finish.
func removeEnrolment(ctx context.Context, tx pgx.Tx, by, target ids.UserID) (string, error) {
	var ref *string
	var ended int
	if err := tx.QueryRow(ctx,
		`WITH removed AS (
		   DELETE FROM user_mfa WHERE user_id = $1 AND confirmed_at IS NOT NULL RETURNING secret_ref
		 ), ended AS (
		   UPDATE session SET revoked_at = now()
		    WHERE user_id = $1 AND revoked_at IS NULL AND EXISTS (SELECT 1 FROM removed)
		   RETURNING 1
		 )
		 SELECT (SELECT secret_ref FROM removed), (SELECT count(*) FROM ended)`,
		target).Scan(&ref, &ended); err != nil || ref == nil {
		return "", err
	}
	auditID, err := storekit.Audit(ctx, tx, auditVerbMFARemoved, "user", target.UUID,
		nil, map[string]any{"sessions_ended": ended})
	if err != nil {
		return "", err
	}
	return *ref, storekit.EmitEvent(ctx, tx, auditID, target.UUID, crmcontracts.PublicEventUserMfaRemoved{
		UserId:        openapi_types.UUID(target.UUID),
		By:            openapi_types.UUID(by.UUID),
		SessionsEnded: ended,
	})
}

// confirmedFactorHolders reports which of these members hold a confirmed
// second factor. A pending enrolment guards no sign-in, so it locks nobody out.
func confirmedFactorHolders(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID]bool, error) {
	return usersMatching(ctx, tx,
		`SELECT user_id FROM user_mfa WHERE user_id = ANY($1) AND confirmed_at IS NOT NULL`, users)
}
