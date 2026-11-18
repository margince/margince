// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// ourReplySQL is the one rule for "we replied after the request". The open
// request under alias a has an unfinished reminder. Our reply on its thread
// reached its sender, and no message of theirs came after that reply. It
// selects one row, or none. The request stays open: a reply settles nothing.
//
// reminderReadable gates the reminder under alias reminder_row. A reader who
// may not read it learns that we replied, not which task tracks it.
func ourReplySQL(asOf, reminderReadable string) string {
	return `SELECT reply.occurred_at AS replied_at, reply.occurred_precision AS replied_precision,
	   (SELECT asked.remaining FROM activity_request_settlement asked
	     WHERE asked.request_activity_id = a.id AND asked.verdict = '` + RequestStillOwed + `'
	       AND asked.rule_version = '` + RequestSettlementRule + `') AS still_asked,
	   reminder_row.id AS reminder_id, reminder_row.version AS reminder_version
	  FROM (` + storekit.ThreadNewestReplySQL("a", asOf) + `) reply
	  LEFT JOIN LATERAL (SELECT reminder_row.id, reminder_row.version FROM activity reminder_row
	     WHERE reminder_row.source_system = '` + EmailRequestTaskSource + `'
	       AND reminder_row.source_activity_id = a.id AND NOT reminder_row.is_done
	       AND reminder_row.archived_at IS NULL AND ` + reminderReadable + `
	     ORDER BY reminder_row.created_at DESC, reminder_row.id DESC LIMIT 1) reminder_row ON true
	 WHERE ` + requestOpenSQL + ` AND ` + openRequestReminderSQL + `
	   AND NOT EXISTS (SELECT 1 FROM activity newer WHERE ` + newerInboundSQL("reply", asOf) + `)`
}

// ourReplyRow is ourReplySQL's row as scanned. Every field is nil when the
// rule does not hold.
type ourReplyRow struct {
	repliedAt       *time.Time
	precision       *string
	stillAsked      *string
	reminderID      *openapi_types.UUID
	reminderVersion *int64
}

// targets are the scan destinations, in ourReplySQL's column order.
func (r *ourReplyRow) targets() []any {
	return []any{&r.repliedAt, &r.precision, &r.stillAsked, &r.reminderID, &r.reminderVersion}
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
	}
	if r.stillAsked != nil && *r.stillAsked != "" {
		out.StillAsked = r.stillAsked
	}
	if r.reminderID != nil && r.reminderVersion != nil {
		out.ReminderId, out.ReminderVersion = r.reminderID, r.reminderVersion
	}
	return out
}
