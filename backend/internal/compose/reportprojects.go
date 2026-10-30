// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The project keys of the prebuilt catalog: what a delivery manager asks of
// the bodies of work in flight. Kept apart from reportspecs.go because they
// share a vocabulary of their own — the project row's columns, the money and
// the commitments its deals and tasks fold to — and that file sits near its
// length cap.

import (
	"fmt"
	"github.com/margince/margince/backend/internal/platform/database/storekit"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
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

	// A project's commitments are the open tasks filed under it. Overdue is
	// a due date already past; a task with no due date is open but never
	// overdue. Both read through the caller's activity content clause.
	openCommitmentsExpr = "(SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id" +
		" AND al.entity_type = 'project' AND al.project_id = t.id" +
		" WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND " + reportActivityScopeToken + ")"
)

// Project totals and coverage compose the deal module's valuation under the
// same field-mask and row-scope predicates as the project header.
var (
	overdueCommitmentsExpr = "(SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id" +
		" AND al.entity_type = 'project' AND al.project_id = t.id" +
		" WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND " + storekit.TaskDueSQL("a") + " < now() AND " +
		reportActivityScopeToken + ")"
	openDealValueBaseExpr = projectDealMoneySQL("open", false)
	wonDealValueBaseExpr  = projectDealMoneySQL("won", false)
	openDealUnpricedExpr  = projectDealMoneySQL("open", true)
	wonDealUnpricedExpr   = projectDealMoneySQL("won", true)
)

func projectDealMoneySQL(status string, unpriced bool) string {
	expression := deals.BaseValueSumSQL(reportAsOfToken, reportBaseCurrencyToken, "d", reportDealMaskToken)
	if unpriced {
		expression = deals.UnpricedCountSQL(reportAsOfToken, reportBaseCurrencyToken, "d", reportDealMaskToken)
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

// colProjectCustomer is the company a project report GROUPS by: its customer,
// the first company attached with that role.
//
// A project is worked by several companies, and a dimension has to be one value
// per row — you cannot group a project under three headings at once. The
// customer is the honest choice: it is what company_id has meant since the
// edge existed, and it is the company a reader means when they ask which
// account a delivery is for.
const colProjectCustomer = "(SELECT c.company_id FROM relationship c" +
	" WHERE c.kind = 'project_company' AND c.project_id = t.id" +
	" AND c.archived_at IS NULL AND c.role = 'customer'" +
	" ORDER BY c.created_at, c.id LIMIT 1)"

// colProjectAnyCompany is what a company FILTER matches: any live company on
// the project, so narrowing a report to a partner shows the deliveries that
// partner is genuinely on rather than only the ones they are the customer of.
const colProjectAnyCompany = "(SELECT c.company_id FROM relationship c" +
	" WHERE c.kind = 'project_company' AND c.project_id = t.id AND c.archived_at IS NULL" +
	" ORDER BY (c.role = 'customer') DESC, c.created_at, c.id LIMIT 1)"

// projectsByPhaseSpec counts the projects in each phase and folds the money
// their deals are worth. The project's company is row-scoped and masked on a
// normal read, so grouping by it carries the same obligation the deal reports'
// company columns do.
func projectsByPhaseSpec() reportSpec {
	return reportSpec{
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
			fieldCompanyID: colProjectAnyCompany,
			fieldOwnerID:   colOwnerID,
			fieldPhase:     colPhase,
		},
		referenceScopes: map[string]string{colProjectCustomer: tableCompany, colProjectAnyCompany: tableCompany},
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
	}
}

// projectCommitmentsSpec lists each live project with its open and overdue
// commitments, the most overdue first — the morning read of what was promised
// on which body of work.
func projectCommitmentsSpec() reportSpec {
	return reportSpec{
		entity:    datasource.EntityProject,
		table:     tableProject,
		baseWhere: whereArchivedNull,
		basePlain: "live (unarchived) projects, each with the open tasks filed under it (overdue: due date already past)",
		// Install-wide, as projectsByPhaseSpec. The default grouping names
		// owner_id beside the project's own id, which is a listing and not a
		// breakdown by owner, so it answers whole.
		population: measureEveryReadableRow,
		dimensions: projectRowDimensions(),
		rowKeys:    projectRowKeys,
		measures: map[string]string{
			measureOpenCommitments: openCommitmentsExpr,
			measureOverdue:         overdueCommitmentsExpr,
		},
		filters: map[string]string{
			fieldCompanyID: colProjectAnyCompany,
			fieldOwnerID:   colOwnerID,
			fieldPhase:     colPhase,
		},
		referenceScopes: map[string]string{colProjectCustomer: tableCompany, colProjectAnyCompany: tableCompany},
		// The commitment counts read TASKS, which take the activity grant.
		grants:    map[string]string{measureOpenCommitments: tableActivity, measureOverdue: tableActivity},
		defaultBy: []string{fieldProjectID, fieldName, fieldKey, fieldPhase, fieldOwnerID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnSum, Field: measureOverdue, As: measureOverdue},
			{Fn: aggFnSum, Field: measureOpenCommitments, As: measureOpenCommitments},
		},
		orderBy: "sum(" + overdueCommitmentsExpr + ") DESC, sum(" + openCommitmentsExpr + ") DESC",
		notes:   "rows are ordered most overdue first",
	}
}

