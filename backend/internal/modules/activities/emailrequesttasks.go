// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// EmailRequestTaskSource identifies tasks derived from a confirmed email request.
const EmailRequestTaskSource = provenance.EmailRequestSource

const automaticRequestHorizonDays = 14

// capture_import proves mailbox delivery, not who was addressed. Participant
// user IDs are stamped by import too; only addresses or the mapper's To header
// establish a direct recipient. mailmap.Message.Record prepends the parsed
// envelope before all sender text; only that captured-email path may use it.
// Resolve before the waiting scan's bound.
const emailRequestAssigneeSQL = `SELECT min(u.id::text)::uuid AS user_id FROM capture_import ci
      JOIN (SELECT id AS owner_id, app_user.* FROM app_user) u ON u.id = ci.user_id AND %s
      WHERE ci.activity_id = a.id AND (EXISTS (
        SELECT 1 FROM activity_participant recipient
        WHERE recipient.activity_id = a.id AND recipient.role = 'to'
          AND lower(recipient.address) = lower(u.email))
		 OR (a.source_system = '` + connector.EmailSourceSystem + `' AND a.captured_by LIKE 'connector:%%' AND a.body LIKE E'From:%%\nTo:%%\n\n%%'
		   AND lower(split_part(a.body, E'\n', 2)) = 'to: ' || lower(u.email)))
      HAVING count(DISTINCT u.id) = 1`

// CaptureEmailRequests reconciles recent confirmed requests into undated tasks
// for their one directly addressed importing seat. Historical and ambiguously
// addressed requests remain in review; this pass neither assigns them silently
// nor recreates reminders a human archived or completed.
func (s *Store) CaptureEmailRequests(ctx context.Context, asOf time.Time) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		domains, err := s.ownDomainList(ctx, tx)
		if err != nil {
			return err
		}
		assigneeSQL := fmt.Sprintf(emailRequestAssigneeSQL, auth.AssigneeEligibleSQL(actor, "u", arg))
		wake := messageSnoozeLiftedSQL(fmt.Sprintf("$%d", arg(asOf)), "TRUE")
		eligible, err := waitingReplyExistsClause(ctx, arg, asOf, nil, nil, domains, nil, automaticRequestHorizonDays,
			outstandingRequestSQL+`
     AND a.capture_label IS DISTINCT FROM 'noise'
     AND a.occurred_at >= $`+fmt.Sprint(arg(asOf.AddDate(0, 0, -automaticRequestHorizonDays)))+`
     AND a.audience = 'workspace' AND a.restricted_at IS NULL
     AND NOT EXISTS (SELECT 1 FROM activity task WHERE task.source_system = 'email_request' AND task.source_activity_id = a.id)
     AND (`+assigneeSQL+`) IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM activity_reader_state mine
       WHERE mine.activity_id = a.id AND mine.reader_id = (`+assigneeSQL+`)
       AND (mine.state = 'not_mine' OR (mine.state = 'snoozed' AND NOT `+wake+`)))`)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT a.id, (`+assigneeSQL+`) FROM activity a
    WHERE `+eligible+` ORDER BY a.occurred_at DESC, a.id LIMIT 64 FOR UPDATE OF a SKIP LOCKED`, args...)
		if err != nil {
			return fmt.Errorf("activities: reading email requests: %w", err)
		}
		type assignment struct{ message, user ids.UUID }
		candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (assignment, error) {
			var value assignment
			err := row.Scan(&value.message, &value.user)
			return value, err
		})
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if err := s.captureEmailRequest(ctx, tx, candidate.message, candidate.user, asOf); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) captureEmailRequest(ctx context.Context, tx pgx.Tx, messageID, userID ids.UUID, asOf time.Time) error {
	source, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](messageID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	_, _, err = s.LogActivityTx(ctx, tx, emailRequestTaskInput(source, userID, asOf))
	return err
}
