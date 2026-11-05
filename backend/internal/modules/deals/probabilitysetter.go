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
)

// probabilitySettersSQL reads, per named deal, the member behind the newest
// audit row that wrote its probability. That is the human who typed it, or the
// human an agent acted for. A write of the source alone counts: a colleague who
// confirms Margince's value changes only the source.
var probabilitySettersSQL = fmt.Sprintf(`
	SELECT named.id, COALESCE(actor_user.id, obo.id), COALESCE(actor_user.display_name, obo.display_name)
	  FROM unnest($1::uuid[]) AS named(id)
	  CROSS JOIN LATERAL (
	    SELECT w.actor_type, w.actor_id, w.on_behalf_of FROM audit_log w
	     WHERE w.entity_type = 'deal' AND w.entity_id = named.id AND w.after ?| array['%s', '%s']
	     ORDER BY w.occurred_at DESC, w.id DESC LIMIT 1) a`+auditattribution.ActorNameJoins,
	winProbabilityOverrideColumn, winProbabilitySetByColumn)

// stampProbabilitySetters names who set each deal's own probability where a
// colleague set it. A deal that follows its stage, or whose probability
// Margince set, names nobody.
func stampProbabilitySetters(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal) error {
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
