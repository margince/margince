// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The backfill for project_phase_history.audit_log_id: rows written before the
// column existed get the link to the audit row the same transaction wrote.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PhaseHistoryLinkPage is one inspected page: the history rows looked at and
// how many of them got a link.
type PhaseHistoryLinkPage struct {
	Scanned []ids.UUID
	Linked  int
}

// linkPhaseHistorySQL links a history row only when one audit row matches.
// The pair a transaction writes shares occurred_at, because both columns
// default to now(), the transaction start. The action tells the birth row from
// a move. Two moves in one transaction match two audit rows each, so they stay
// unlinked and nothing is guessed.
const linkPhaseHistorySQL = `
	WITH page AS (
	    SELECT h.id, h.project_id, h.from_phase, h.occurred_at
	      FROM project_phase_history h
	     WHERE h.audit_log_id IS NULL AND h.id > $1
	     ORDER BY h.id
	     LIMIT $2
	), matched AS (
	    SELECT page.id, (array_agg(a.id))[1] AS audit_log_id
	      FROM page
	      JOIN audit_log a
	        ON a.entity_type = 'project' AND a.entity_id = page.project_id
	       AND a.occurred_at = page.occurred_at
	       AND a.action = CASE WHEN page.from_phase IS NULL THEN 'create' ELSE 'advance_phase' END
	     GROUP BY page.id
	    HAVING count(*) = 1
	), linked AS (
	    UPDATE project_phase_history h
	       SET audit_log_id = matched.audit_log_id
	      FROM matched
	     WHERE h.id = matched.id AND h.audit_log_id IS NULL
	    RETURNING h.id
	)
	SELECT page.id, EXISTS (SELECT 1 FROM linked l WHERE l.id = page.id)
	  FROM page ORDER BY page.id`

// LinkPhaseHistoryTx links one page of unlinked history rows, after the given
// id. It writes only audit_log_id; changed_by, reason and occurred_at stay as
// the writer left them.
func LinkPhaseHistoryTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (PhaseHistoryLinkPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return PhaseHistoryLinkPage{}, err
	}
	rows, err := tx.Query(ctx, linkPhaseHistorySQL, after, limit)
	if err != nil {
		return PhaseHistoryLinkPage{}, fmt.Errorf("projects: linking phase history: %w", err)
	}
	defer rows.Close()
	var page PhaseHistoryLinkPage
	for rows.Next() {
		var id ids.UUID
		var linked bool
		if err := rows.Scan(&id, &linked); err != nil {
			return PhaseHistoryLinkPage{}, fmt.Errorf("projects: reading a phase history row: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
		if linked {
			page.Linked++
		}
	}
	if err := rows.Err(); err != nil {
		return PhaseHistoryLinkPage{}, fmt.Errorf("projects: linking phase history: %w", err)
	}
	return page, nil
}

// EstimatePhaseHistoryLinks counts, up to limit+1, the rows LinkPhaseHistoryTx
// inspects: every history row that has no link yet.
func EstimatePhaseHistoryLinks(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM project_phase_history WHERE audit_log_id IS NULL LIMIT $1) unlinked`,
		limit+1).Scan(&count)
	return count, err
}
