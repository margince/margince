// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// partnerActiveWindow is how recent the last activity on a certified partner's
// company must be for the partner to count as active. The definition copy says
// ninety days.
const partnerActiveWindow = 90 * 24 * time.Hour

// The two groups partner_activity splits partners into.
const (
	partnerActiveGroup   = "activity:active"
	partnerInactiveGroup = "activity:inactive"
)

const (
	partnerCertStatus   = "cert_status"
	partnerLastActivity = "last_activity_at"
	companyDisplayName  = "display_name"
)

const partnerCompanyJoin = "LEFT JOIN company pc ON pc.id=t.partner_company_id"

// partnerNameVisible renders whether this reader may see the partner company
// under alias pc. A partner they cannot see still counts, under one shared
// group that names no one.
func partnerNameVisible(ctx context.Context, b *reportingBindings) (string, error) {
	if !auth.ReadGranted(ctx, string(recordTypeCompany)) {
		return "FALSE", nil
	}
	scope, err := auth.ScopeClauseFor(ctx, string(recordTypeCompany), "pc", b.arg)
	if err != nil {
		return "", err
	}
	name, _, err := auth.MaskExcludedClause(ctx, string(recordTypeCompany), companyDisplayName, "pc", b.arg)
	if err != nil {
		return "", err
	}
	return "(" + orUnnarrowed(scope) + " AND " + orUnnarrowed(name) + ")", nil
}

// partnerGroup splits a partner deal metric by partner and by whether the
// partner sourced or influenced the deal. An attribution never set counts as
// sourced, as the deal page says.
func partnerGroup(frame crmcontracts.ReportingContext) func(context.Context, *reportingBindings) (string, string, error) {
	return func(ctx context.Context, b *reportingBindings) (string, string, error) {
		visible, err := partnerNameVisible(ctx, b)
		if err != nil {
			return "", "", err
		}
		said := reporting.ExplanationCopy(frame)
		attribution := "COALESCE(t.partner_attribution,'sourced')"
		key := "'partner:'||CASE WHEN " + visible + " THEN t.partner_company_id::text ELSE 'hidden' END||':'||" + attribution
		name := "CASE WHEN " + visible + " THEN pc.display_name ELSE " + b.add(said.Say(reportingBreakdownLabels.PartnerHidden)) + " END"
		kind := "CASE " + attribution + " WHEN 'influenced' THEN " + b.add(said.Say(reportingBreakdownLabels.Influenced)) + " ELSE " + b.add(said.Say(reportingBreakdownLabels.Sourced)) + " END"
		return key, name + "||' · '||" + kind, nil
	}
}

func readReportingPartnerPipeline(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	query := metricFactQuery{from: partnerCompanyJoin, where: "t.archived_at IS NULL AND t.partner_company_id IS NOT NULL", owner: colOwnerID, at: "t.created_at", key: "t.id::text", stage: "''", stageLabel: "''", outcome: "t.status", provenance: "'current_state'", money: true, group: partnerGroup(frame)}
	return readDealMetricFacts(ctx, tx, frame, reportingPartnerPipeline, query)
}

func readReportingPartnerWon(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	query := closingMetricQuery(true)
	query.from += " " + partnerCompanyJoin
	query.where += " AND t.partner_company_id IS NOT NULL"
	query.group = partnerGroup(frame)
	return readDealMetricFacts(ctx, tx, frame, reportingPartnerWon, query)
}

// readReportingPartnerActivity reads every live partner as it stands now, one
// fact each, grouped active or inactive. Active is certified with an activity
// on its company within partnerActiveWindow. company.last_activity_at is kept
// by trigger; partner.last_contact_at has no writer, so it decides nothing.
func readReportingPartnerActivity(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, _ crmcontracts.ReportingFramework, _ metricEvaluator) ([]reporting.Fact, crmcontracts.ReportingCoverage, error) {
	if frame.PipelineId != nil {
		return nil, noPipelineGap(frame), nil
	}
	// A partner is its company, so without company read every row would name
	// one; the figure is withheld, as the partner list refuses it.
	if !auth.ReadGranted(ctx, string(recordTypeCompany)) {
		gap := reportingGap(reportingUnavailable, reporting.ExplanationCopy(frame).Say(reportingBreakdownLabels.CompanyAccess))
		gap.Withheld = true
		return nil, gap, nil
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "c.owner_id", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeCompany), "c", b.arg)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	companyFields, companyWithheld, err := reportingFieldScope(ctx, string(recordTypeCompany), "c", []string{companyDisplayName, fieldOwnerID, partnerLastActivity}, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	partnerFields, partnerWithheld, err := reportingFieldScope(ctx, objectPartner, "p", []string{partnerCertStatus}, &b)
	if err != nil {
		return nil, crmcontracts.ReportingCoverage{}, err
	}
	said := reporting.ExplanationCopy(frame)
	at := b.add(frame.EvaluatedAt)
	active := "(p.cert_status='certified' AND c.last_activity_at>=" + b.add(frame.EvaluatedAt.Add(-partnerActiveWindow)) + " AND c.last_activity_at<" + at + ")"
	query := `SELECT p.id::text,c.id,c.display_name,c.owner_id,COALESCE(u.display_name,''),NULL::timestamptz,1::float8,'current_state',
 CASE WHEN ` + active + ` THEN '` + partnerActiveGroup + `' ELSE '` + partnerInactiveGroup + `' END,
 CASE WHEN ` + active + ` THEN ` + b.add(said.Say(reportingBreakdownLabels.Active)) + ` ELSE ` + b.add(said.Say(reportingBreakdownLabels.Inactive)) + ` END
 FROM partner p JOIN company c ON c.id=p.company_id LEFT JOIN app_user u ON u.id=c.owner_id
 WHERE p.archived_at IS NULL AND c.archived_at IS NULL AND p.created_at<` + at + `
 AND ` + orUnnarrowed(population) + ` AND ` + orUnnarrowed(rowScope) + ` AND ` + companyFields + ` AND ` + partnerFields + `
 ORDER BY p.id LIMIT ` + b.add(reportingFactLimit+1)
	return readGroupedFacts(ctx, tx, frame, reportingPartnerActivity, string(recordTypeCompany), query, b, companyWithheld || partnerWithheld)
}
