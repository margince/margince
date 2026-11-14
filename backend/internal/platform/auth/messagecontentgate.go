// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ActivityMessageContentClause is ActivityContentClause for a reader whose
// rows are messages, never tasks. It answers the same for every task-free row
// and refuses every other, so a reader wrong about its rows sees less, never
// more. It leaves out the task hops, which keeps a statement that gates several
// messages small enough to plan.
func ActivityMessageContentClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	return messageContentClauses.render(p, alias, arg, messageContentGateFor), nil
}

// taskFreeSQL is "row alias is no task that stands for a source message". It
// is no captured request, promise or lead reply, and no task naming a source.
// Every task hop of the content gate is shut for such a row.
func taskFreeSQL(alias string) string {
	return fmt.Sprintf(`(coalesce(%[1]s.source_system, '') NOT IN ('%[2]s', '%[3]s', '%[4]s') AND (%[1]s.kind <> 'task' OR %[1]s.source_activity_id IS NULL))`,
		alias, provenance.EmailRequestSource, provenance.CommitmentTaskSource, provenance.LeadRepliedSource)
}

// messageContentGateFor is contentGateFor on a task-free row (taskFreeSQL),
// where the task hops are false and drop out, about two thirds of its size.
// It refuses every other row, so it never admits what contentGateFor denies.
func messageContentGateFor(p principal.Principal, alias string, arg func(any) int) string {
	gate := taskFreeSQL(alias) + " AND " + ActivityAvailableClause(alias) + " AND " + discoverReach(p, alias, arg, nil)
	if p.Type == principal.PrincipalSystem {
		return gate
	}
	return gate + " AND " + activityAudienceArm(p, alias, arg) + agentRestrictionArm(p, alias)
}
