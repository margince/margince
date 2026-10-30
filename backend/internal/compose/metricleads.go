// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// countedLeadSQL admits a lead that came in and stands for itself. A merged
// duplicate is its survivor's lead. An archive other than qualification or
// disqualification took the lead back.
const countedLeadSQL = "l.merged_into_id IS NULL AND (l.archived_at IS NULL OR l.status IN ('promoted','disqualified'))"

const leadOwnerColumn = "l.owner_id"

// leadLabelSQL labels a lead's row by its name or else its company. The company
// is used only where the seat may read it; otherwise the bound fallback is.
// A reader's field scope already withholds a lead whose name is masked.
func leadLabelSQL(ctx context.Context, b *reportingBindings, fallback string) (string, error) {
	company := "NULLIF(l.company_name,'')"
	visible, masked, err := auth.MaskExcludedClause(ctx, string(recordTypeLead), companyNameField, "l", b.arg)
	if err != nil {
		return "", err
	}
	if masked {
		company = "NULL"
		if visible != "" {
			company = "CASE WHEN " + visible + " THEN NULLIF(l.company_name,'') END"
		}
	}
	return "COALESCE(NULLIF(l.full_name,'')," + company + "," + fallback + ")", nil
}

func reportingLeadFields(metric crmcontracts.ReportingMetricID) []string {
	fields := []string{fieldOwnerID, reportingStatusField, fieldFullName, fieldCreatedAt}
	switch metric {
	case reportingLeadsCreated:
		fields = append(fields, "origin")
	case reportingLeadsPromoted:
		fields = append(fields, "qualified_deal_id")
	case reportingLeadFirstResponse, reportingLeadsAwaitingResponse:
		fields = append(fields, "routed_at", "first_response_at")
	}
	return fields
}

// leadMetricScope is what every lead reader narrows by: the picked scope, the
// leads this reader may see, and none whose read columns are withheld.
func leadMetricScope(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, b *reportingBindings) (string, bool, error) {
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), leadOwnerColumn, b.arg, unownedIsExcluded)
	if err != nil {
		return "", false, err
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeLead), "l", b.arg)
	if err != nil {
		return "", false, err
	}
	fieldScope, withheld, err := reportingFieldScope(ctx, string(recordTypeLead), "l", reportingLeadFields(metric), b)
	if err != nil {
		return "", false, err
	}
	return orUnnarrowed(population) + " AND " + orUnnarrowed(rowScope) + " AND " + fieldScope, withheld, nil
}

func orUnnarrowed(clause string) string {
	if clause == "" {
		return sqlUnnarrowed
	}
	return clause
}

func noPipelineGap(frame crmcontracts.ReportingContext) crmcontracts.ReportingCoverage {
	return reportingGap("unsupported", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.NoPipeline))
}

func readReportingLeadsCreated(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadsCreated, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	query := `SELECT l.id::text,l.id,` + label + `,l.owner_id,COALESCE(u.display_name,''),l.created_at,1::float8,'lead_created',
 'origin:'||COALESCE(l.origin,''),COALESCE(lo.label,` + b.add(said.Say(reportingBreakdownLabels.NoOrigin)) + `)
 FROM lead l LEFT JOIN lead_origin lo ON lo.key=l.origin LEFT JOIN app_user u ON u.id=l.owner_id
 WHERE ` + countedLeadSQL + ` AND l.created_at>=` + b.add(reportingEarliest(frame)) + ` AND l.created_at<` + b.add(frame.EvaluatedAt) + ` AND ` + scope + `
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	return readGroupedFacts(ctx, tx, frame, reportingLeadsCreated, string(recordTypeLead), query, b, withheld)
}

// readReportingLeadStatusMoves counts through weekly.LeadMovesSQL, the rule the
// weekly scorecard counts by, and keeps only the moves it names.
func readReportingLeadStatusMoves(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadStatusMoves, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	var label strings.Builder
	label.WriteString("CASE m.to_status")
	for _, status := range []crmcontracts.LeadStatus{crmcontracts.LeadStatusContacted, crmcontracts.LeadStatusEngaged, crmcontracts.LeadStatusPromoted, crmcontracts.LeadStatusDisqualified} {
		label.WriteString(" WHEN '" + string(status) + "' THEN " + b.add(said.Say(reportingLeadStatusLabels[status])))
	}
	label.WriteString(" END")
	leads := "(SELECT l.id FROM lead l WHERE " + scope + ") scoped"
	moves := weekly.LeadMovesSQL(leads, b.add(reportingEarliest(frame)), b.add(frame.EvaluatedAt))
	name, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	query := `SELECT m.audit_id::text,l.id,` + name + `,l.owner_id,COALESCE(u.display_name,''),m.occurred_at,1::float8,'lead_status_move',
 'status:'||m.to_status,` + label.String() + `
 FROM (` + moves + `) m JOIN lead l ON l.id=m.lead_id LEFT JOIN app_user u ON u.id=l.owner_id
 WHERE m.move IS NOT NULL ORDER BY m.audit_id LIMIT ` + b.add(reportingFactLimit+1)
	return readGroupedFacts(ctx, tx, frame, reportingLeadStatusMoves, string(recordTypeLead), query, b, withheld)
}

// readReportingLeadsPromoted reads every lead that came in, valued 1 when it
// was qualified into a deal and 0 when not. The cohort is the share's
// denominator and the sum its numerator.
func readReportingLeadsPromoted(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadsPromoted, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	at := b.add(frame.EvaluatedAt)
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	query := `SELECT l.id::text,l.id,` + label + `,l.owner_id,COALESCE(u.display_name,''),l.created_at,
 CASE WHEN l.status='promoted' AND l.qualified_deal_id IS NOT NULL AND l.promoted_at<` + at + ` THEN 1 ELSE 0 END::float8,'lead_created','',''
 FROM lead l LEFT JOIN app_user u ON u.id=l.owner_id
 WHERE ` + countedLeadSQL + ` AND l.created_at>=` + b.add(reportingEarliest(frame)) + ` AND l.created_at<` + at + ` AND ` + scope + `
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	return readGroupedFacts(ctx, tx, frame, reportingLeadsPromoted, string(recordTypeLead), query, b, withheld)
}

