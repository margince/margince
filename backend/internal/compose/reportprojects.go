// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The project keys of the prebuilt catalog: what a delivery manager asks of
// the bodies of work in flight. They share a vocabulary of their own.

import (
	"cmp"
	"fmt"
	"maps"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/moneytotal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	// The project measures, as a caller names them.
	measureOpenDealValue    = "open_deal_value_minor"
	measureWonDealValue     = "won_deal_value_minor"
	measureOpenDealUnpriced = "open_deal_unpriced"
	measureWonDealUnpriced  = "won_deal_unpriced"
	measureOpenCommitments  = "open_commitments"
	measureOverdue          = "overdue_commitments"

	fieldPhase          = "phase"
	fieldName           = "name"
	fieldKey            = "key"
	fieldLastActivityAt = "last_activity_at"
	fieldQuietSince     = "quiet_since"
	fieldDays           = "days"

	colRowID        = "t.id"
	colName         = "t.name"
	colKey          = "t.key"
	colPhase        = "t.phase"
	colLastActivity = "t.last_activity_at"

	openCommitmentsExpr    = "commitments.open_tasks"
	overdueCommitmentsExpr = "commitments.overdue_tasks"
)

// Project totals and coverage compose the deal module's valuation under the
// same field-mask and row-scope predicates as the project header.
var (
	// A project's commitments are the open tasks filed under it. Overdue is
	// a due date already past; a task with no due date is open but never
	// overdue. One pass counts both, so the statement carries the caller's
	// activity content clause once.
	joinProjectCommitments = "LEFT JOIN LATERAL (SELECT count(*) AS open_tasks," +
		" count(*) FILTER (WHERE " + storekit.TaskDueBeforeSQL("a", "now()") + ") AS overdue_tasks" +
		" FROM activity a JOIN activity_link al ON al.activity_id = a.id" +
		" AND al.entity_type = 'project' AND al.project_id = t.id" +
		" WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND " + reportActivityScopeToken + ") commitments ON true"
	openDealValueBaseExpr = projectDealMoneySQL("open", false)
	wonDealValueBaseExpr  = projectDealMoneySQL("won", false)
	openDealUnpricedExpr  = projectDealMoneySQL("open", true)
	wonDealUnpricedExpr   = projectDealMoneySQL("won", true)
)

func projectDealMoneySQL(status string, unpriced bool) string {
	expression := "coalesce(" + deals.BaseValueSumAtSQL(reportAsOfToken, reportRatesAtToken, reportBaseCurrencyToken, "d", reportDealMaskToken) + ", 0)"
	if unpriced {
		expression = deals.UnpricedCountAtSQL(reportAsOfToken, reportRatesAtToken, reportBaseCurrencyToken, "d", reportDealMaskToken)
	}
	return "(SELECT " + expression +
		" FROM deal d WHERE d.project_id = t.id AND d.status = '" + status +
		"' AND d.archived_at IS NULL AND " + reportDealScopeToken + ")"
}

// projectRowDimensions is the vocabulary the two listing-shaped project keys
// share: grouping by the row's own id makes each row one project, with the
// columns a reader needs to act on it.
func projectRowDimensions() map[string]string {
	return map[string]string{
		fieldProjectID: colRowID,
		fieldName:      colName,
		fieldKey:       colKey,
		fieldPhase:     colPhase,
		fieldOwnerID:   colOwnerID,
		fieldCompanyID: colProjectCustomer,
	}
}

var projectRowKeys = []string{fieldProjectID, fieldName, fieldKey}

var projectRowMoments = []string{fieldLastActivityAt, fieldQuietSince}

// colProjectCustomer is the company a project report groups by: its first live
// customer, the company a project read names. A dimension is one value per row,
// and a project is worked by several companies.
const colProjectCustomer = "(SELECT c.company_id FROM relationship c" +
	" JOIN company customer ON customer.id = c.company_id AND customer.archived_at IS NULL" +
	" WHERE c.kind = '" + auth.ProjectCompanyKind + "' AND c.project_id = t.id" +
	" AND c.archived_at IS NULL AND c.role = '" + auth.ProjectCustomerRole + "'" +
	" ORDER BY c.created_at, c.id LIMIT 1)"

