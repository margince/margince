// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sdrTeamTargets reads members' own targets, as the reporting service holds
// them. The team review asks it once for the whole page of members.
type sdrTeamTargets interface {
	OwnerTargets(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, metric crmcontracts.ReportingMetricID,
		window crmcontracts.ReportingWindow, owners []ids.UUID) (map[ids.UUID]int64, bool, error)
}

// sdrTeamLeads are one member's lead figures. monthQualified is the qualified
// rule over the target month, the actual a monthly target stands against.
type sdrTeamLeads struct{ qualified, unanswered, monthQualified int }

// readSdrTeamLeads counts each seat's leads by the predicates the Leads
// qualified and Leads still waiting for a reply readings use, under the same
// row scope and field masks, in one statement for every seat. A lead counts
// for its owner now. month is nil when no target is read.
func readSdrTeamLeads(ctx context.Context, tx pgx.Tx, seats []ids.UUID, window sdrTeamWindow, month *sdrTeamWindow, at time.Time) (map[ids.UUID]sdrTeamLeads, error) {
	var b reportingBindings
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeLead), "l", b.arg)
	if err != nil {
		return nil, err
	}
	qualifiedScope, _, err := reportingFieldScope(ctx, string(recordTypeLead), "l", reportingLeadFields(reportingLeadsPromoted), &b)
	if err != nil {
		return nil, err
	}
	awaitingScope, _, err := reportingFieldScope(ctx, string(recordTypeLead), "l", reportingLeadFields(reportingLeadsAwaitingResponse), &b)
	if err != nil {
		return nil, err
	}
	cutoff, from, to := b.add(at), b.add(window.from), b.add(window.to)
	qualified := leadQualifiedBySQL(cutoff) + " AND " + qualifiedScope
	createdIn := func(start, end string) string { return "l.created_at>=" + start + " AND l.created_at<" + end }
	monthQualified, earliest := "false", window.from
	if month != nil {
		monthQualified = createdIn(b.add(month.from), b.add(month.to)) + " AND " + qualified
		if month.from.Before(earliest) {
			earliest = month.from
		}
	}
	since := b.add(earliest)
	query := `SELECT l.owner_id,count(*) FILTER (WHERE ` + createdIn(from, to) + ` AND ` + qualified + `),
 count(*) FILTER (WHERE ` + leadArrivedBetweenSQL(from, to) + ` AND ` + leadAwaitingBySQL(cutoff) + ` AND ` + awaitingScope + `),
 count(*) FILTER (WHERE ` + monthQualified + `)
 FROM lead l WHERE ` + countedLeadSQL + ` AND l.owner_id=ANY(` + b.add(seats) + `) AND ` + orUnnarrowed(rowScope) + `
 AND (l.created_at>=` + since + ` OR ` + leadArrivalSQL + `>=` + since + `) GROUP BY l.owner_id`
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID]sdrTeamLeads{}
	for rows.Next() {
		var seat ids.UUID
		var figures sdrTeamLeads
		if err := rows.Scan(&seat, &figures.qualified, &figures.unanswered, &figures.monthQualified); err != nil {
			return nil, err
		}
		out[seat] = figures
	}
	return out, rows.Err()
}

// readerWithheldFigures names the figures the requesting reader's grants
// leave out of a review. A target held against leads the reader may not see
// says nothing, so targets need lead read as well as target read.
func readerWithheldFigures(ctx context.Context, workflow crmcontracts.SdrTeamReviewWorkflow) []crmcontracts.SdrTeamReviewWithheldFigures {
	out := []crmcontracts.SdrTeamReviewWithheldFigures{}
	leads := auth.Allows(ctx, string(recordTypeLead), principal.ActionRead)
	if !weekly.MayReadMeetings(ctx) {
		out = append(out, crmcontracts.SdrTeamReviewWithheldFiguresMeetings)
	}
	if workflow == crmcontracts.SdrTeamReviewWorkflowHandoff && !auth.Allows(ctx, contacts.HandoffObject, principal.ActionRead) {
		out = append(out, crmcontracts.SdrTeamReviewWithheldFiguresHandoffs)
	}
	if !leads {
		out = append(out, crmcontracts.SdrTeamReviewWithheldFiguresLeads)
	}
	if workflow == crmcontracts.SdrTeamReviewWorkflowQualification && (!leads || !auth.Allows(ctx, "sales_target", principal.ActionRead)) {
		out = append(out, crmcontracts.SdrTeamReviewWithheldFiguresTargets)
	}
	return out
}

