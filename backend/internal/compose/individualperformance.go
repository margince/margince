// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's "Show individual performance" switch, applied where the
// analytics lens decides whose work a reader measures.
//
// Off, a reader still measures themselves and any team or company total. They
// lose every answer keyed to one colleague. They also lose every total small
// enough to be one: a team of two minus their own figure is the colleague's.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// individualPerformance answers the switch for this request: the value its door
// bound, or one read in the caller's transaction.
func individualPerformance(ctx context.Context, tx pgx.Tx) (bool, error) {
	if allowed, bound := auth.IndividualPerformanceOf(ctx); bound {
		return allowed, nil
	}
	return identity.IndividualPerformanceTx(ctx, tx)
}

// bindIndividualPerformance reads the switch once for a job run, so every
// member the run acts as answers to the same value.
func bindIndividualPerformance(ctx context.Context, pool *pgxpool.Pool) (context.Context, bool, error) {
	var allowed bool
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		allowed, err = identity.IndividualPerformanceTx(ctx, tx)
		return err
	})
	if err != nil {
		return ctx, false, fmt.Errorf("compose: reading whether individual performance is shown: %w", err)
	}
	return auth.WithIndividualPerformance(ctx, allowed), allowed, nil
}

// requireMeasurableSeat refuses naming anyone but the reader while the
// switch is off.
func requireMeasurableSeat(ctx context.Context, tx pgx.Tx, seat ids.UUID) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil {
		return err
	}
	if auth.MayMeasureSeat(ctx, allowed, seat) {
		return nil
	}
	return auth.IndividualPerformanceOffError{}
}

// smallestTotal is how many members a total must cover while the switch is off.
// With three, a total minus the reader's own figure still covers two colleagues.
const smallestTotal = 3

// requireTotalOfEnough refuses, while the switch is off, a population total
// that covers too few members to stay a total.
func requireTotalOfEnough(ctx context.Context, tx pgx.Tx, p principal.Principal, resolved ResolvedScope) error {
	if resolved.Kind == ScopeKindOwner {
		return nil
	}
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	var members string
	switch resolved.Kind {
	case ScopeKindTeam:
		members = teamMembersSQL(arg(*resolved.ID), "= $%d")
	case ScopeKindManagedTeams:
		members = fmt.Sprintf("SELECT $%d::uuid UNION %s", arg(p.UserID), teamMembersSQL(arg(p.TeamIDs), "= ANY($%d)"))
	default:
		members = "SELECT u.id FROM app_user u WHERE " + identity.LiveMemberSQL("u") + " AND NOT u.is_agent"
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ("+members+") population", args...).Scan(&count); err != nil {
		return fmt.Errorf("compose: counting the members a total covers: %w", err)
	}
	if count < smallestTotal {
		return auth.IndividualPerformanceOffError{}
	}
	return nil
}

// explainWithheldOwners says on each owner chart why colleagues are missing.
// Without it an empty chart reads as a team that sold nothing.
func explainWithheldOwners(ctx context.Context, tx pgx.Tx, out *reporting.Evaluation) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	reason := reporting.ExplanationCopy(out.Result.Context).Say(reportingCoverageWords.IndividualOff)
	for index := range out.Result.Charts {
		if chart := &out.Result.Charts[index]; chart.Kind == reportingOwnerAttainment {
			chart.Coverage.Withheld = true
			chart.Coverage.Reason = &reason
		}
	}
	return nil
}

// requireFrozenScopeMeasurable asks a frozen reading's population what a live
// one is asked on every read, while the switch is off. A snapshot of a
// colleague's forecast is that colleague's number, however old.
func requireFrozenScopeMeasurable(ctx context.Context, tx pgx.Tx, scope forecasting.Scope) error {
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return err
	}
	_, err = ResolveAnalyticsScope(ctx, tx, requestedFromForecastScope(scope))
	return err
}
