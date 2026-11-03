// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The candidate reads behind the Account coverage views: the open deals of a
// population, the accounts behind them, and the accounts won within the
// window. Every read carries the deal and company row scope, so a candidate
// is one the reader may open.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// coverageScope is the population clause with both row scopes, and the
// arguments the statement takes so far. arg registers one more argument and
// answers its position, the list builders' convention.
type coverageScope struct {
	where string
	args  []any
	arg   func(v any) int
}

// scopedPopulation renders the population clause and the deal and company
// row scopes as one `WHERE` fragment over aliases d and c.
func scopedPopulation(ctx context.Context, pop *coveragePopulation) (*coverageScope, error) {
	scope := &coverageScope{where: "true", args: append([]any{}, pop.args...)}
	scope.arg = func(v any) int {
		scope.args = append(scope.args, v)
		return len(scope.args)
	}
	if err := auth.Require(ctx, tableDeal, principal.ActionRead); err != nil {
		return scope, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return scope, err
	}
	if pop.clause != "" {
		scope.where = pop.clause
	}
	for _, table := range []struct{ name, alias string }{{tableDeal, "d"}, {"company", "c"}} {
		clause, err := auth.ScopeClauseFor(ctx, table.name, table.alias, scope.arg)
		if err != nil {
			return scope, err
		}
		if clause != "" {
			scope.where += " AND " + clause
		}
	}
	return scope, nil
}

// baseValue renders the deal's value in the installation currency under alias
// d, NULL where the reader's field mask withholds the amount.
func (scope *coverageScope) baseValue(ctx context.Context, pop *coveragePopulation) (string, error) {
	asOf := fmt.Sprintf("$%d::date", scope.arg(storekit.WorkspaceDay(pop.now, pop.zone)))
	return auth.MaskedExpressionSQL(ctx, tableDeal, "amount_minor", "d",
		deals.BaseValueSQL(asOf, installationBaseCurrencySQL, "d"), scope.arg)
}

// openDealsOf lists the population's open deals, largest first, with the
// account each belongs to.
func openDealsOf(ctx context.Context, tx pgx.Tx, pop *coveragePopulation) ([]crmcontracts.AccountCoverageRow, error) {
	scope, err := scopedPopulation(ctx, pop)
	if err != nil {
		return nil, err
	}
	amount, err := auth.MaskedColumnSQL(ctx, tableDeal, "amount_minor", "d", "amount_minor", scope.arg)
	if err != nil {
		return nil, err
	}
	value, err := scope.baseValue(ctx, pop)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.id, d.name, %s, d.currency, s.name, d.owner_id, d.expected_close_date,
		       c.id, c.display_name, c.owner_id
		  FROM deal d
		  JOIN stage s ON s.id = d.stage_id
		  JOIN company c ON c.id = d.company_id AND c.archived_at IS NULL
		 CROSS JOIN LATERAL (SELECT %s AS base_minor) v
		 WHERE d.status = 'open' AND d.archived_at IS NULL AND %s
		 ORDER BY v.base_minor DESC NULLS LAST, d.id
		 LIMIT %d`, amount, value, scope.where, coverageCandidates), scope.args...)
	if err != nil {
		return nil, fmt.Errorf("read the population's open deals: %w", err)
	}
	defer rows.Close()
	var out []crmcontracts.AccountCoverageRow
	for rows.Next() {
		var row crmcontracts.AccountCoverageRow
		var deal crmcontracts.AccountCoverageDeal
		var stage string
		var closeDate *time.Time
		if err := rows.Scan(&deal.Id, &deal.Name, &deal.AmountMinor, &deal.Currency, &stage, &deal.OwnerId, &closeDate, &row.Company.Id, &row.Company.Name, &row.Company.OwnerId); err != nil {
			return nil, fmt.Errorf("read an open deal: %w", err)
		}
		deal.Stage = &stage
		if closeDate != nil {
			deal.ExpectedCloseDate = &openapi_types.Date{Time: *closeDate}
		}
		row.Deal = &deal
		out = append(out, row)
	}
	pop.noteCap(len(out))
	return out, rows.Err()
}

// accountsInPipeline lists the accounts with an open deal in the population,
// with how many and how much, largest first. The sum is in the installation
// currency, and absent when a deal carries no rate or a masked amount.
func accountsInPipeline(ctx context.Context, tx pgx.Tx, pop *coveragePopulation) ([]crmcontracts.AccountCoverageRow, error) {
	scope, err := scopedPopulation(ctx, pop)
	if err != nil {
		return nil, err
	}
	value, err := scope.baseValue(ctx, pop)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.display_name, c.owner_id, count(d.id),
		       CASE WHEN bool_and(v.base_minor IS NOT NULL) THEN sum(v.base_minor) END
		  FROM deal d
		  JOIN company c ON c.id = d.company_id AND c.archived_at IS NULL
		 CROSS JOIN LATERAL (SELECT %s AS base_minor) v
		 WHERE d.status = 'open' AND d.archived_at IS NULL AND %s
		 GROUP BY c.id, c.display_name, c.owner_id
		 ORDER BY sum(v.base_minor) DESC NULLS LAST, c.id
		 LIMIT %d`, value, scope.where, coverageCandidates), scope.args...)
	if err != nil {
		return nil, fmt.Errorf("read the accounts in pipeline: %w", err)
	}
	defer rows.Close()
	var out []crmcontracts.AccountCoverageRow
	for rows.Next() {
		var row crmcontracts.AccountCoverageRow
		var open int
		if err := rows.Scan(&row.Company.Id, &row.Company.Name, &row.Company.OwnerId, &open, &row.OpenAmountMinor); err != nil {
			return nil, fmt.Errorf("read an account in pipeline: %w", err)
		}
		row.OpenDeals = &open
		out = append(out, row)
	}
	pop.noteCap(len(out))
	return out, rows.Err()
}

