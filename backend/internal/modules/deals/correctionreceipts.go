// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the sweep changed on a rep's own deals last night, for the morning's
// "handled for you" lane.
//
// The corrections are applied when they are made — there is no card, and no
// approval row to read them back from — so this is the only telling a rep gets.
// It carries the before and the after, the reason, and the audit row an Undo
// puts back.

package deals

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CorrectionReceipt is one applied correction as a reader sees it.
type CorrectionReceipt struct {
	DealID   ids.DealID
	DealName string
	// AuditLogID is what an Undo names: the record-history restore route reads
	// the before-image from it, which is why nothing here copies those values
	// into a second place.
	AuditLogID ids.UUID
	// Fields the correction moved, so the sentence can name them all rather
	// than the one the reader happens to notice.
	Fields    []string
	Basis     string
	AppliedAt time.Time
	// Reversed reports that somebody already put this back. The row stays on
	// the lane saying so, rather than vanishing and leaving the reader
	// wondering whether their Undo landed.
	Reversed bool
	Version  int64
}

// RecentCorrectionsOwnedBy reads what the sweep changed on THIS reader's deals
// since an instant, newest first.
//
// Narrowed to their own deals, not to every deal they may see. A manager who
// can read the whole pipeline does not want every rep's overnight hygiene in
// their morning — the receipt answers "what happened to my work while I was
// away", and a lane that answered it for the workspace would be unreadable on
// any installation with more than a few reps.
func (s *Store) RecentCorrectionsOwnedBy(
	ctx context.Context, since time.Time, limit int,
) ([]CorrectionReceipt, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		// No seat, no "my deals" to answer for. An empty lane rather than a
		// refusal: a background assembly with no reader is a real caller.
		return nil, nil
	}
	var out []CorrectionReceipt
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		sincePos := arg(since)
		ownerPos := arg(actor.UserID)
		limitPos := arg(limit)
		// The reader's own row scope, ON TOP of the owner narrowing above.
		//
		// The two answer different questions and neither implies the other: the
		// owner clause is the product rule (a receipt is about MY deals), and
		// the scope clause is the permission one (which deals this seat may see
		// at all). A seat whose scope was narrowed below its own deals must not
		// read them back through this lane, and asking only the product
		// question would let it.
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		query := storekit.SQLf(`
			SELECT c.deal_id, d.name, c.audit_log_id, c.fields,
			       -- The reason, only for the seat whose grants composed it.
			       --
			       -- The sentence can name a contact and a correspondence date,
			       -- read under the owner's permissions on the night it was
			       -- written, and it is stored text by the time anybody reads it
			       -- back. A deal that has changed hands since shows the change
			       -- with no reason rather than disclosing a name this reader was
			       -- never entitled to. Compared in SQL so no row leaves the
			       -- database carrying a sentence its reader may not have.
			       --
			       -- IS NOT DISTINCT FROM, never a bare equality. An unowned deal
			       -- records the empty string and owner_id is NULL, and equality
			       -- against NULL is NULL — so that test would withhold the reason on
			       -- every unowned deal, whose sentence names nobody and was
			       -- composed under no seat's grants at all.
			       CASE WHEN nullif(a.evidence->>'basis_owner', '') IS NOT DISTINCT FROM d.owner_id::text
			            THEN coalesce(a.evidence->>'basis', '')
			            ELSE '' END,
			       c.applied_at,
			       c.reversed_at IS NOT NULL, d.version
			  FROM deal_correction c
			  JOIN deal d ON d.id = c.deal_id
			  JOIN audit_log a ON a.id = c.audit_log_id
			 WHERE c.applied_at >= $%d
			   AND d.owner_id = $%d
			   AND d.archived_at IS NULL`, sincePos, ownerPos)
		if scope != "" {
			query += storekit.SQLf(" AND %s", scope)
		}
		query += storekit.SQLf(" ORDER BY c.applied_at DESC LIMIT $%d", limitPos)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("deals: reading last night's corrections: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row CorrectionReceipt
			if err := rows.Scan(&row.DealID, &row.DealName, &row.AuditLogID,
				&row.Fields, &row.Basis, &row.AppliedAt, &row.Reversed,
				&row.Version); err != nil {
				return err
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
