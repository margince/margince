// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// QualifyReminderSource is the source_system the qualify reminder task is
// logged under, with the lead's id as its source_id. The pair makes the task
// idempotent, and the scan below reads it so a reminded lead is not offered
// again, whether or not its task is still open. A lead merge carries the
// loser's task onto the survivor under the loser's id, so the scan also
// counts a reminder linked to the lead.
const QualifyReminderSource = "lead_qualify"

// qualifyReminderBatch bounds one scan, so a backlog drains over a few passes
// rather than in one long transaction.
const qualifyReminderBatch = 200

// LeadsDueQualifyReminder lists the live leads worked from an existing contact
// that were created before createdBefore and have never been reminded. A lead
// that was qualified or disqualified is archived, so it is not listed.
func (s *Store) LeadsDueQualifyReminder(ctx context.Context, createdBefore time.Time) ([]ids.LeadID, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return nil, err
	}
	var due []ids.LeadID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM lead
			WHERE from_contact_id IS NOT NULL AND archived_at IS NULL AND created_at < $1
			  AND NOT EXISTS (
			        SELECT 1 FROM activity a
			         WHERE a.source_system = $2 AND a.source_id = lead.id::text)
			  AND NOT EXISTS (
			        SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
			         WHERE l.lead_id = lead.id AND a.source_system = $2)
			ORDER BY created_at
			LIMIT $3`, createdBefore, QualifyReminderSource, qualifyReminderBatch)
		if err != nil {
			return fmt.Errorf("select leads due a qualify reminder: %w", err)
		}
		due, err = pgx.CollectRows(rows, pgx.RowTo[ids.LeadID])
		return err
	})
	return due, err
}
