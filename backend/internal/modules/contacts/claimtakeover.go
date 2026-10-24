// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Taking over a promise one of us made that nobody was asked to keep. The
// caller in compose writes the task. This file is the claim's half. It finds
// the claims the taking moves and records who took them over and when.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PromiseToTake is one promise as a colleague is about to take it over.
type PromiseToTake struct {
	Claim   ids.UUID
	Contact ids.UUID
	Source  ids.UUID
	// Locator is the promise's evidence identity, which keys its one task.
	Locator     string
	Body        string
	SourceLabel *string
	DueAt       *time.Time
	Current     bool
	NeedsReview bool
	Kind        string
	Status      string
	Task        *ids.UUID
	TakenBy     *ids.UUID
	TakerName   *string
	TakenAt     *time.Time
	// Claims lists the open claims of this promise with no owner yet, the
	// clicked one included, that the caller may read.
	Claims []ids.UUID
}

// PromiseToTakeTx reads the promise behind one claim for a take over, locked
// against a second taker until the caller's transaction ends.
//
// A claim on a contact the caller may not read answers ErrNotFound. So does
// one quoting a source they may not read, as on every other claim read.
func (s *Store) PromiseToTakeTx(ctx context.Context, tx pgx.Tx, claimID ids.UUID) (PromiseToTake, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return PromiseToTake{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return PromiseToTake{}, err
	}
	out := PromiseToTake{Claim: claimID}
	err := tx.QueryRow(ctx, `SELECT contact_id, source_activity_id, evidence_fingerprint
 FROM conversation_claim WHERE id = $1 AND archived_at IS NULL`, claimID).Scan(&out.Contact, &out.Source, &out.Locator)
	if errors.Is(err, pgx.ErrNoRows) {
		return PromiseToTake{}, apperrors.ErrNotFound
	}
	if err != nil {
		return PromiseToTake{}, fmt.Errorf("read the promise to take over: %w", err)
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "contact", out.Contact); err != nil {
		return PromiseToTake{}, err
	}
	if err := ensureClaimContentVisible(ctx, tx, claimID); err != nil {
		return PromiseToTake{}, err
	}
	// One promise, one taker: two colleagues pressing at once, on the same
	// claim or on the copy filed on another contact, take turns here.
	if err := storekit.LockWriteIdentity(ctx, tx, "promise_take_over", out.Source.String()+":"+out.Locator); err != nil {
		return PromiseToTake{}, err
	}
	if err := s.readPromiseState(ctx, tx, &out); err != nil {
		return PromiseToTake{}, err
	}
	claims, err := s.ownerlessCopies(ctx, tx, out)
	if err != nil {
		return PromiseToTake{}, err
	}
	out.Claims = claims
	return out, nil
}

// readPromiseState reads the clicked claim after the lock, so a taker who
// waited sees what the first one wrote.
func (s *Store) readPromiseState(ctx context.Context, tx pgx.Tx, p *PromiseToTake) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	claimPos := arg(p.Claim)
	current := currentCommitmentAdmission(s.clock(), arg)
	content, err := claimContentClause(ctx, arg)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, fmt.Sprintf(`
 SELECT c.kind, c.status, c.needs_review, (%s), c.body, a.subject, c.due_at,
        c.task_activity_id, c.taken_by_user_id, taker.display_name, c.taken_at
   FROM conversation_claim c
   JOIN activity a ON a.id = c.source_activity_id
   LEFT JOIN app_user taker ON taker.id = c.taken_by_user_id
  WHERE c.id = $%d AND c.archived_at IS NULL AND a.archived_at IS NULL AND %s
    FOR UPDATE OF c`, current, claimPos, content), args...).Scan(
		&p.Kind, &p.Status, &p.NeedsReview, &p.Current, &p.Body, &p.SourceLabel, &p.DueAt,
		&p.Task, &p.TakenBy, &p.TakerName, &p.TakenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read the promise to take over: %w", err)
	}
	return nil
}

// ownerlessCopies lists the open claims of this promise, on every contact it
// was filed on, that nobody holds yet and the caller may read.
func (s *Store) ownerlessCopies(ctx context.Context, tx pgx.Tx, p PromiseToTake) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sourcePos, locatorPos := arg(p.Source), arg(p.Locator)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
 SELECT c.id FROM conversation_claim c
   JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
  WHERE c.source_activity_id = $%d AND c.evidence_fingerprint = $%d
    AND c.kind = 'commitment_ours' AND c.status = 'open' AND c.archived_at IS NULL
    AND c.task_activity_id IS NULL AND c.taken_by_user_id IS NULL AND (%s)
  ORDER BY c.id
    FOR UPDATE OF c`, sourcePos, locatorPos, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("read the copies of the promise: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// RecordPromiseTakenTx points every claim the take over moves at the taker's
// task and names the taker, with one audit row and one changed event each.
func (s *Store) RecordPromiseTakenTx(ctx context.Context, tx pgx.Tx, p PromiseToTake, taskID, taker ids.UUID, at time.Time) error {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
 UPDATE conversation_claim SET task_activity_id = $2, taken_by_user_id = $3, taken_at = $4
  WHERE id = ANY($1) AND task_activity_id IS NULL AND taken_by_user_id IS NULL
 RETURNING id, contact_id`, p.Claims, taskID, taker, at)
	if err != nil {
		return fmt.Errorf("record who took over the promise: %w", err)
	}
	type moved struct{ claim, contact ids.UUID }
	taken, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (moved, error) {
		var m moved
		return m, row.Scan(&m.claim, &m.contact)
	})
	if err != nil {
		return fmt.Errorf("record who took over the promise: %w", err)
	}
	if len(taken) != len(p.Claims) {
		return fmt.Errorf("the promise changed while it was taken over: %w", apperrors.ErrConflict)
	}
	for _, m := range taken {
		before := map[string]any{claimIDKey: m.claim.String(), claimTaskKey: nil, "taken_by_user_id": nil}
		after := map[string]any{
			claimIDKey: m.claim.String(), claimTaskKey: taskID.String(),
			"taken_by_user_id": taker.String(), "taken_at": at,
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "contact", m.contact, before, after)
		if err != nil {
			return fmt.Errorf("audit the take over: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, m.contact,
			crmcontracts.PublicEventConversationClaimChanged{ClaimId: openapi_types.UUID(m.claim), Status: claimStatusOpen}); err != nil {
			return fmt.Errorf("emit conversation_claim.changed: %w", err)
		}
	}
	return nil
}
