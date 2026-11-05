// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EmailRowState keeps obligation and delivery facts in one page-batched read.
type EmailRowState struct {
	RequestHasReminder bool
	Delivery           DeliveryState
	Move               crmcontracts.EmailSummaryMove
}

// EmailStatesFor reads obligation and delivery after the caller's content
// gate. "Needs reply" is owedSQL and "waiting for them" is
// awaitingTheirAnswerSQL for a mail the reader sent. The waiting and follow-up
// lanes start from the same two, so a badge and a lane row agree on whose move
// it is. A captured reminder's completion settles its source request.
//
// asOf is the instant the move is judged at, the reading's own clock.
func EmailStatesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, asOf time.Time) (map[ids.UUID]EmailRowState, error) {
	if len(activityIDs) == 0 {
		return map[ids.UUID]EmailRowState{}, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wanted := arg(activityIDs)
	in, err := moveInputs(ctx, arg, asOf)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, coalesce(delivery.status, ''), delivery.reason,
 delivery.sent_at, delivery.bounced_at, delivery.bounce_reason, coalesce(delivery.attachments, '[]'::jsonb),
 `+ControllerSendSQL+`,
 `+moveSQL(in, mailKinds)+`, `+openRequestReminderSQL+`
 FROM activity a LEFT JOIN comms_outbound delivery ON delivery.activity_id = a.id
 WHERE a.id = ANY(`+fmt.Sprintf("$%d", wanted)+`) AND a.restricted_at IS NULL`, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading email states: %w", err)
	}
	defer rows.Close()
	out := make(map[ids.UUID]EmailRowState, len(activityIDs))
	for rows.Next() {
		var id ids.UUID
		var state EmailRowState
		if err := rows.Scan(&id, &state.Delivery.Status, &state.Delivery.Reason, &state.Delivery.SentAt,
			&state.Delivery.BouncedAt, &state.Delivery.BounceReason, &state.Delivery.Files,
			&state.Delivery.Controller, &state.Delivery.FromAddress, &state.Move,
			&state.RequestHasReminder); err != nil {
			return nil, err
		}
		out[id] = state
	}
	return out, rows.Err()
}

// moveSQL is whose move the mail on alias a is, as its History badge reads.
//
// A request reads needs_reply only inside the waiting horizon, the same bound
// the queue and the waiting_reply filter apply (withinWaitingHorizonSQL). Past
// it the request is no longer current work unless a human kept it, so it reads
// none. The mail stays on the timeline; none does not mean answered.
func moveSQL(in moveBinding, kinds string) string {
	return `CASE WHEN coalesce(` + owedSQL(in.asOf, neverRelaxed, neverRelaxed, neverRelaxed) + ` AND ` +
		withinWaitingHorizonSQL(in.asOf, in.horizon) + `, false)
   THEN '` + string(crmcontracts.EmailSummaryMoveNeedsReply) + `'
   WHEN coalesce(` + awaitingTheirAnswerSQL(in.asOf, in.sentByReader, in.laterContent, kinds) + `, false)
   THEN '` + string(crmcontracts.EmailSummaryMoveWaitingForThem) + `'
   ELSE '` + string(crmcontracts.EmailSummaryMoveNone) + `' END`
}

// moveBinding is what moveSQL reads, already bound as parameters.
type moveBinding struct {
	asOf, sentByReader, laterContent, horizon string
}

// moveInputs binds what moveSQL reads about the reader and the instant. Only
// the seat stamp counts as the reader's. Their hand-typed addresses live with
// capture, which a page read cannot reach. Such a mail reads none, never
// theirs. The horizon is the stored measurement the queue reads, clamped to its
// ceiling.
func moveInputs(ctx context.Context, arg func(any) int, asOf time.Time) (moveBinding, error) {
	laterContent, err := auth.ActivityContentClause(ctx, "later", arg)
	if err != nil {
		return moveBinding{}, err
	}
	return moveBinding{
		asOf:         fmt.Sprintf("$%d::timestamptz", arg(asOf)),
		sentByReader: sentByReaderSQL(fmt.Sprintf("$%d", arg(readerOrNobody(ctx))), fmt.Sprintf("$%d", arg([]string{}))),
		laterContent: laterContent,
		horizon:      storekit.WaitingHorizonSQL(asOf, arg),
	}, nil
}

// MailMoveColumnSQL is the History badge's move on the activity under alias a
// as a column. It is NULL when the row is not a mail the reader may read, since
// the move is judged from the mail's content.
func MailMoveColumnSQL(ctx context.Context, arg func(any) int, asOf time.Time) (string, error) {
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	in, err := moveInputs(ctx, arg, asOf)
	if err != nil {
		return "", err
	}
	return `CASE WHEN a.kind = 'email' AND a.restricted_at IS NULL AND ` + content + ` THEN ` +
		moveSQL(in, mailKinds) + ` END`, nil
}

// MailMovesFor reads the History badge's move for each mail among activityIDs.
// A mail the reader may not read, or another kind, is absent, so its caller
// decides whose move it is by its own rule.
func MailMovesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, asOf time.Time) (map[ids.UUID]crmcontracts.EmailSummaryMove, error) {
	out := make(map[ids.UUID]crmcontracts.EmailSummaryMove, len(activityIDs))
	if len(activityIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wanted := arg(activityIDs)
	move, err := MailMoveColumnSQL(ctx, arg, asOf)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, move FROM (SELECT a.id, `+move+` AS move
 FROM activity a WHERE a.id = ANY(`+fmt.Sprintf("$%d", wanted)+`)) a
 WHERE move IS NOT NULL`, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading whose move each mail is: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var move crmcontracts.EmailSummaryMove
		if err := rows.Scan(&id, &move); err != nil {
			return nil, fmt.Errorf("activities: reading whose move each mail is: %w", err)
		}
		out[id] = move
	}
	return out, rows.Err()
}
