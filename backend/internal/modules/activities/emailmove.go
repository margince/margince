// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
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
func EmailStatesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]EmailRowState, error) {
	if len(activityIDs) == 0 {
		return map[ids.UUID]EmailRowState{}, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wanted := arg(activityIDs)
	// Only the seat stamp: the reader's hand-typed addresses live with capture,
	// which this page read cannot reach, so such a mail reads none, never theirs.
	sentByReader := sentByReaderSQL(fmt.Sprintf("$%d", arg(readerOrNobody(ctx))), fmt.Sprintf("$%d", arg([]string{})))
	laterContent, err := auth.ActivityContentClause(ctx, "later", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, coalesce(delivery.status, ''), delivery.reason,
 delivery.sent_at, delivery.bounced_at, delivery.bounce_reason, coalesce(delivery.attachments, '[]'::jsonb),
 `+moveSQL(sentByReader, laterContent)+`, `+openRequestReminderSQL+`
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
			&state.Delivery.BouncedAt, &state.Delivery.BounceReason, &state.Delivery.Files, &state.Move,
			&state.RequestHasReminder); err != nil {
			return nil, err
		}
		out[id] = state
	}
	return out, rows.Err()
}

// moveSQL is whose move the mail on alias a is, as its History badge reads.
func moveSQL(sentByReader, laterContent string) string {
	return `CASE WHEN coalesce(` + owedSQL("now()", neverRelaxed, neverRelaxed, neverRelaxed) + `, false)
   THEN '` + string(crmcontracts.EmailSummaryMoveNeedsReply) + `'
   WHEN coalesce(` + awaitingTheirAnswerSQL("now()", sentByReader, laterContent) + `, false)
   THEN '` + string(crmcontracts.EmailSummaryMoveWaitingForThem) + `'
   ELSE '` + string(crmcontracts.EmailSummaryMoveNone) + `' END`
}

// MailMovesFor reads the History badge's move for each mail the reader may
// discover among activityIDs. Another kind, or a mail hidden from the reader,
// is absent, so its caller decides whose move it is by its own rule.
func MailMovesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]crmcontracts.EmailSummaryMove, error) {
	out := make(map[ids.UUID]crmcontracts.EmailSummaryMove, len(activityIDs))
	if len(activityIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wanted := arg(activityIDs)
	discover, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	laterContent, err := auth.ActivityContentClause(ctx, "later", arg)
	if err != nil {
		return nil, err
	}
	sentByReader := sentByReaderSQL(fmt.Sprintf("$%d", arg(readerOrNobody(ctx))), fmt.Sprintf("$%d", arg([]string{})))
	rows, err := tx.Query(ctx, `SELECT a.id, `+moveSQL(sentByReader, laterContent)+`
 FROM activity a
 WHERE a.id = ANY(`+fmt.Sprintf("$%d", wanted)+`) AND a.kind = 'email' AND a.restricted_at IS NULL AND `+discover, args...)
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
