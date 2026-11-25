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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dayrule"
	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// WeeklyAttention binds the team read's live lists. Stateless: every read
// runs on the caller's own transaction.
type WeeklyAttention struct{}

var _ weekly.TeamAttention = WeeklyAttention{}

// TeamExceptions are the open findings on deals the members own now and on
// the team's standing calls.
func (WeeklyAttention) TeamExceptions(
	ctx context.Context, tx pgx.Tx, teamID ids.UUID, members []ids.UUID, limit int,
) ([]crmcontracts.InputCheck, error) {
	found, err := TeamAssuranceExceptions(ctx, tx, teamID, members, limit)
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
	zone, err := dayrule.Zone(ctx, tx, occurredday.Combined, ids.Nil)
	if err != nil {
		return nil, err
	}
	quiet := idlebase.SQL("d")
	query := fmt.Sprintf(`
		SELECT d.id, d.name, d.owner_id, %[1]s, %[2]s, now()
		  FROM deal d
		 WHERE d.archived_at IS NULL AND d.owner_id = ANY($%[3]d) AND %[4]s AND (%[5]s)
		 ORDER BY %[1]s, d.id
		 LIMIT $%[6]d`, quiet, deals.LastActivityDaySQL("d"), arg(members), deals.StalledSQL("d"), scope, arg(limit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the team's stalled deals: %w", err)
	}
	// A date-only newest activity counts from its own date, so a team far east
	// of UTC does not read the deal a day less quiet than it is.
	stale, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (weekly.StaleDeal, error) {
		var d weekly.StaleDeal
		var since occurredday.Moment
		var asOf time.Time
		if err := row.Scan(&d.DealID, &d.DealName, &d.OwnerID, &since.At, &since.DateOnly, &asOf); err != nil {
			return d, err
		}
		d.LastActivityAt, d.DaysQuiet = since.At, occurredday.DaysSince(since, asOf, zone)
		return d, nil
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading the team's stalled deals: %w", err)
	}
	return stale, nil
}
