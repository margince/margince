// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

// The first recorded qualification keeps its owner even after reassignment.
// Missing historical ownership remains unknown rather than using today's owner.
func firstQualificationSQL(at string) string {
	return `JOIN LATERAL (SELECT a.id, COALESCE((a.after->>'promoted_at')::timestamptz,a.occurred_at) AS qualified_at,
 NULLIF(a.evidence->>'qualification_owner_id','')::uuid AS owner_id,
 COALESCE(a.evidence ? 'qualification_owner_id',false) AS ownership_recorded
 FROM audit_log a WHERE a.entity_type='lead' AND a.action='promote' AND a.entity_id=l.id
 AND a.after->>'status'='promoted' AND a.after->>'qualified_deal_id' IS NOT NULL
 AND a.before->>'status' IS DISTINCT FROM 'promoted' AND COALESCE((a.after->>'promoted_at')::timestamptz,a.occurred_at)<` + at + `
 ORDER BY COALESCE((a.after->>'promoted_at')::timestamptz,a.occurred_at),a.id LIMIT 1) qualified ON true`
}

func readReportingLeadsQualified(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	var b reportingBindings
	scope, withheld, err := leadMetricScope(ctx, tx, frame, reportingLeadsQualified, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	label, err := leadLabelSQL(ctx, &b, b.add(reporting.ExplanationCopy(frame).Say(reportingBreakdownLabels.Lead)))
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	query := `SELECT qualified.id::text,l.id,` + label + `,qualified.owner_id,COALESCE(u.display_name,''),qualified.qualified_at,
 1::float8,CASE WHEN qualified.ownership_recorded THEN 'first_qualification' ELSE 'qualification_owner_unknown' END,'',''
 FROM lead l ` + firstQualificationSQL(b.add(frame.EvaluatedAt)) + `
 LEFT JOIN app_user u ON u.id=qualified.owner_id
 WHERE qualified.qualified_at>=` + b.add(reportingEarliest(frame)) + ` AND ` + scope + `
 ORDER BY qualified.id LIMIT ` + b.add(reportingFactLimit+1)
	facts, coverage, err := readGroupedFacts(ctx, tx, frame, reportingLeadsQualified, string(recordTypeLead), query, b, withheld)
	if err != nil {
		return nil, coverage, err
	}
	if frame.Scope.Kind != ScopeKindWorkspace {
		kept := facts[:0]
		unknown := false
		for _, fact := range facts {
			if fact.Provenance == "qualification_owner_unknown" {
				unknown = true
				continue
			}
			kept = append(kept, fact)
		}
		facts = kept
		if unknown {
			coverage = reportingGap(reportingPartial, reporting.ExplanationCopy(frame).Say(reportingIncompleteWords[reportingLeadsQualified]))
			coverage.Withheld = withheld
		}
	}
	return facts, coverage, nil
}