// withProjectCompany gives a project report its company vocabulary. Naming a
// project's company names an edge, which owes relationship.read. A filter
// matches any live edge to the bound company, whatever its role.
func withProjectCompany(spec reportSpec) reportSpec {
	spec.referenceScopes = map[string]string{colProjectCustomer: tableCompany}
	spec.filterScopes = map[string]string{fieldCompanyID: tableCompany}
	spec.filterPredicates = map[string]string{fieldCompanyID: "EXISTS (SELECT 1 FROM relationship c" +
		" JOIN company edge_company ON edge_company.id = c.company_id AND edge_company.archived_at IS NULL" +
		" WHERE c.kind = '" + auth.ProjectCompanyKind + "' AND c.project_id = t.id AND c.archived_at IS NULL AND c.company_id = %s)"}
	grants := map[string]string{fieldCompanyID: relationshipObject}
	maps.Copy(grants, spec.grants)
	spec.grants = grants
	return spec
}

// projectsByPhaseSpec counts the projects in each phase and folds the money
// their deals are worth.
func projectsByPhaseSpec() reportSpec {
	return withProjectCompany(reportSpec{
		entity:    datasource.EntityProject,
		table:     tableProject,
		baseWhere: whereArchivedNull,
		basePlain: "live (unarchived) projects, with each project's open and won deal value in the installation's base currency",
		// A delivery lead asks how many projects are in delivery across the
		// installation. Naming one owner goes through the owner gate
		// (reportownergate.go).
		population: measureEveryReadableRow,
		dimensions: map[string]string{
			fieldPhase:     colPhase,
			fieldCompanyID: colProjectCustomer,
			fieldOwnerID:   colOwnerID,
		},
		measures: map[string]string{
			measureOpenDealValue:    openDealValueBaseExpr,
			measureWonDealValue:     wonDealValueBaseExpr,
			measureOpenDealUnpriced: openDealUnpricedExpr,
			measureWonDealUnpriced:  wonDealUnpricedExpr,
		},
		sumCoverage: map[string]string{measureOpenDealValue: openDealUnpricedExpr, measureWonDealValue: wonDealUnpricedExpr},
		filters: map[string]string{
			fieldCompanyID: colProjectCustomer,
			fieldOwnerID:   colOwnerID,
			fieldPhase:     colPhase,
		},
		// The money measures fold DEALS, which the project grant says nothing
		// about: the deal grant is owed before either is served.
		grants:    map[string]string{measureOpenDealValue: tableDeal, measureWonDealValue: tableDeal, measureOpenDealUnpriced: tableDeal, measureWonDealUnpriced: tableDeal},
		defaultBy: []string{fieldPhase},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: "projects"},
			{Fn: aggFnSum, Field: measureOpenDealValue, As: measureOpenDealValue},
			{Fn: aggFnSum, Field: measureWonDealValue, As: measureWonDealValue},
			{Fn: aggFnSum, Field: measureOpenDealUnpriced, As: measureOpenDealUnpriced},
			{Fn: aggFnSum, Field: measureWonDealUnpriced, As: measureWonDealUnpriced},
		},
		notes: "open and won deal values use the same base-currency valuation; unpriced counts identify amounts omitted for missing prices",
	})
}

// projectCommitmentsSpec lists each live project with its open and overdue
// commitments, the most overdue first.
func projectCommitmentsSpec() reportSpec {
	return withProjectCompany(reportSpec{
		entity:    datasource.EntityProject,
		table:     tableProject,
		baseWhere: whereArchivedNull,
		basePlain: "live (unarchived) projects, each with the open tasks filed under it (overdue: due date already past)",
		joins:     []string{joinProjectCommitments},
		// Install-wide, as projectsByPhaseSpec: the default grouping is a
		// listing, not a breakdown by owner.
		population: measureEveryReadableRow,
		dimensions: projectRowDimensions(),
		rowKeys:    projectRowKeys,
		measures: map[string]string{
			measureOpenCommitments: openCommitmentsExpr,
			measureOverdue:         overdueCommitmentsExpr,
		},
		filters: map[string]string{
			fieldCompanyID: colProjectCustomer,
			fieldOwnerID:   colOwnerID,
			fieldPhase:     colPhase,
		},
		// The commitment counts read TASKS, which take the activity grant.
		grants:    map[string]string{measureOpenCommitments: tableActivity, measureOverdue: tableActivity},
		defaultBy: []string{fieldProjectID, fieldName, fieldKey, fieldPhase, fieldOwnerID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnSum, Field: measureOverdue, As: measureOverdue},
			{Fn: aggFnSum, Field: measureOpenCommitments, As: measureOpenCommitments},
		},
		orderBy: "sum(" + overdueCommitmentsExpr + ") DESC, sum(" + openCommitmentsExpr + ") DESC",
		notes:   "rows are ordered most overdue first",
	})
}