// leadArrivedSQL is the arrival cohort both response readings count from: a
// counted lead no import wrote, which arrived in the window ending at `at`.
func leadArrivedSQL(frame crmcontracts.ReportingContext, b *reportingBindings, at string) string {
	return countedLeadSQL + " AND " + weekly.NonImportedLeadSQL + " AND " + leadArrivalSQL + ">=" + b.add(reportingEarliest(frame)) + " AND " + leadArrivalSQL + "<" + at
}

// leadArrivalSQL is when a lead arrived: the instant the response target runs from.
const leadArrivalSQL = "COALESCE(l.routed_at,l.created_at)"

// readReportingLeadFirstResponse times each answered lead from when it arrived
// to its first response, in hours.
func readReportingLeadFirstResponse(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadFirstResponse, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	at := b.add(frame.EvaluatedAt)
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	query := `SELECT l.id::text,l.id,` + label + `,l.owner_id,COALESCE(u.display_name,''),` + leadArrivalSQL + `,
 GREATEST(EXTRACT(EPOCH FROM (l.first_response_at-` + leadArrivalSQL + `))/3600,0)::float8,'lead_first_response','',''
 FROM lead l LEFT JOIN app_user u ON u.id=l.owner_id
 WHERE ` + leadArrivedSQL(frame, &b, at) + ` AND l.first_response_at<` + at + ` AND ` + scope + `
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	return readGroupedFacts(ctx, tx, frame, reportingLeadFirstResponse, string(recordTypeLead), query, b, withheld)
}

// readReportingLeadsAwaitingResponse counts the leads of the same arrival
// cohort that still owe a first reply when the reading is taken. Each wrote
// in by email, nobody has answered it, and the lead is still open.
func readReportingLeadsAwaitingResponse(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadsAwaitingResponse, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	at := b.add(frame.EvaluatedAt)
	label, err := leadLabelSQL(ctx, &b, b.add(said.Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	query := `SELECT l.id::text,l.id,` + label + `,l.owner_id,COALESCE(u.display_name,''),` + leadArrivalSQL + `,1::float8,'lead_awaiting_response','',''
 FROM lead l LEFT JOIN app_user u ON u.id=l.owner_id
 WHERE ` + leadArrivedSQL(frame, &b, at) + ` AND l.archived_at IS NULL AND (l.first_response_at IS NULL OR l.first_response_at>=` + at + `)
 AND ` + auth.LiveLeadInquiryClause("l") + ` AND ` + scope + `
 ORDER BY l.id LIMIT ` + b.add(reportingFactLimit+1)
	return readGroupedFacts(ctx, tx, frame, reportingLeadsAwaitingResponse, string(recordTypeLead), query, b, withheld)
}

// readGroupedFacts scans a reader's rows of key, source, label, owner, owner
// label, instant, value, provenance, group key and group label into facts.
func readGroupedFacts(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID, source, query string, b reportingBindings, withheld bool) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	facts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reporting.Fact, error) {
		fact := reporting.Fact{Metric: metric, SourceType: source}
		var owner *ids.UUID
		err := row.Scan(&fact.Row.Key, &fact.SourceID, &fact.Row.Label, &owner, &fact.OwnerLabel, &fact.Row.OccurredAt, &fact.Row.Value, &fact.Provenance, &fact.GroupKey, &fact.GroupLabel)
		if owner != nil {
			fact.OwnerID = *owner
			fact.Row.OwnerId = ptrUUID(*owner)
		}
		fact.Row.SourceId = ptrUUID(fact.SourceID)
		fact.Row.SourceType = &fact.SourceType
		return fact, err
	})
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	if len(facts) > reportingFactLimit {
		return nil, crmcontracts.ReportingCoverage{}, fmt.Errorf("narrow the reporting population before evaluating: %w", apperrors.ErrInvalidArgument)
	}
	return reportingCohorts(frame, facts), crmcontracts.ReportingCoverage{Status: "ok", Withheld: withheld}, nil
}
