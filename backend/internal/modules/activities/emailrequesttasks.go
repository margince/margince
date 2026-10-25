// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// EmailRequestTaskSource identifies tasks derived from a confirmed email request.
const EmailRequestTaskSource = provenance.EmailRequestSource

const automaticRequestHorizonDays = 14

// emailRequestAssigneeSQL is the reminder's assignee: the mail's sole
// directly addressed seat (soleDirectSeatSQL), the decision the waiting queue
// reads, when that seat's own mailbox delivered it. capture_import proves
// delivery, not who was addressed, so it only confirms the seat.
func emailRequestAssigneeSQL(identities string) string {
	return `SELECT DISTINCT u.id AS user_id FROM capture_import ci
      JOIN (SELECT id AS owner_id, app_user.* FROM app_user) u ON u.id = ci.user_id AND %s
      WHERE ci.activity_id = a.id AND u.id = ` + strings.ReplaceAll(soleDirectSeatSQL(identities), "%", "%%")
}

// CaptureEmailRequests reconciles recent confirmed requests into tasks for
// their one directly addressed importing seat. A task is due on the day the
// request states (statedRequestDue) and is otherwise undated. The pass retires
// the ones it filed whose request has aged past the waiting horizon untouched. Historical and ambiguously
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
	if err := auth.Require(ctx, "activity", principal.ActionDelete); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.retireAgedReminders(ctx, tx, asOf); err != nil {
			return err
		}
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		domains, err := s.ownDomainList(ctx, tx)
		if err != nil {
			return err
		}
		identities, err := s.mailSeatIdentitiesSQL(ctx, tx, arg)
		if err != nil {
			return err
		}
		assigneeSQL := fmt.Sprintf(emailRequestAssigneeSQL(identities), auth.AssigneeEligibleSQL(actor, "u", arg))
		wake := messageSnoozeLiftedSQL(fmt.Sprintf("$%d", arg(asOf)), "TRUE")
		eligible, err := waitingReplyExistsClause(ctx, arg, asOf, nil, nil, domains, nil, automaticRequestHorizonDays,
			outstandingRequestSQL+`
     AND `+storekit.AutomaticRequestIntentClause("a")+`
     AND a.occurred_at >= $`+fmt.Sprint(arg(asOf.AddDate(0, 0, -automaticRequestHorizonDays)))+`
     AND a.audience = 'workspace' AND a.restricted_at IS NULL
     AND NOT EXISTS (SELECT 1 FROM activity task WHERE task.source_system = 'email_request' AND task.source_activity_id = a.id)
     AND (`+assigneeSQL+`) IS NOT NULL
     AND NOT `+readerSetAsideSQL("("+assigneeSQL+")", wake), neverRelaxed)
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
		// One seat read per assignee per pass, not per message.
		seats := map[ids.UUID]principal.Principal{}
		for _, candidate := range candidates {
			seat, known := seats[candidate.user]
			if !known {
				if seat, err = auth.SeatPrincipal(ctx, tx, candidate.user); err != nil {
					return err
				}
				seats[candidate.user] = seat
			}
			if err := s.captureEmailRequest(ctx, tx, candidate.message, seat, asOf); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) captureEmailRequest(ctx context.Context, tx pgx.Tx, messageID ids.UUID, seat principal.Principal, asOf time.Time) error {
	source, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](messageID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	request, err := emailRequestTask(ctx, tx, source, seat, asOf)
	if err != nil {
		return err
	}
	due, stated, err := s.statedRequestDue(ctx, tx, source, asOf)
	if err != nil {
		return err
	}
	if stated {
		request.DueAt = &due
	}
	_, _, err = s.LogActivityTx(ctx, tx, request)
	return err
}
