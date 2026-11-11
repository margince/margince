// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Reopening the quiet reminders a newsletter closed before a newsletter
// stopped counting as a touch.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// quietReminderScope is every quiet reminder. It reads uq_activity_source, so
// a page stays bounded however large the activity table is.
func quietReminderScope(sourcesPos int) string {
	return storekit.SQLf(`FROM activity a
		 WHERE a.source_system = ANY($%d) AND a.source_id IS NOT NULL AND a.kind = 'task'`, sourcesPos)
}

// CountQuietReminders counts the quiet reminders the repair inspects, reading
// at most maximum+1.
func CountQuietReminders(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+quietReminderScope(1)+` LIMIT $2) reminders`,
		provenance.EngineReminderSources(), maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting quiet reminders: %w", err)
	}
	return count, nil
}

// ReopenNewsletterClosedRemindersTx reopens each of the next limit quiet
// reminders a newsletter closed, and answers the ids it inspected.
//
// The engine's actor wrote the completion's audit row, after a newsletter past
// the anchor was captured. No touch genuineEngagement admits has reached the
// record since the anchor. A reminder a person or agent worked on stays.
func (s *Store) ReopenNewsletterClosedRemindersTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	inspected, err := lockQuietReminderPage(ctx, tx, after, limit)
	if err != nil || len(inspected) == 0 {
		return inspected, 0, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	anchor := reminderAnchor("a", arg(provenance.ReminderAnchorSeparator))
	closedByNewsletter := storekit.SQLf(`EXISTS (SELECT 1 FROM audit_log c
		   WHERE c.entity_type = 'activity' AND c.entity_id = a.id AND c.action = 'update'
		     AND c.actor_type = $%[1]d AND c.actor_id = $%[2]d AND c.after->>'is_done' = 'true'
		     AND EXISTS (SELECT 1 FROM activity x WHERE x.id IN (%[3]s) AND %[4]s
		                  AND x.occurred_at > coalesce(%[5]s, '-infinity') -- occurred_at raw: past the reminder's anchor, a window's lower edge
		                  AND x.created_at <= c.occurred_at)) -- occurred_at raw: the audit row's time, which the newsletter's capture precedes
		 AND NOT EXISTS (SELECT 1 FROM audit_log w
		   WHERE w.entity_type = 'activity' AND w.entity_id = a.id AND w.actor_type = ANY($%[6]d))
		 AND NOT EXISTS (SELECT 1 FROM activity x WHERE x.id IN (%[3]s) AND %[7]s
		   AND x.occurred_at > coalesce(%[5]s, '-infinity')) -- occurred_at raw: past the reminder's anchor, a window's lower edge`,
		arg(string(principal.PrincipalSystem)), arg(systemCapturedBy),
		reminderReachSet("a"), relstrength.BulkMailSQL("x"), anchor,
		arg([]string{string(principal.PrincipalHuman), string(principal.PrincipalAgent)}),
		genuineEngagement("x", arg(systemSource), arg(systemCapturedBy), arg(systemCapturedByPattern)))
	// The page is locked, so this statement's fresh snapshot sees every edit a
	// person committed before the lock, and none can land before the reopen.
	query := storekit.SQLf(`SELECT a.id FROM activity a WHERE a.id = ANY($%d) AND a.is_done AND %s`,
		arg(inspected), closedByNewsletter)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading quiet reminders a newsletter closed: %w", err)
	}
	closed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading quiet reminders a newsletter closed: %w", err)
	}
	reopened, err := s.flipNamedSystemTasksTx(ctx, tx, closed, true)
	return inspected, reopened, err
}

// lockQuietReminderPage locks the next limit quiet reminders after after and
// answers their ids.
func lockQuietReminderPage(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT a.id `+quietReminderScope(1)+` AND a.id > $2 ORDER BY a.id LIMIT $3 FOR UPDATE OF a`,
		provenance.EngineReminderSources(), after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: locking quiet reminders: %w", err)
	}
	page, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: locking quiet reminders: %w", err)
	}
	return page, nil
}

// reminderReachSet is the activities that reach a record the reminder aliased
// as alias is on: the walk CompleteQuietRemindersReachedBy takes, read from the
// reminder's side.
func reminderReachSet(alias string) string {
	return sprintf(`SELECT t.activity_id FROM activity_link rl JOIN activity_link t ON t.deal_id = rl.deal_id
		     WHERE rl.activity_id = %[1]s.id
		    UNION SELECT t.activity_id FROM activity_link rl JOIN activity_link t ON t.contact_id = rl.contact_id
		     WHERE rl.activity_id = %[1]s.id
		    UNION SELECT reach.activity_id FROM activity_link rl CROSS JOIN LATERAL (%[2]s) reach
		     WHERE rl.activity_id = %[1]s.id AND rl.company_id IS NOT NULL`, alias, companyLinkReach("rl.company_id"))
}
