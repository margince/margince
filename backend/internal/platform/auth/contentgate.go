// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// contentGateSource names the task-source probe that each content gate and
// audience arm renders once, and nothing else renders. Counting it counts the
// copies of the gate a statement asks Postgres to plan.
const contentGateSource = "content_gate_source"

// ContentGateCopies answers how many content gates, or audience arms, sql
// carries. Postgres plans each copy, and a pooled connection keeps that memory.
func ContentGateCopies(sql string) int {
	return strings.Count(sql, "WHERE "+contentGateSource+".id = ")
}

// ContentReadableActivitiesSQL renders the ids among need whose content the
// caller may read, under one copy of the content gate. A statement that gates
// several aliases materializes this once and tests membership in it. need is a
// query that yields activity ids, built from compile-time SQL and placeholders.
func ContentReadableActivitiesSQL(ctx context.Context, need string, arg func(any) int) (string, error) {
	gate, err := ActivityContentClause(ctx, "readable", arg)
	if err != nil {
		return "", err
	}
	// need is far smaller than activity, and the planner cannot know it, so it
	// would walk the whole table. The array makes it look each id up instead.
	return "SELECT readable.id FROM activity readable WHERE readable.id = ANY(ARRAY(" + need + ")) AND " + gate, nil
}

// contentAudienceArm is the audience half of the content gate, with audience
// testing the row itself. Source availability is independent of the task's
// audience: a reminder cannot grant access to evidence the reader can no
// longer open.
func contentAudienceArm(p principal.Principal, alias string, arg func(any) int, audience clauseBuilder) string {
	own := audience(p, alias, arg)
	// Reassignment hands over the reminder, never a grant to its source email.
	own = fmt.Sprintf("(%s OR coalesce(%s.source_system = '%s' AND %s.assignee_id = $%d, false))",
		own, alias, provenance.EmailRequestSource, alias, arg(p.UserID))
	return own + " AND (" + taskFreeSQL(alias) + " OR " + taskEvidenceReadable(p, alias, arg) + ")" +
		fmt.Sprintf(" AND (coalesce(%s.source_system, '') <> '%s' OR %s)",
			alias, provenance.CommitmentTaskSource, commitmentEvidenceRetained(alias+".source_activity_id", alias+".source_id")) +
		agentRestrictionArm(p, alias)
}

// taskEvidenceReadable is "the caller may read every message a task stands
// for": its source, and each message that renewed its promise. A captured
// request is a personal reminder of its source, not a new grant to that
// correspondence, so archiving the source withholds it too. The messages share
// one message gate, as Postgres keeps a plan of every copy per connection.
func taskEvidenceReadable(p principal.Principal, alias string, arg func(any) int) string {
	needed := fmt.Sprintf(`SELECT %[2]s AS id, true AS hop
	 UNION ALL SELECT renewal.renewed_by, false FROM activity_commitment_admission renewal
	  WHERE coalesce(%[1]s.source_system, '') = '%[3]s' AND renewal.source_activity_id = %[1]s.source_activity_id
	    AND renewal.locator = %[1]s.source_id AND renewal.renewed_by IS NOT NULL`,
		alias, provenance.TaskSourceActivitySQL(alias), provenance.CommitmentTaskSource)
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM (%[1]s) needed WHERE NOT EXISTS (SELECT 1 FROM activity %[2]s
	   WHERE %[2]s.id = needed.id AND %[2]s.restricted_at IS NULL
	     AND (NOT needed.hop OR coalesce(%[3]s.source_system, '') <> '%[4]s' OR %[2]s.archived_at IS NULL)
	     AND %[5]s))`, needed, contentGateSource, alias, provenance.EmailRequestSource, messageContentGateFor(p, contentGateSource, arg))
}