// projectsGoneQuietSpec lists the projects in flight that nothing has been
// filed against for `days` days (default projects.DefaultProjectQuietDays). The
// project_gone_quiet signal is raised off the SAME predicate
// (projects.ProjectQuietSQL), so the report and the signal never disagree about
// which projects are quiet.
func projectsGoneQuietSpec() reportSpec {
	dimensions := projectRowDimensions()
	dimensions[fieldLastActivityAt] = colLastActivity
	dimensions[fieldQuietSince] = projects.ProjectQuietAnchorSQL("t")
	return reportSpec{
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
			fieldCompanyID: colProjectAnyCompany,
			fieldOwnerID:   colOwnerID,
			fieldPhase:     colPhase,
		},
		thresholds: map[string]reportThreshold{
			fieldDays: {
				clause:       func(pos int) string { return projects.ProjectQuietSQL("t", "now()", pos) },
				defaultValue: projects.DefaultProjectQuietDays,
			},
		},
		referenceScopes: map[string]string{colProjectCustomer: tableCompany, colProjectAnyCompany: tableCompany},
		defaultBy:       []string{fieldProjectID, fieldName, fieldKey, fieldPhase, fieldOwnerID, fieldLastActivityAt, fieldQuietSince},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: "projects"},
		},
		orderBy: "min(" + projects.ProjectQuietAnchorSQL("t") + ")",
		notes:   "`days` is a whole number of days of silence, default 30; quiet_since is when the silence began",
	}
}

func reportMoneyAggregateSQL(spec reportSpec, aggregate reportAggregate, expression string) string {
	rendered := fmt.Sprintf("%s(%s)", aggregate.Fn, expression)
	coverage, ok := spec.sumCoverage[aggregate.Field]
	if !ok {
		return rendered
	}
	if aggregate.Fn == aggFnSum {
		return deals.FinalizeBaseTotalSQL(rendered, "sum("+coverage+")")
	}
	// An average, a minimum or a maximum over a converted amount skips the
	// rows that could not be priced, and reads as the whole group. They have
	// no zero to fall back on, so an unpriced row leaves no answer.
	if foldsMoney[aggregate.Fn] {
		return deals.StrictBaseTotalSQL(rendered, "sum("+coverage+")")
	}
	return rendered
}

// foldsMoney are the aggregates that combine amounts into one, apart from the
// sum, which has its own answer for an empty group.
var foldsMoney = map[string]bool{aggFnAvg: true, aggFnMin: true, aggFnMax: true}
