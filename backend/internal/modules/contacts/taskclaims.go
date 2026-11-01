// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TaskClaim retains recipient identity independently of task completion.
type TaskClaim struct {
	ClaimOnTask
	Task        *ids.UUID
	Locator     string
	DisplayName string
	Status      string
}

// ClaimsOnTasks applies both task and contact visibility before returning recipients.
func (s *Store) ClaimsOnTasks(ctx context.Context, tasks []ids.UUID) ([]TaskClaim, error) {
	return s.readTaskClaims(ctx, tasks, "c.task_activity_id")
}

// ClaimsByIDs preserves the same visibility rules when a proposal names recipients.
func (s *Store) ClaimsByIDs(ctx context.Context, claims []ids.UUID) ([]TaskClaim, error) {
	return s.readTaskClaims(ctx, claims, "c.id")
}

func (s *Store) readTaskClaims(ctx context.Context, named []ids.UUID, identity string) ([]TaskClaim, error) {
	query, args, err := taskClaimsStatement(ctx, named, identity)
	if err != nil || len(named) == 0 {
		return nil, err
	}
	var out []TaskClaim
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("read task recipient claims: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskClaim, error) {
			var claim TaskClaim
			err := row.Scan(&claim.ID, &claim.Contact, &claim.Source, &claim.Task, &claim.DisplayName, &claim.Status, &claim.Locator)
			return claim, err
		})
		return err
	})
	return out, err
}

// taskClaimsStatement reads the claims whose identity column holds one of the
// named ids, inside the reader's contact, evidence and task visibility. The
// content gate runs once, over the claims' sources and tasks together.
func taskClaimsStatement(ctx context.Context, named []ids.UUID, identity string) (string, []any, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return "", nil, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return "", nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	taskPos := arg(named)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return "", nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	readable, err := auth.ContentReadableActivitiesSQL(ctx, `SELECT c.source_activity_id FROM task_claims c
 UNION ALL SELECT c.task_activity_id FROM task_claims c WHERE c.task_activity_id IS NOT NULL`, arg)
	if err != nil {
		return "", nil, err
	}
	renewal, err := auth.CommitmentEvidenceClause(ctx, "c.source_activity_id", "c.evidence_fingerprint", arg)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf(`
 WITH task_claims AS MATERIALIZED (
   SELECT c.* FROM conversation_claim c WHERE %s = ANY($%d) AND c.archived_at IS NULL),
 readable_claim_activity AS MATERIALIZED (%s)
 SELECT c.id, c.contact_id, c.source_activity_id, c.task_activity_id, pr.full_name, c.status, c.evidence_fingerprint
 FROM task_claims c
 JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
 JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
 LEFT JOIN activity task ON task.id = c.task_activity_id AND task.archived_at IS NULL
 WHERE (%s) AND a.id IN (SELECT id FROM readable_claim_activity) AND (%s)
 AND (c.task_activity_id IS NULL OR task.id IN (SELECT id FROM readable_claim_activity))
 ORDER BY c.task_activity_id, pr.full_name, c.id`, identity, taskPos, readable, scope, renewal), args, nil
}
