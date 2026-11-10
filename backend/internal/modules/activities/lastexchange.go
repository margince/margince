// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WhoseMove is who answers an exchange next. A mail's History badge move
// decides when the reader may read the mail; otherwise its direction does.
//
// earlierOwed overrides both for our own outbound mail: the correspondent's
// newest earlier mail still reads needs_reply, so this row did not answer it
// (EarlierMailOwed). The record then says the move is ours, as that badge does.
func WhoseMove(move *crmcontracts.EmailSummaryMove, direction string, earlierOwed bool) crmcontracts.LastExchangeWhoseMove {
	if earlierOwed && direction == string(crmcontracts.ActivityDirectionOutbound) {
		return crmcontracts.LastExchangeWhoseMoveUs
	}
	if move != nil {
		switch *move {
		case crmcontracts.EmailSummaryMoveWaitingForThem:
			return crmcontracts.LastExchangeWhoseMoveThem
		case crmcontracts.EmailSummaryMoveNeedsReply:
			return crmcontracts.LastExchangeWhoseMoveUs
		}
		return crmcontracts.LastExchangeWhoseMoveNeither
	}
	switch direction {
	case string(crmcontracts.ActivityDirectionOutbound):
		return crmcontracts.LastExchangeWhoseMoveThem
	case string(crmcontracts.ActivityDirectionInbound):
		return crmcontracts.LastExchangeWhoseMoveUs
	}
	return crmcontracts.LastExchangeWhoseMoveNeither
}

// lastExchange reads the record's newest call, meeting, email or message, by
// the interaction rule the account's strip reads its newest message with.
func (s *Store) lastExchange(
	ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, now time.Time, columns CustomColumns,
) (*crmcontracts.LastExchange, error) {
	one := 1
	rows, _, err := s.ListActivitiesIn(ctx, tx, ListActivitiesInput{
		EntityType: &entityType, EntityID: &entityID, InteractionsAsOf: &now,
		Limit: &one, AsOf: &now, Columns: columns,
	})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	row := rows[0]
	moves, err := MailMovesFor(ctx, tx, []ids.UUID{ids.UUID(row.Id)}, now)
	if err != nil {
		return nil, err
	}
	var move *crmcontracts.EmailSummaryMove
	if found, ok := moves[ids.UUID(row.Id)]; ok {
		move = &found
	}
	direction := ""
	if row.Direction != nil {
		direction = string(*row.Direction)
	}
	owed, err := EarlierMailOwed(ctx, tx, ids.UUID(row.Id), now)
	if err != nil {
		return nil, err
	}
	return &crmcontracts.LastExchange{Activity: row, WhoseMove: WhoseMove(move, direction, owed)}, nil
}

// earlierInboundMailSQL keeps the live inbound mail the row aliased their
// received before the row aliased ours.
const earlierInboundMailSQL = `their.kind = 'email' AND their.direction = 'inbound'
		     AND their.archived_at IS NULL
		     AND their.occurred_at < ours.occurred_at -- occurred_at raw: an ordering of two rows, not whether either happened
		     `

// EarlierMailOwed reports whether our outbound mail left its correspondent's
// newest earlier mail reading needs_reply on its History badge. A reply logged
// by hand, off the mail's thread and unattested, answers nothing by the answer
// rule, so that mail is still owed after it. A call or a meeting is not asked:
// a voicemail we left puts the move with them, whatever the badge says.
//
// The correspondent is the row's counterparty address when it has one. A mail
// with an address may be filed under contacts it was only copied to. So only a
// row with no address, as a hand-logged mail often is, is matched by a contact
// it is filed under. The newest earlier mail is chosen among those the reader may
// open, so a mail kept from them can neither decide nor hide the move.
func EarlierMailOwed(ctx context.Context, tx pgx.Tx, activityID ids.UUID, now time.Time) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ours := arg(activityID)
	readable, err := auth.ActivityContentClause(ctx, "their", arg)
	if err != nil {
		return false, err
	}
	var earlier ids.UUID
	// One indexed probe per way of naming the correspondent. A single join on
	// the address or a linked contact reads every inbound mail in the workspace.
	// The content gate is applied once, over the candidates both probes found.
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		WITH found AS (
		  SELECT their.id
		    FROM activity ours
		    JOIN activity their ON their.counterparty_email = ours.counterparty_email
		   WHERE ours.id = $%[1]d AND ours.kind = 'email' AND ours.direction = 'outbound'
		     AND `+earlierInboundMailSQL+`
		  UNION
		  SELECT their.id
		    FROM activity ours
		    JOIN activity_link ourl ON ourl.activity_id = ours.id AND ourl.contact_id IS NOT NULL
		    JOIN activity_link theirs ON theirs.contact_id = ourl.contact_id
		    JOIN activity their ON their.id = theirs.activity_id
		   WHERE ours.id = $%[1]d AND ours.kind = 'email' AND ours.direction = 'outbound'
		     AND ours.counterparty_email IS NULL
		     AND `+earlierInboundMailSQL+`)
		SELECT their.id FROM found JOIN activity their ON their.id = found.id
		 WHERE %[2]s
		 ORDER BY their.occurred_at DESC, their.id DESC
		 LIMIT 1`, ours, readable), args...).Scan(&earlier)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading the mail an outbound row follows: %w", err)
	}
	moves, err := MailMovesFor(ctx, tx, []ids.UUID{earlier}, now)
	if err != nil {
		return false, err
	}
	move, ok := moves[earlier]
	return ok && move == crmcontracts.EmailSummaryMoveNeedsReply, nil
}