// wonAccounts lists the accounts whose newest won deal of the population
// closed within the window and which hold no open deal. The open-deal test
// is not row-scoped. An open deal a colleague holds still means the account
// is in pipeline, and nothing about that deal is shown.
func wonAccounts(ctx context.Context, tx pgx.Tx, pop *coveragePopulation) ([]crmcontracts.AccountCoverageRow, error) {
	scope, err := scopedPopulation(ctx, pop)
	if err != nil {
		return nil, err
	}
	since := scope.arg(pop.now.AddDate(0, 0, -*pop.thresholds.WonWithinDays))
	amount, err := auth.MaskedColumnSQL(ctx, tableDeal, "amount_minor", "d", "amount_minor", scope.arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT won.company_id, won.company_name, won.company_owner_id,
		       won.deal_id, won.deal_name, won.amount_minor, won.currency, won.deal_owner_id, won.closed_at
		  FROM (
		  SELECT DISTINCT ON (c.id) c.id AS company_id, c.display_name AS company_name, c.owner_id AS company_owner_id,
		         d.id AS deal_id, d.name AS deal_name, %s AS amount_minor, d.currency, d.owner_id AS deal_owner_id, d.closed_at
		    FROM deal d
		    JOIN company c ON c.id = d.company_id AND c.archived_at IS NULL
		   WHERE d.status = 'won' AND d.archived_at IS NULL AND d.closed_at >= $%d AND %s
		     AND NOT EXISTS (SELECT 1 FROM deal o WHERE o.company_id = c.id AND o.status = 'open' AND o.archived_at IS NULL)
		   ORDER BY c.id, d.closed_at DESC, d.id
		) won
		 ORDER BY won.closed_at DESC
		 LIMIT %d`, amount, since, scope.where, coverageCandidates), scope.args...)
	if err != nil {
		return nil, fmt.Errorf("read the won accounts: %w", err)
	}
	defer rows.Close()
	var out []crmcontracts.AccountCoverageRow
	for rows.Next() {
		var row crmcontracts.AccountCoverageRow
		var won crmcontracts.AccountCoverageDeal
		if err := rows.Scan(&row.Company.Id, &row.Company.Name, &row.Company.OwnerId, &won.Id, &won.Name, &won.AmountMinor, &won.Currency, &won.OwnerId, &won.ClosedAt); err != nil {
			return nil, fmt.Errorf("read a won account: %w", err)
		}
		row.WonDeal = &won
		out = append(out, row)
	}
	pop.noteCap(len(out))
	return out, rows.Err()
}

// noteCap records that a candidate read filled its cap, so the counts are
// over the largest candidates rather than every one.
func (pop *coveragePopulation) noteCap(read int) {
	if read >= coverageCandidates {
		pop.truncated = true
	}
}
