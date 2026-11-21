// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ScheduledSendReadableSQL is the predicate over the scheduled_send row alias
// for the rows the caller may read. An unsent message is its author's alone:
// its body and blind copies are not workspace-readable the way a sent one is.
// A principal with no human behind it reads none. alias is a compile-time
// literal.
func ScheduledSendReadableSQL(ctx context.Context, alias string, arg func(any) int) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() || actor.UserID.IsZero() {
		return scopeNothing, nil
	}
	return fmt.Sprintf("%s.scheduled_by = $%d", pgx.Identifier{alias}.Sanitize(), arg(actor.UserID)), nil
}

// ScheduledSendRecordsSQL selects the ids of the records of one type a
// scheduled send files under. Those are the records its account send names,
// its reply adds, or its anchor is linked to. alias and recordType are
// compile-time literals, and recordType is an activity link target.
func ScheduledSendRecordsSQL(alias, recordType string) string {
	row := pgx.Identifier{alias}.Sanitize()
	return fmt.Sprintf(`SELECT (l->>'EntityID')::uuid
		  FROM jsonb_array_elements(COALESCE(%[1]s.origin_links, '[]') || COALESCE(%[1]s.also_links, '[]')) l
		 WHERE l->>'EntityType' = '%[2]s'
		UNION ALL SELECT al.%[2]s_id FROM activity_link al
		 WHERE al.activity_id = %[1]s.anchor_activity_id AND al.entity_type = '%[2]s'`, row, recordType)
}
