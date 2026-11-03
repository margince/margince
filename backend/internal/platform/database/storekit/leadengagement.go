// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// inboundReplyKinds are the activity kinds a lead replies to us in: mail, or a
// message on a channel.
var inboundReplyKinds = []string{replyKindEmail, replyKindMessage}

const (
	replyKindEmail   = "email"
	replyKindMessage = "message"
)

// InboundReplySQL is a reply someone sent us, on the activity row alias.
func InboundReplySQL(alias string) string {
	return `(` + alias + `.kind IN ('` + strings.Join(inboundReplyKinds, "','") + `') AND ` + alias + `.direction = 'inbound')`
}

// IsInboundReply is InboundReplySQL for a reader holding the row in Go.
func IsInboundReply(kind, direction string) bool {
	return direction == "inbound" && slices.Contains(inboundReplyKinds, kind)
}

// CurrentLeadEngagementClause keeps qualification advice tied to its source event.
// source is a compile-time UUID expression, never request input.
func CurrentLeadEngagementClause(source string, asOf time.Time, arg func(any) int) string {
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	return `EXISTS (SELECT 1 FROM activity lead_source WHERE lead_source.id = ` + source + `
 AND lead_source.restricted_at IS NULL AND ` + SourceWithinHorizonSQL("lead_source", asOf, arg) + `
 AND ((` + InboundReplySQL("lead_source") + `
 AND ` + occurredday.BegunSQL("lead_source", instant) + ` AND NOT ` + AnsweredSQL("lead_source", instant) + `)
 OR (lead_source.kind = 'meeting' AND ` + relstrength.MeetingCountsSQL("lead_source") + `)))`
}

// TaskDueSQL removes a generated notification timestamp from deadline reads.
// Human changes keep their stored date; historical task records remain untouched.
func TaskDueSQL(alias string) string {
	sources := taskSourcesWith(func(p taskSourcePolicy) bool { return p.generatedDate })
	return `CASE WHEN ` + alias + `.kind = 'task' AND ` + alias + `.source_system IN ('` + strings.Join(sources, "','") + `')
 AND ` + alias + `.due_at = ` + alias + `.occurred_at AND NOT ` + humanSetDueSQL(alias) + ` THEN NULL ELSE ` + alias + `.due_at END`
}

func humanSetDueSQL(alias string) string {
	return `EXISTS (SELECT 1 FROM audit_log dated WHERE dated.entity_type='activity'
 AND dated.entity_id=` + alias + `.id AND dated.actor_type='human' AND dated.after ? 'due_at'
 AND NOT ` + AutomaticTaskRepairAuditClause("dated") + `
 AND ` + AuditNotFromAnImport("dated") + `)`
}

// Later advice replaces an automatic cue even if its original source becomes current again.
// Human adoption is admitted separately by CurrentTaskClause.
func latestLeadAdviceSQL(alias string) string {
	return `NOT EXISTS (SELECT 1 FROM activity successor
 JOIN activity_link successor_link ON successor_link.activity_id = successor.id
 JOIN activity_link original_link ON original_link.lead_id = successor_link.lead_id
 WHERE original_link.activity_id = ` + alias + `.id AND successor.source_system = ` + alias + `.source_system
 AND (successor.created_at, successor.id) > (` + alias + `.created_at, ` + alias + `.id))`
}
