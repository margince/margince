// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// countAutoChangeReversals counts the automatic changes the team's members
// undid inside the week.
//
// A reversal is any audit row that names the entry it undoes, whatever its verb.
// Undoing an agent's create archives the record and undoing its archive
// restores it, so one action alone misses half. The undone entry counts only
// when an agent, a connector or the system wrote it. Members match on the
// principal id an audit row stores for a seat: "human:" and the user id.
func countAutoChangeReversals(
	ctx context.Context, tx pgx.Tx, members []FrozenMember, start, end time.Time,
) (int, error) {
	principals := make([]string, 0, len(members))
	for _, m := range members {
		principals = append(principals, "human:"+m.UserID.String())
	}
	if len(principals) == 0 {
		return 0, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	key, from, to, who := arg(storekit.EvidenceKeyUndidAuditLog), arg(start), arg(end), arg(principals)
	var n int
	// The key is matched as text before the cast, so one malformed evidence
	// value cannot fail the whole week.
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*)
		  FROM audit_log r
		  JOIN audit_log u ON u.id = CASE
		       WHEN r.evidence->>$%[1]d ~ '^[0-9a-fA-F-]{36}$' THEN (r.evidence->>$%[1]d)::uuid END
		 WHERE r.evidence ? $%[1]d
		   AND r.occurred_at >= $%[2]d AND r.occurred_at < $%[3]d
		   AND r.actor_type = 'human' AND r.actor_id = ANY($%[4]d)
		   AND u.actor_type IN ('agent', 'connector', 'system')`, key, from, to, who), args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("weekly: counting the team's undone automatic changes: %w", err)
	}
	return n, nil
}