// sdrTeamMemberFigures adds the lead figures and targets to the member rows,
// each under the requesting reader's own grant, and names the ones withheld.
type sdrTeamMemberFigures struct {
	targets  sdrTeamTargets
	scope    crmcontracts.ReportingScope
	workflow crmcontracts.SdrTeamReviewWorkflow
	calendar reporting.Calendar
}

func (f sdrTeamMemberFigures) apply(ctx context.Context, tx pgx.Tx, out *crmcontracts.SdrTeamReview, window sdrTeamWindow) error {
	seats := make([]ids.UUID, len(out.Members))
	for index, member := range out.Members {
		seats[index] = ids.UUID(member.Id)
	}
	if slices.Contains(out.WithheldFigures, crmcontracts.SdrTeamReviewWithheldFiguresHandoffs) {
		for index := range out.Members {
			member := &out.Members[index]
			member.Submitted, member.Accepted, member.Rejected, member.Recycled, member.Waiting, member.OldestWaitingSince = 0, 0, 0, 0, 0, nil
		}
	}
	if slices.Contains(out.WithheldFigures, crmcontracts.SdrTeamReviewWithheldFiguresLeads) {
		return nil
	}
	month, targets, err := f.readTargets(ctx, tx, out, window, seats)
	if err != nil {
		return err
	}
	leads, err := readSdrTeamLeads(ctx, tx, seats, window, month, out.AsOf)
	if err != nil {
		return err
	}
	for index := range out.Members {
		member := &out.Members[index]
		figures := leads[ids.UUID(member.Id)]
		member.Qualified, member.Unanswered = &figures.qualified, &figures.unanswered
		if value, ok := targets[ids.UUID(member.Id)]; ok {
			member.Target = &crmcontracts.SdrTeamMemberTarget{Value: value, Actual: figures.monthQualified}
		}
	}
	return nil
}

// readTargets reads the members' monthly targets for qualified leads, in
// qualification mode only, held against the month an evaluation with a
// monthly target basis reads.
func (f sdrTeamMemberFigures) readTargets(ctx context.Context, tx pgx.Tx, out *crmcontracts.SdrTeamReview, window sdrTeamWindow, seats []ids.UUID) (*sdrTeamWindow, map[ids.UUID]int64, error) {
	if f.workflow != crmcontracts.SdrTeamReviewWorkflowQualification || f.targets == nil ||
		slices.Contains(out.WithheldFigures, crmcontracts.SdrTeamReviewWithheldFiguresTargets) {
		return nil, nil, nil
	}
	target, err := reporting.TargetWindow(crmcontracts.ReportingWindow{StartAt: window.from, EndAt: window.to}, "month", f.calendar)
	if err != nil {
		return nil, nil, err
	}
	targets, readable, err := f.targets.OwnerTargets(ctx, tx, f.scope, reportingLeadsPromoted, target, seats)
	if err != nil || !readable {
		return nil, nil, err
	}
	out.MemberTargets = &crmcontracts.SdrTeamMemberTargets{Metric: crmcontracts.SdrTeamMemberTargetsMetricLeadsPromoted, Window: target}
	month := sdrTeamWindow{from: target.StartAt, to: target.EndAt}
	if out.AsOf.Before(month.to) {
		month.to = out.AsOf
	}
	return &month, targets, nil
}
