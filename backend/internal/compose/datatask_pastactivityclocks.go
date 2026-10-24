// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastActivityClocksDataTask moves back the stored clocks the rule before
// 1792656701 left in the future, through the refold the job runs.
func pastActivityClocksDataTask() datatask.Task {
	audit := datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"}
	return datatask.Task{
		ID: "core/1792656701_past_activity_clocks", Version: 1,
		Title: "Move last-activity dates back to what has happened", Requires: []string{"core/1792656701"},
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
	return clockRefoldStep(name, table, audit, clockSelection{
		count: storekit.CountFutureClocks, next: storekit.FutureClocks,
	})
}

// clockSelection is how one clock repair finds the records it refolds. It
// holds a bounded count for the estimate and the next page by id for a batch.
type clockSelection struct {
	count func(context.Context, storekit.ClockQuerier, storekit.ClockTable, int64) (int64, error)
	next  func(context.Context, storekit.ClockQuerier, storekit.ClockTable, ids.UUID, int) ([]ids.UUID, error)
}

// clockRefoldStep walks one table by id and refolds each record the selection
// names. The selection must drop a refolded record, so a second run finds
// nothing.
func clockRefoldStep(name string, table storekit.ClockTable, audit datatask.AuditPolicy, selection clockSelection) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 200, MinLimit: 10, Audit: audit,
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := selection.count(ctx, q, table, maximum)
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
			selected, err := selection.next(ctx, tx, table, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if err := refoldClocksTx(ctx, tx, clockTargets(table, selected)); err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, selected, len(selected))
		},
	}
}

// clockTargets files the records of one table under that table's targets.
func clockTargets(table storekit.ClockTable, records []ids.UUID) storekit.LastActivityTargets {
	var targets storekit.LastActivityTargets
	switch table {
	case storekit.ClockCompanies:
		targets.Companies = records
	case storekit.ClockDeals:
		targets.Deals = records
	case storekit.ClockProjects:
		targets.Projects = records
	default:
		targets.Contacts = records
	}
	return targets
}
