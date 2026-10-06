// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A bulk change marking Worklist promises done reaches each claim here, and
// settles it through moveClaimStatus, the write POST /claims/{id}/settle makes.
// Its undo opens the claim again through the same write.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// claimStatusDone is the status a kept promise settles to, as the settle
// request spells it.
const claimStatusDone = string(crmcontracts.SettleClaimRequestOutcomeDone)

// BulkClaim is one claim as a bulk change found it under its row lock.
type BulkClaim struct {
	Body    string
	Version int64
	Done    bool
}

// LockClaimForBulkTx takes the row lock a bulk change holds for its whole
// transaction. A claim that is gone, or whose contact or source message the
// caller may not see, answers apperrors.ErrNotFound.
func (s *Store) LockClaimForBulkTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (BulkClaim, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return BulkClaim{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, "conversation_claim", id, storekit.LiveOnly); err != nil {
		return BulkClaim{}, err
	}
	var claim BulkClaim
	var contactID, activityID ids.UUID
	var status string
	err := tx.QueryRow(ctx, `
		SELECT contact_id, source_activity_id, status, body, version FROM conversation_claim
		 WHERE id = $1`, id).Scan(&contactID, &activityID, &status, &claim.Body, &claim.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkClaim{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BulkClaim{}, fmt.Errorf("lock claim %s for a bulk change: %w", id, err)
	}
	// The settlement's own gates, asked under the lock and before the version
	// is compared, so a claim the caller may not settle is never reported as
	// having moved.
	if err := auth.EnsureWritableLive(ctx, tx, "contact", contactID); err != nil {
		return BulkClaim{}, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activityID); err != nil {
		return BulkClaim{}, err
	}
	claim.Done = status == claimStatusDone
	return claim, nil
}

// SetClaimDoneTx settles one open claim as done, or opens a done one again,
// under the gates SettleConversationClaim holds, and answers the version the
// write left it at.
func (s *Store) SetClaimDoneTx(ctx context.Context, tx pgx.Tx, id ids.UUID, done bool) (int64, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return 0, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return 0, err
	}
	from, to := claimStatusOpen, claimStatusDone
	if !done {
		from, to = to, from
	}
	if err := moveClaimStatus(ctx, tx, id, from, to); err != nil {
		return 0, err
	}
	var version int64
	if err := tx.QueryRow(ctx, `SELECT version FROM conversation_claim WHERE id = $1`, id).Scan(&version); err != nil {
		return 0, fmt.Errorf("read back claim %s: %w", id, err)
	}
	return version, nil
}

// VisibleClaims answers which of the named claims the caller may see now:
// the contact and activity read grants OpenCommitmentsDue asks, their contact
// in scope, and the message they quote readable.
func VisibleClaims(ctx context.Context, tx pgx.Tx, named []ids.UUID) (map[ids.UUID]bool, error) {
	out := make(map[ids.UUID]bool, len(named))
	for _, object := range []string{"contact", "activity"} {
		err := auth.Require(ctx, object, principal.ActionRead)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
	for _, id := range named {
		var contactID, activityID ids.UUID
		err := tx.QueryRow(ctx, `
			SELECT contact_id, source_activity_id FROM conversation_claim
			 WHERE id = $1 AND archived_at IS NULL`, id).Scan(&contactID, &activityID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read claim %s: %w", id, err)
		}
		seen, err := claimSeen(ctx, tx, contactID, activityID)
		if err != nil {
			return nil, err
		}
		out[id] = seen
	}
	return out, nil
}

// claimSeen reduces the two gates a claim is read through to a verdict; only a
// not-found is one.
func claimSeen(ctx context.Context, tx pgx.Tx, contactID, activityID ids.UUID) (bool, error) {
	err := auth.EnsureReadable(ctx, tx, "contact", contactID)
	if err == nil {
		err = auth.EnsureActivityContentVisibleLive(ctx, tx, activityID)
	}
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}
