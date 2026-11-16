// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
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
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// EmailRequestTaskSource identifies tasks derived from a confirmed email request.
const EmailRequestTaskSource = provenance.EmailRequestSource

const automaticRequestHorizonDays = 14

// emailRequestAssigneeSQL is the reminder's assignee: the mail's sole
// directly addressed seat (soleDirectSeatSQL), the decision the waiting queue
// reads, when that seat's own mailbox delivered it. capture_import proves
// delivery, not who was addressed, so it only confirms the seat. A shared
// mailbox's import confirms the seat that holds its mail's work instead.
func emailRequestAssigneeSQL(identities string) string {
	return `SELECT DISTINCT u.id AS user_id FROM capture_import ci
      JOIN (SELECT id AS owner_id, app_user.* FROM app_user) u
        ON u.id = coalesce(ci.user_id, ` + strings.ReplaceAll(storekit.SharedMailWorkOwnerSQL("a"), "%", "%%") + `) AND %s
      WHERE ci.activity_id = a.id AND u.id = ` + strings.ReplaceAll(soleDirectSeatSQL(identities), "%", "%%")
}

// CaptureEmailRequests reconciles recent confirmed requests into tasks for
// their one directly addressed importing seat. A task is due on the day the
// request states (statedRequestDue) and is otherwise undated. The pass retires
// the ones it filed whose request has aged past the waiting horizon untouched. Historical and ambiguously
// addressed requests remain in review; this pass neither assigns them silently
// nor recreates reminders a human archived or completed.
func (s *Store) CaptureEmailRequests(ctx context.Context, asOf time.Time) error {
	return s.CaptureEmailRequestsAsked(ctx, asOf, nil)
}

// RequestAsks is what each judged message asks, in the sender's own words,
// keyed by the message. A message absent from it files a reminder that says
// nothing beyond its subject.
type RequestAsks map[ids.UUID]string

// Keep records what a message asks, once its verdict is written.
func (a RequestAsks) Keep(message ids.UUID, written bool, ask string) {
	if written && ask != "" {
		a[message] = ask
	}
}

// CaptureEmailRequestsAsked is CaptureEmailRequests for a pass that just
// judged the messages, so each reminder it files quotes what its sender asked.
func (s *Store) CaptureEmailRequestsAsked(ctx context.Context, asOf time.Time, asks RequestAsks) error {
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
		domains, err := s.ownDomainList(ctx, tx)
		if err != nil {
			return err
		}
		if err := s.retireNeverOwedReminders(ctx, tx, asOf, domains); err != nil {
			return err
		}
		if err := s.rehomeSharedReminders(ctx, tx, asOf); err != nil {
			return err
		}
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		identities, err := s.mailSeatIdentitiesSQL(ctx, tx, arg)
		if err != nil {
			return err
		}
		assigneeSQL := fmt.Sprintf(emailRequestAssigneeSQL(identities), auth.AssigneeEligibleSQL("u"))
		wake := messageSnoozeLiftedSQL(fmt.Sprintf("$%d", arg(asOf)), "TRUE")
		eligible, err := waitingReplyExistsClause(ctx, arg, asOf, nil, nil, domains, nil, automaticRequestHorizonDays,
			outstandingRequestSQL+`
     AND `+storekit.AutomaticRequestIntentClause("a")+`
     AND a.occurred_at >= $`+fmt.Sprint(arg(asOf.AddDate(0, 0, -automaticRequestHorizonDays)))+`
     AND (`+auth.WorkspaceAudienceSQL("a")+` OR `+storekit.SharedMailWorkOwnerSQL("a")+` IS NOT NULL) AND a.restricted_at IS NULL
     AND NOT `+answerableInvitationSQL+`
     AND NOT `+invitationNeverOwedSQL(fmt.Sprintf("$%d::timestamptz", arg(asOf)), fmt.Sprintf("$%d", arg(domains)))+`
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
			if err := s.captureEmailRequest(ctx, tx, candidate.message, seat, asOf, asks[candidate.message]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) captureEmailRequest(ctx context.Context, tx pgx.Tx, messageID ids.UUID, seat principal.Principal, asOf time.Time, ask string) error {
	source, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](messageID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	request, err := emailRequestTask(ctx, tx, source, seat, asOf)
	if err != nil {
		return err
	}
	if err := quoteTheAsk(ctx, tx, &request, messageID, ask); err != nil {
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

// quoteTheAsk gives the reminder its reason: who asked, and what, in their own
// words. The words are the mail's own, so the reminder reads as the sender
// wrote in every reader's language; only the frame around them translates.
func quoteTheAsk(ctx context.Context, tx pgx.Tx, request *LogActivityInput, message ids.UUID, ask string) error {
	ask = strings.TrimSpace(ask)
	if ask == "" {
		return nil
	}
	// The name the sender's own mail gives them, so the reminder says what
	// every reader of that mail already sees.
	var sender string
	err := tx.QueryRow(ctx, `
		SELECT p.display_name FROM activity_participant p
		 WHERE p.activity_id = $1 AND p.role = 'from' AND nullif(btrim(p.display_name), '') IS NOT NULL
		 ORDER BY p.id LIMIT 1`, message).Scan(&sender)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("activities: reading who asked: %w", err)
	}
	why := systemtext.Sentence("request.asked_unnamed", systemtext.Text(ask))
	if sender != "" {
		why = systemtext.Sentence("request.asked", systemtext.Text(sender), systemtext.Text(ask))
	}
	return request.SayWhy(why, textlang.English)
}
