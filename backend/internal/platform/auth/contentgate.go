// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"strings"
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
