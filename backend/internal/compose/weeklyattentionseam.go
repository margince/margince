// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What needs a manager's attention now, for the team weekly's read.
//
// In compose because both halves read `deal` and the findings list already
// lives here; compose/weekly may not import this package.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WeeklyAttention binds the team read's live lists. Stateless: every read
// runs on the caller's own transaction.
type WeeklyAttention struct{}

var _ weekly.TeamAttention = WeeklyAttention{}

// TeamExceptions are the open findings on deals the members own now.
func (WeeklyAttention) TeamExceptions(
	ctx context.Context, tx pgx.Tx, members []ids.UUID, limit int,
) ([]crmcontracts.InputCheck, error) {
	found, err := TeamAssuranceExceptions(ctx, tx, members, limit)
	if err != nil {
		return nil, err
	}
	return assurance.ExceptionsToWire(found), nil
}

// StalledDealsOf are the members' open deals quiet past the stalled period,
// quietest first. The stalled rule is the deals module's own spelling.
//
// A caller without deal read gets no deals rather than a refusal. The team read
// has already admitted them, and an empty list is what they may see.
func (WeeklyAttention) StalledDealsOf(
	ctx context.Context, tx pgx.Tx, members []ids.UUID, limit int,
) ([]weekly.StaleDeal, error) {
	if !auth.ReadGranted(ctx, tableDeal) || len(members) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlUnnarrowed
	}
	quiet := idlebase.SQL("d")
	query := fmt.Sprintf(`
		SELECT d.id, d.name, d.owner_id, %[1]s,
		       (extract(epoch FROM now() - %[1]s) / 86400)::int
		  FROM deal d
		 WHERE d.archived_at IS NULL AND d.owner_id = ANY($%[2]d) AND %[3]s AND (%[4]s)
		 ORDER BY %[1]s, d.id
		 LIMIT $%[5]d`, quiet, arg(members), deals.StalledSQL("d"), scope, arg(limit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the team's stalled deals: %w", err)
	}
	stale, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (weekly.StaleDeal, error) {
		var d weekly.StaleDeal
		err := row.Scan(&d.DealID, &d.DealName, &d.OwnerID, &d.LastActivityAt, &d.DaysQuiet)
		return d, err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the team's stalled deals: %w", err)
	}
	return stale, nil
}