// projectsGoneQuietSpec lists the projects in flight that nothing has been
// filed against for `days` days. The project_gone_quiet signal is raised off
// the same predicate (projects.ProjectQuietSQL), so the two agree.
func projectsGoneQuietSpec() reportSpec {
	dimensions := projectRowDimensions()
	dimensions[fieldLastActivityAt] = colLastActivity
	dimensions[fieldQuietSince] = projects.ProjectQuietAnchorSQL("t")
	return withProjectCompany(reportSpec{
		entity:    datasource.EntityProject,
		table:     tableProject,
		baseWhere: whereArchivedNull + " AND " + projects.ProjectInFlightSQL("t"),
		basePlain: "live projects being pursued or delivered that nothing has been filed against for at least `days` days (a project with no activity at all is measured from its creation)",
		// Install-wide, as projectCommitmentsSpec.
		population: measureEveryReadableRow,
		dimensions: dimensions,
		rowKeys:    projectRowKeys,
		rowMoments: projectRowMoments,
		measures:   map[string]string{},
		filters: map[string]string{
			fieldCompanyID: colProjectCustomer,
			fieldOwnerID:   colOwnerID,
			fieldPhase:     colPhase,
		},
		thresholds: map[string]reportThreshold{
			fieldDays: {
				clause:       func(pos int) string { return projects.ProjectQuietSQL("t", "now()", pos) },
				defaultValue: projects.DefaultProjectQuietDays,
			},
		},
		defaultBy: []string{fieldProjectID, fieldName, fieldKey, fieldPhase, fieldOwnerID, fieldLastActivityAt, fieldQuietSince},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: "projects"},
		},
		orderBy: "min(" + projects.ProjectQuietAnchorSQL("t") + ")",
		notes:   "`days` is a whole number of days of silence, default 30; quiet_since is when the silence began",
	})
}

// reportMoneyAggregateSQL renders one aggregate over a measure. Over a
// converted amount it answers only what it can price.
//
// A group holding an unpriced amount has no answer, whatever the function, so a
// function added to the vocabulary later is held to it too. A sum over an empty
// group reads zero, and a partialSums spec sums what converts. Every other function folds the rows as each row reads, and
// a project with no deals reads zero because its measure does.
func reportMoneyAggregateSQL(spec reportSpec, aggregate reportAggregate, expression string) string {
	rendered := aggregateOver(aggregate.Fn, expression)
	coverage, ok := spec.sumCoverage[aggregate.Field]
	if !ok {
		return rendered
	}
	if spec.partialSums && aggregate.Fn == aggFnSum {
		return moneytotal.PartialSumSQL(rendered)
	}
	return moneytotal.CompleteAggregateSQL(rendered, "sum("+coverage+")", aggregate.Fn == aggFnSum)
}

// partialSumAliases names the sums of this plan that add only the deals that
// convert, which the row's coverage then qualifies.
func partialSumAliases(spec reportSpec, aggregates []reportAggregate) (names []string) {
	for _, agg := range aggregates {
		if _, covered := spec.sumCoverage[agg.Field]; covered && spec.partialSums && agg.Fn == aggFnSum {
			names = append(names, cmp.Or(agg.As, agg.Fn))
		}
	}
	return names
}

// aggregateOver is one function over an expression. A percentile is rendered by
// analyticsquery.PercentileExpr, which the typed query also renders it with.
func aggregateOver(fn, expression string) string {
	if fn == aggFnMedian || fn == aggFnP75 {
		return analyticsquery.PercentileExpr(expression, analyticsquery.AggFn(fn))
	}
	return fmt.Sprintf("%s(%s)", fn, expression)
}
