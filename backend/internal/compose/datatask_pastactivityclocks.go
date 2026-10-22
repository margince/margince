// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastActivityClocksDataTask moves back the stored clocks the rule before
// 1792645053 left in the future, through the refold the job runs.
func pastActivityClocksDataTask() datatask.Task {
	audit := datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"}
	return datatask.Task{
		ID: "core/1792645053_past_activity_clocks", Version: 1,
		Title: "Move last-activity dates back to what has happened", Requires: []string{"core/1792645053"},
		Steps: []datatask.Step{
			futureClockStep("contacts", storekit.ClockContacts, audit),
			futureClockStep("companies", storekit.ClockCompanies, audit),
			futureClockStep("deals", storekit.ClockDeals, audit),
		},
	}
}

// futureClockStep walks one table by id, refolding each record whose stored
// clock is later than now. A refolded record leaves the selection, so a
// second run finds nothing.
func futureClockStep(name string, table storekit.ClockTable, audit datatask.AuditPolicy) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 200, MinLimit: 10, Audit: audit,
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := storekit.CountFutureClocks(ctx, q, table, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			future, err := storekit.FutureClocks(ctx, tx, table, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			var targets storekit.LastActivityTargets
			switch table {
			case storekit.ClockCompanies:
				targets.Companies = future
			case storekit.ClockDeals:
				targets.Deals = future
			default:
				targets.Contacts = future
			}
			if err := refoldClocksTx(ctx, tx, targets); err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, future, len(future))
		},
	}
}
