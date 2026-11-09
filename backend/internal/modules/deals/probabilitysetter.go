// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// probabilitySettersSQL reads, per named deal, the member behind the newest
// audit row that chose its probability: who typed it, or whom an agent acted
// for. An Undo puts back the choice from before the correction it reverses, so
// its row names whoever made that choice. An archived seat names nobody.
var probabilitySettersSQL = fmt.Sprintf(`
	SELECT named.id, setter.id, setter.display_name
	  FROM unnest($1::uuid[]) AS named(id)
	  CROSS JOIN LATERAL (
	    SELECT n.actor_type, n.actor_id, n.on_behalf_of, n.evidence->>'%[1]s' AS reverses FROM audit_log n
	     WHERE %[2]s ORDER BY n.occurred_at DESC, n.id DESC LIMIT 1) newest
	  CROSS JOIN LATERAL (
	    SELECT newest.actor_type, newest.actor_id, newest.on_behalf_of WHERE newest.reverses IS NULL
	    UNION ALL
	    (SELECT w.actor_type, w.actor_id, w.on_behalf_of
	       FROM deal_correction c JOIN audit_log taken ON taken.id = c.audit_log_id
	       JOIN audit_log w ON %[3]s AND w.evidence->>'%[1]s' IS NULL AND w.occurred_at < taken.occurred_at
	      WHERE c.id::text = newest.reverses
	      ORDER BY w.occurred_at DESC, w.id DESC LIMIT 1)) a`+auditattribution.ActorNameJoins+`
	  JOIN app_user setter ON setter.id = COALESCE(actor_user.id, obo.id) AND %[4]s`,
	reversesCorrectionEvidenceKey, choseProbabilitySQL("n"), choseProbabilitySQL("w"), workforce.NamedSeatSQL("setter"))

// choseProbabilitySQL is an audit row on the named deal that left a
// probability a colleague set. It wrote the source as human, or changed only the
// value. A colleague who confirms Margince's value writes the source alone; a
// write that clears the probability chooses nothing. alias is a literal.
func choseProbabilitySQL(alias string) string {
	return fmt.Sprintf(`%[1]s.entity_type = 'deal' AND %[1]s.entity_id = named.id
	 AND %[1]s.after ?| array['%[2]s', '%[3]s']
	 AND CASE WHEN %[1]s.after ? '%[3]s' THEN %[1]s.after->>'%[3]s' = '%[4]s'
	          ELSE %[1]s.after->>'%[2]s' IS NOT NULL END`,
		alias, winProbabilityOverrideColumn, winProbabilitySetByColumn, SourceHuman)
}

// stampProbabilitySetters names who set each deal's own probability where a
// colleague set it, for a reader auditattribution.ReaderMayKnowWhoActed
// admits. A deal that follows its stage, or whose probability Margince set,
// names nobody.
func stampProbabilitySetters(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal) error {
	if !auditattribution.ReaderMayKnowWhoActed(ctx) {
		return nil
	}
	var named []ids.UUID
	for i := range deals {
		source := deals[i].WinProbabilitySource
		if source != nil && *source == crmcontracts.DealWinProbabilitySource(SourceHuman) {
			named = append(named, ids.UUID(deals[i].Id))
		}
	}
	if len(named) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, probabilitySettersSQL, named)
	if err != nil {
		return fmt.Errorf("deals: reading who set the deals' probability: %w", err)
	}
	defer rows.Close()
	type setter struct {
		id   *ids.UUID
		name *string
	}
	setters := make(map[ids.UUID]setter, len(named))
	for rows.Next() {
		var deal ids.UUID
		var who setter
		if err := rows.Scan(&deal, &who.id, &who.name); err != nil {
			return fmt.Errorf("deals: reading who set a deal's probability: %w", err)
		}
		setters[deal] = who
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("deals: reading who set the deals' probability: %w", err)
	}
	for i := range deals {
		who, found := setters[ids.UUID(deals[i].Id)]
		if found && who.id != nil {
			id := openapi_types.UUID(*who.id)
			deals[i].WinProbabilitySetByUserId, deals[i].WinProbabilitySetByName = &id, who.name
		}
	}
	return nil
}
