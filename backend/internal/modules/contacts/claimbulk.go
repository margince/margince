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
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
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

// ErrClaimNeedsTheUser refuses an agent a promise: whether a human kept their
// word is theirs to say (SettleConversationClaim).
var ErrClaimNeedsTheUser = &httperr.DetailedError{
	Status: http.StatusForbidden, Code: "commitment_needs_the_user",
	Detail: "a commitment is marked done by the user, not by an agent; ask them to mark it done on their Worklist",
}

// LockClaimForBulkTx takes the row lock a bulk change holds for its whole
// transaction, over a claim a Worklist promise row names (ourPromiseWithNoTask).
// Any other claim, or one whose contact or source message the caller may not
// see, answers apperrors.ErrNotFound. Then an agent is refused with
// ErrClaimNeedsTheUser, and only then is write authority asked, so a visible
// promise always gives an agent that one reason.
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
		SELECT c.contact_id, c.source_activity_id, c.status, c.body, c.version FROM conversation_claim c
		 WHERE c.id = $1 AND `+ourPromiseWithNoTask, id).Scan(&contactID, &activityID, &status, &claim.Body, &claim.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkClaim{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BulkClaim{}, fmt.Errorf("lock claim %s for a bulk change: %w", id, err)
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID); err != nil {
		return BulkClaim{}, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activityID); err != nil {
		return BulkClaim{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return BulkClaim{}, ErrClaimNeedsTheUser
		}
		return BulkClaim{}, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, "contact", contactID); err != nil {
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

// VisibleClaims answers which of the named claims the caller may see now,
// through the gates OpenCommitmentsDue reads promises under: the contact and
// activity read grants, a live contact in scope, and a live message whose
// content the caller may read. One query for the whole batch.
func VisibleClaims(ctx context.Context, tx pgx.Tx, named []ids.UUID) (map[ids.UUID]bool, error) {
	out := make(map[ids.UUID]bool, len(named))
	err := auth.Require(ctx, "contact", principal.ActionRead)
	if err == nil {
		err = auth.Require(ctx, "activity", principal.ActionRead)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if len(named) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(named)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if activityScope == "" {
		activityScope = sqlAlwaysVisible
	}
	contactScope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return nil, err
	}
	if contactScope == "" {
		contactScope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id FROM conversation_claim c
		  JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		  JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
		 WHERE c.id = ANY($%d) AND c.archived_at IS NULL AND (%s) AND (%s)`,
		idsPos, activityScope, contactScope), args...)
	if err != nil {
		return nil, fmt.Errorf("read which claims the caller may see: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read which claims the caller may see: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}
