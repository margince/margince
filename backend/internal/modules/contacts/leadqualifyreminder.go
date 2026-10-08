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
// again, whether or not its task is still open.
const QualifyReminderSource = "lead_qualify"

// qualifyReminderBatch bounds one scan, so a backlog drains over a few passes
// rather than in one long transaction.
const qualifyReminderBatch = 200

// QualifyReminderDue is a lead worked from a contact that is still open past
// the reminder age, with whoever answers for it.
type QualifyReminderDue struct {
	LeadID  ids.LeadID
	OwnerID *ids.UserID
	Name    string
}

// LeadsDueQualifyReminder lists the live leads worked from an existing contact
// that were created before createdBefore and have never been reminded. A lead
// that was qualified or disqualified is archived, so it is not listed.
func (s *Store) LeadsDueQualifyReminder(ctx context.Context, createdBefore time.Time) ([]QualifyReminderDue, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return nil, err
	}
	var due []QualifyReminderDue
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, owner_id, COALESCE(NULLIF(btrim(full_name), ''), email::text, '')
			FROM lead
			WHERE from_contact_id IS NOT NULL AND archived_at IS NULL AND created_at < $1
			  AND NOT EXISTS (
			        SELECT 1 FROM activity a
			         WHERE a.source_system = $2 AND a.source_id = lead.id::text)
			ORDER BY created_at
			LIMIT $3`, createdBefore, QualifyReminderSource, qualifyReminderBatch)
		if err != nil {
			return fmt.Errorf("select leads due a qualify reminder: %w", err)
		}
		due, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (QualifyReminderDue, error) {
			var d QualifyReminderDue
			return d, row.Scan(&d.LeadID, &d.OwnerID, &d.Name)
		})
		return err
	})
	return due, err
}
