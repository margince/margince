// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// A colleague's own open request, read and withdrawn by that colleague or their
// agent, whichever staged it. The inbox shows a payload-owned card only to its
// decider, so the one who asked reaches their request here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownRequestKinds are the kinds a user or their agent stages for a colleague to answer.
var ownRequestKinds = map[string]bool{approvalsubject.KindOwnershipTransfer: true}

// OwnRequest is one open request the caller or their agent staged.
type OwnRequest struct {
	ID             ids.ApprovalID
	ProposedChange json.RawMessage
	ExpiresAt      time.Time
}

// OwnRequestForTargetTx answers the caller's open request of kind on one
// record, or apperrors.ErrNotFound when they have none.
func OwnRequestForTargetTx(ctx context.Context, tx pgx.Tx, kind, targetType string, targetID ids.UUID) (OwnRequest, error) {
	stager, err := ownRequestStager(ctx, kind)
	if err != nil {
		return OwnRequest{}, err
	}
	var out OwnRequest
	err = tx.QueryRow(ctx, `SELECT id, proposed_change, expires_at FROM approval
		 WHERE kind = $1 AND target_entity_type = $2 AND target_entity_id = $3
		   AND (proposed_by = $4 OR (on_behalf_of = $5 AND passport_id IS NOT NULL))
		   AND status = 'pending' AND expires_at > now()
		 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`,
		kind, targetType, targetID, principal.HumanIDPrefix+stager.String(), stager).Scan(&out.ID, &out.ProposedChange, &out.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OwnRequest{}, apperrors.ErrNotFound
	}
	if err != nil {
		return OwnRequest{}, fmt.Errorf("crmapprovals: reading the caller's own request: %w", err)
	}
	out.ExpiresAt = out.ExpiresAt.UTC()
	return out, nil
}

// WithdrawOwnRequestTx withdraws the caller's open request of kind on one
// record and answers which it was. apperrors.ErrNotFound means there was none.
func WithdrawOwnRequestTx(ctx context.Context, tx pgx.Tx, kind, targetType string, targetID ids.UUID, reason string) (ids.ApprovalID, error) {
	own, err := OwnRequestForTargetTx(ctx, tx, kind, targetType, targetID)
	if err != nil {
		return ids.ApprovalID{}, err
	}
	withdrawn, err := withdrawInTx(ctx, tx, own.ID, reason)
	if err != nil {
		return ids.ApprovalID{}, err
	}
	if !withdrawn {
		return ids.ApprovalID{}, apperrors.ErrNotFound
	}
	return own.ID, nil
}

// ownRequestStager is the colleague whose request this is. A user's request
// names them in proposed_by, and their agent's names them in on_behalf_of.
func ownRequestStager(ctx context.Context, kind string) (ids.UUID, error) {
	if !ownRequestKinds[kind] {
		return ids.Nil, fmt.Errorf("crmapprovals: %q is not a request a colleague stages", kind)
	}
	p, ok := principal.Actor(ctx)
	if !ok || !p.HumanOrItsAgent() || p.UserID.IsZero() {
		return ids.Nil, fmt.Errorf("crmapprovals: a colleague's own request is read by that colleague or their agent: %w",
			apperrors.ErrPermissionDenied)
	}
	return p.UserID, nil
}

// OpenRequestsForTargetTx answers every open request of kind on one record,
// whoever staged it, locked so a concurrent decision waits for the caller.
// The caller has already passed the record's own gate for the request it files.
func OpenRequestsForTargetTx(ctx context.Context, tx pgx.Tx, kind, targetType string, targetID ids.UUID) ([]OwnRequest, error) {
	if !ownRequestKinds[kind] {
		return nil, fmt.Errorf("crmapprovals: %q is not a request a colleague stages", kind)
	}
	rows, err := tx.Query(ctx, `SELECT id, proposed_change, expires_at FROM approval
		 WHERE kind = $1 AND target_entity_type = $2 AND target_entity_id = $3
		   AND status = 'pending' AND expires_at > now()
		 `+lockOrder+` FOR UPDATE`, kind, targetType, targetID)
	if err != nil {
		return nil, fmt.Errorf("crmapprovals: reading the open requests on a record: %w", err)
	}
	open, err := pgx.CollectRows(rows, pgx.RowToStructByPos[OwnRequest])
	if err != nil {
		return nil, fmt.Errorf("crmapprovals: reading the open requests on a record: %w", err)
	}
	return open, nil
}
