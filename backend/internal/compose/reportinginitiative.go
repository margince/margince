// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// initiativeMembership is who belongs to the selected initiative, read once
// per evaluation under the reader's row scope and handed to every metric
// reader through the context. Sourced deals are the member deals a member
// lead was promoted into after the launch: the one evidenced origin.
type initiativeMembership struct {
	leads, deals, baseline, sourced []ids.UUID
	launch                          time.Time
}

type initiativeMembershipKey struct{}

func withInitiativeMembership(ctx context.Context, members *initiativeMembership) context.Context {
	return context.WithValue(ctx, initiativeMembershipKey{}, members)
}

func initiativeMembershipOf(ctx context.Context) *initiativeMembership {
	members, ok := ctx.Value(initiativeMembershipKey{}).(*initiativeMembership)
	if !ok {
		return nil
	}
	return members
}

// reportingInitiativeContext names the selected initiative on the frame. The
// read needs the initiative grant; a missing or archived one is not found.
func reportingInitiativeContext(ctx context.Context, tx pgx.Tx, selection crmcontracts.ReportingSelection, out *crmcontracts.ReportingContext) error {
	if selection.InitiativeId == nil {
		return nil
	}
	if !auth.ReadGranted(ctx, "sales_initiative") {
		return apperrors.ErrPermissionDenied
	}
	var name string
	var launch *time.Time
	err := tx.QueryRow(ctx, `SELECT name, launch_at FROM sales_initiative WHERE id = $1 AND archived_at IS NULL`, *selection.InitiativeId).Scan(&name, &launch)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("the selected initiative does not exist: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return err
	}
	out.InitiativeId, out.InitiativeLabel, out.InitiativeLaunchAt = selection.InitiativeId, &name, launch
	return nil
}

// loadInitiativeMembership reads the membership the metric readers narrow by.
func loadInitiativeMembership(ctx context.Context, tx pgx.Tx, store *initiatives.Store, frame crmcontracts.ReportingContext) (*initiativeMembership, error) {
	if frame.InitiativeId == nil || store == nil {
		return nil, nil
	}
	id := ids.UUID(*frame.InitiativeId)
	membership, err := store.MembershipOf(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	out := &initiativeMembership{leads: membership.Leads, deals: membership.Deals, launch: membership.Launch}
	rows, err := tx.Query(ctx, `SELECT deal_id FROM sales_initiative_baseline WHERE initiative_id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("read the launch baseline: %w", err)
	}
	if out.baseline, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID]); err != nil {
		return nil, err
	}
	if out.launch.IsZero() {
		return out, nil
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT l.qualified_deal_id FROM lead l WHERE l.id = ANY($1) AND l.qualified_deal_id = ANY($2) AND l.promoted_at >= $3`, out.leads, out.deals, out.launch)
	if err != nil {
		return nil, fmt.Errorf("read the sourced deals: %w", err)
	}
	out.sourced, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	return out, err
}

// initiativeMemberSQL narrows rows aliased `alias` to the members of the
// selected initiative, or adds nothing when none is selected.
func initiativeMemberSQL(ctx context.Context, b *reportingBindings, alias string, recordType string) string {
	members := initiativeMembershipOf(ctx)
	if members == nil {
		return ""
	}
	set := members.deals
	if recordType == string(recordTypeLead) {
		set = members.leads
	}
	return " AND " + alias + ".id = ANY(" + b.add(set) + ")"
}

// reportingInitiativeOriginLabels name the groups of the initiative pipeline:
// where each open member deal came from, against the launch.
var reportingInitiativeOriginLabels = map[string]langcopy.Phrase{
	"existing":      {En: "Existing at launch", De: "Beim Start vorhanden", Vi: "Có sẵn khi khởi động"},
	"sourced":       {En: "Sourced by the initiative", De: "Durch die Initiative gewonnen", Vi: "Do sáng kiến mang về"},
	"created_after": {En: "Created after launch", De: "Nach dem Start angelegt", Vi: "Tạo sau khi khởi động"},
	"added_after":   {En: "Added after launch", De: "Nach dem Start hinzugefügt", Vi: "Thêm sau khi khởi động"},
}

// readReportingInitiativePipeline reads the open member deals of the
// selected initiative, each under its origin. Without an initiative there is
// no population to read.
func readReportingInitiativePipeline(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	members := initiativeMembershipOf(ctx)
	if members == nil {
		return nil, reportingGap("unsupported", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.ChooseInitiative)), nil
	}
	said := reporting.ExplanationCopy(frame)
	query := openMetricQuery(false)
	query.group = func(_ context.Context, b *reportingBindings) (string, string, error) {
		origin := "CASE WHEN t.id = ANY(" + b.add(members.baseline) + ") THEN 'existing' WHEN t.id = ANY(" + b.add(members.sourced) + ") THEN 'sourced' WHEN t.created_at >= " + b.add(members.launch) + " THEN 'created_after' ELSE 'added_after' END"
		label := "CASE " + origin
		for key, phrase := range reportingInitiativeOriginLabels {
			label += " WHEN '" + key + "' THEN " + b.add(said.Say(phrase))
		}
		return "'origin:'||" + origin, label + " END", nil
	}
	return readDealMetricFacts(ctx, tx, frame, reportingInitiativePipeline, query)
}

// readReportingQualifiedCount counts the distinct deals that reached a
// qualifying stage in the period. Under an initiative it counts the sourced
// deals alone: membership says a deal belongs, never that the effort made it.
func readReportingQualifiedCount(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, framework crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if !reportingHasQualification(frame, framework) {
		return nil, reportingGap("not_configured", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.Qualification)), nil
	}
	query := qualifiedMetricQuery(framework)
	query.value, query.money = "1::float8", false
	if members := initiativeMembershipOf(ctx); members != nil {
		query.binds = map[string]any{"sourced": members.sourced}
		query.where += " AND t.id = ANY(@sourced)"
	}
	return readDealMetricFacts(ctx, tx, frame, reportingQualifiedOpportunitiesCreated, query)
}

// requireInitiativeRead is the grant an initiative dimension needs on top of
// the metric's own: the reader must be able to open the initiative itself.
func requireInitiativeRead(ctx context.Context, frame crmcontracts.ReportingContext) error {
	if frame.InitiativeId == nil {
		return nil
	}
	return auth.Require(ctx, "sales_initiative", principal.ActionRead)
}
