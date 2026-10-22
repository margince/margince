// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClaimOnTask is one open claim a task stands for, and the contact it is filed
// on, which is the record settling it writes.
type ClaimOnTask struct {
	ID      ids.UUID
	Contact ids.UUID
	// Source is the conversation the claim was quoted from.
	Source ids.UUID
}

// OpenClaimsOnTask lists the open claims a task stands for. A claim and the
// task an extracted commitment became settle together, and this is the claim
// half of that question.
func (s *Store) OpenClaimsOnTask(ctx context.Context, taskID ids.UUID) ([]ClaimOnTask, error) {
	claims, err := s.ClaimsOnTasks(ctx, []ids.UUID{taskID})
	if err != nil {
		return nil, err
	}
	var open []ClaimOnTask
	for _, claim := range claims {
		if claim.Status == "open" {
			open = append(open, claim.ClaimOnTask)
		}
	}
	return open, nil
}

// ClaimSettlement binds replay to the source identity and original settlement time.
type ClaimSettlement struct {
	TaskID   *ids.UUID
	Status   string
	At       time.Time
	SourceID ids.UUID
	Locator  string
}

// ReadClaimSettlement uses the audit time so later edits cannot renew completion.
func (s *Store) ReadClaimSettlement(ctx context.Context, claimID, settlement ids.UUID) (*ClaimSettlement, error) {
	var out *ClaimSettlement
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.ReadClaimSettlementTx(ctx, tx, claimID, settlement)
		return err
	})
	return out, err
}

// ReadClaimSettlementTx locks the claim while its settlement retires related work.
//
//nolint:nilnil // A missing audit-linked claim has no settlement to apply.
func (s *Store) ReadClaimSettlementTx(ctx context.Context, tx pgx.Tx, claimID, settlement ids.UUID) (*ClaimSettlement, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	claimPos, auditPos := arg(claimID), arg(settlement)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	out := &ClaimSettlement{}
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT c.task_activity_id, c.status, a.occurred_at, c.source_activity_id, c.evidence_fingerprint
		  FROM conversation_claim c
		  JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
		  JOIN audit_log a ON a.id = $%d AND a.after->>'%s' = c.id::text
		 WHERE c.id = $%d AND c.archived_at IS NULL AND (%s)
		   FOR UPDATE OF c`, auditPos, claimIDKey, claimPos, scope), args...).Scan(&out.TaskID, &out.Status, &out.At, &out.SourceID, &out.Locator)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the claim settlement: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, out.SourceID); err != nil {
		return nil, err
	}
	return out, nil
}

// TaskClaimsSettledTx checks all recipients without exposing hidden claims.
// Omitting a recipient outside the caller's scope could incorrectly complete their work.
func (s *Store) TaskClaimsSettledTx(ctx context.Context, tx pgx.Tx, taskID ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, taskID); err != nil {
		return false, err
	}
	args := []any{taskID}
	return claimsSettledTx(ctx, tx, fmt.Sprintf("task_activity_id = $%d", len(args)), args)
}

// PromiseClaimsSettledTx also covers recipient claims whose task is still awaiting approval.
func (s *Store) PromiseClaimsSettledTx(ctx context.Context, tx pgx.Tx, sourceID ids.UUID, locator string) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, sourceID); err != nil {
		return false, err
	}
	args := []any{sourceID}
	where := fmt.Sprintf("source_activity_id = $%d", len(args))
	args = append(args, locator)
	where += fmt.Sprintf(" AND evidence_fingerprint = $%d", len(args))
	return claimsSettledTx(ctx, tx, where, args)
}

func claimsSettledTx(ctx context.Context, tx pgx.Tx, where string, args []any) (bool, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return false, err
	}
	var settled bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM conversation_claim
 WHERE `+where+` AND status = 'open' AND archived_at IS NULL)`, args...).Scan(&settled)
	if err != nil {
		return false, fmt.Errorf("read whether recipient obligations are settled: %w", err)
	}
	return settled, nil
}
