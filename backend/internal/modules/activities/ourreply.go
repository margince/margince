// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ourReplySQL is the one rule for "we replied after the request". The open
// request under alias a has a reminder, our delivered reply reached its sender,
// and nothing of theirs came after it. The request stays open.
// reminderReadable gates the reminder under alias reminder_row. replyByReader
// says the reader sent the reply, read as the row reply_row.
func ourReplySQL(asOf, reminderReadable, replyByReader string) string {
	return `SELECT reply.occurred_at AS replied_at, reply.occurred_precision AS replied_precision,
	   (SELECT asked.remaining FROM activity_request_settlement asked
	     WHERE asked.request_activity_id = a.id AND asked.verdict = '` + RequestStillOwed + `'
	       AND asked.rule_version = '` + RequestSettlementRule + `') AS still_asked,
	   reminder_row.id AS reminder_id, reminder_row.version AS reminder_version,
	   EXISTS (SELECT 1 FROM activity reply_row WHERE reply_row.id = reply.id AND ` + replyByReader + `) AS by_reader
	  FROM (` + storekit.ThreadNewestReplySQL("a", asOf) + `) reply
	  LEFT JOIN LATERAL (SELECT reminder_row.id, reminder_row.version FROM activity reminder_row
	     WHERE reminder_row.source_system = '` + EmailRequestTaskSource + `'
	       AND reminder_row.source_activity_id = a.id AND NOT reminder_row.is_done
	       AND reminder_row.archived_at IS NULL AND ` + reminderReadable + `
	     ORDER BY reminder_row.created_at DESC, reminder_row.id DESC LIMIT 1) reminder_row ON true
	 WHERE ` + requestOpenSQL + ` AND ` + openRequestReminderSQL + `
	   AND NOT EXISTS (SELECT 1 FROM activity newer WHERE ` + inboundOnThreadSQL(asOf) + `
	     AND ` + occurredday.AnswersSQL("newer", "reply") + `)`
}

// keepWritableReminders drops the reminder from each reply whose reader may
// not change it, so a Done offered on it is one the task write accepts.
func keepWritableReminders(ctx context.Context, tx pgx.Tx, states map[ids.UUID]EmailRowState) error {
	var reminders []ids.UUID
	for _, state := range states {
		if state.OurReply != nil && state.OurReply.ReminderId != nil {
			reminders = append(reminders, ids.UUID(*state.OurReply.ReminderId))
		}
	}
	if len(reminders) == 0 {
		return nil
	}
	writable := map[ids.UUID]bool{}
	if auth.AllowsWrite(ctx, "activity", principal.ActionUpdate) {
		var err error
		if writable, err = auth.ActivityWritableSubset(ctx, tx, reminders); err != nil {
			return err
		}
	}
	for _, state := range states {
		if reply := state.OurReply; reply != nil && reply.ReminderId != nil && !writable[ids.UUID(*reply.ReminderId)] {
			reply.ReminderId, reply.ReminderVersion = nil, nil
		}
	}
	return nil
}

// ourReplyRow is ourReplySQL's row as scanned. Every field is nil when the
// rule does not hold.
type ourReplyRow struct {
	repliedAt       *time.Time
	precision       *string
	stillAsked      *string
	reminderID      *openapi_types.UUID
	reminderVersion *int64
	byReader        *bool
}

// targets are the scan destinations, in ourReplySQL's column order.
func (r *ourReplyRow) targets() []any {
	return []any{&r.repliedAt, &r.precision, &r.stillAsked, &r.reminderID, &r.reminderVersion, &r.byReader}
}

// contract renders the row for the request it was read on, or nil when the
// rule does not hold.
func (r ourReplyRow) contract(request openapi_types.UUID) *crmcontracts.RequestReply {
	if r.repliedAt == nil || r.precision == nil {
		return nil
	}
	out := &crmcontracts.RequestReply{
		RequestActivityId: request,
		RepliedAt:         *r.repliedAt,
		RepliedPrecision:  crmcontracts.OccurredPrecision(*r.precision),
		ByReader:          r.byReader != nil && *r.byReader,
	}
	if r.stillAsked != nil && *r.stillAsked != "" {
		out.StillAsked = r.stillAsked
	}
	if r.reminderID != nil && r.reminderVersion != nil {
		out.ReminderId, out.ReminderVersion = r.reminderID, r.reminderVersion
	}
	return out
}
