// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The activity report's own vocabulary. Spelled here rather than borrowed from
// an unrelated surface that happens to use the same words: renaming a report
// dimension must never rename a capture target type.
const (
	fieldKind      = "kind"
	fieldDirection = "direction"
	fieldProject   = "project"
	colKind        = "t.kind"
	colDirection   = "t.direction"

	// activityProjectIDExpr is the project an activity is filed under, read
	// off its activity_link row. It is a scalar because the schema admits at
	// most one project link per activity (uq_activity_link_project), which is
	// what lets ONE expression serve as the dimension, the filter and the
	// drill-through column: `expr = $n` is exactly "EXISTS an activity_link
	// naming that project", and NULL is exactly "filed under none".
	activityProjectIDExpr = "(SELECT al.project_id FROM activity_link al" +
		" WHERE al.activity_id = t.id AND al.entity_type = 'project')"
	// activityProjectLabelExpr is that project as a reader sees it — its key
	// when it has one, else its name — so a breakdown by project needs no
	// second lookup to be readable.
	activityProjectLabelExpr = "(SELECT coalesce(p.key, p.name) FROM activity_link al" +
		" JOIN project p ON p.id = al.project_id" +
		" WHERE al.activity_id = t.id AND al.entity_type = 'project')"
)

// projectFilterScope is the read gate every report filtering by project_id
// runs before the filter binds (reportthreshold.go requireFilterScopes): the
// project grant, then the live row probe.
var projectFilterScope = map[string]string{fieldProjectID: tableProject}

// The prebuilt report catalog: WHAT each report asks, as data.
//
// Split from report.go, which holds the machinery that RUNS a spec — the
// validation, the SQL assembly, the row mapping. The two change for unrelated
// reasons: a new report is a new entry here and nothing else, while a change to
// how a report is executed touches none of these entries. Keeping them in one
// file put the catalog over the 500-line cap the moment a report grew a
// dimension.

// prebuiltReports is the report catalog (data-model §13 shape): keys
// are never UUIDs, so saved-report ids cannot collide.
// fieldDaysInStage is the stage-age measure, named because it is spelled in
// the measure map and in both default aggregates — and a name written three
// times can come to be written three ways, which makes an aggregate reference
// a measure that does not exist.
const fieldDaysInStage = "days_in_stage"

var prebuiltReports = map[string]reportSpec{
	"open-deals-per-company": {
		entity:    datasource.EntityDeal,
		table:     tableDeal,
		baseWhere: whereOpenDeal,
		basePlain: "live (unarchived) open deals",
		// Currency is a dimension AND a filter because amount_minor is a
		// measure here: a caller summing money on this key has to be able to
		// split it by currency. It stays OUT of defaultBy, unlike the two
		// reports below — this key's default plan counts deals and sums
		// nothing, so a currency split would multiply its rows while reporting
		// no more than it does now.
		dimensions: map[string]string{
			fieldOrganizationID: colOrganizationID,
			fieldOwnerID:        colOwnerID,
			fieldCurrency:       colCurrency,
		},
		measures: map[string]string{fieldAmountMinor: colAmountMinor},
		filters: map[string]string{
			fieldOwnerID:    colOwnerID,
			fieldPipelineID: colPipelineID,
			fieldCurrency:   colCurrency,
			fieldProjectID:  colProjectID,
		},
		filterScopes: projectFilterScope,
		// The company a deal points at is row-scoped and masked on a normal
		// deal read, so grouping by it carries the same obligation the partner
		// dimension does.
		referenceScopes: map[string]string{colOrganizationID: tableOrganization},
		defaultBy:       []string{fieldOrganizationID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: "open_deals"},
		},
	},
	// How long deals have sat where they are.
	//
	// Measured from the last time a deal ENTERED its current stage, not from
	// when it was created: a deal that moved forward last week is not an old
	// deal, and reading creation age as stage age would flag every long sale
	// the moment it started.
	//
	// The measure is days, so a percentile over it answers "what does typical
	// look like here" — which is the whole reason median and p75 exist on this
	// engine. Below the sample floor those answer NULL, and a stage with three
	// deals in it says so rather than reporting one deal's age as the norm.
	"stage-age": {
		entity: datasource.EntityDeal,
		table:  tableDeal,
		joins: []string{
			joinStageForWinProbability,
			// The most recent entry into the CURRENT stage. A deal that moved
			// out and back counts from its return, because that is when the
			// clock a reader cares about started again.
			`LEFT JOIN LATERAL (
				SELECT max(h.changed_at) AS entered_at
				FROM deal_stage_history h
				WHERE h.deal_id = t.id AND h.to_stage_id = t.stage_id
			) entry ON true`,
		},
		baseWhere: whereArchivedNull + " AND t.status = 'open'",
		basePlain: "live (unarchived) open deals, aged from the last time each entered its current stage",
		dimensions: map[string]string{
			fieldStageID:    colStageID,
			fieldPipelineID: colPipelineID,
			fieldOwnerID:    colOwnerID,
		},
		measures: map[string]string{
			// Days, as a whole number. A deal with no stage history at all —
			// one whose stage was set at creation before any move — falls back
			// to its creation date rather than reporting NULL: the age is real
			// even where the history is silent about it.
			fieldDaysInStage: "EXTRACT(DAY FROM (now() - COALESCE(entry.entered_at, t.created_at)))",
		},
		filters: map[string]string{
			fieldPipelineID: colPipelineID,
			fieldOwnerID:    colOwnerID,
		},
		defaultBy: []string{fieldStageID},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			// Median and p75 together, because one without the other invites
			// the reading that the middle deal is the whole story. The gap
			// between them is what says whether a stage has a long tail.
			{Fn: aggFnMedian, Field: fieldDaysInStage, As: "median_days"},
			{Fn: aggFnP75, Field: fieldDaysInStage, As: "p75_days"},
		},
	},
	"deals-by-stage": {
		entity:    datasource.EntityDeal,
		table:     tableDeal,
		joins:     []string{joinStageForWinProbability},
		baseWhere: whereArchivedNull,
		basePlain: "live (unarchived) deals",
		dimensions: map[string]string{
			fieldStageID:        colStageID,
			fieldStatus:         colStatus,
			fieldPipelineID:     colPipelineID,
			fieldWinProbability: colWinProbability,
			fieldCurrency:       colCurrency,
			// Grouping BY the partner is what turns "is this deal
			// partner-sourced" into "what did this partner bring us" — the
			// question the partner program is run on, and the one this report
			// could not answer while partner_sourced was a filter alone.
			fieldPartnerOrgID: colPartnerOrgID,
		},
		measures: map[string]string{
			fieldAmountMinor:         colAmountMinor,
			fieldWeightedAmountMinor: weightedAmountMinorExpr,
		},
		// No stage_id filter: nothing serves it (the screen groups BY stage_id
		// instead), and a filter key this report has no caller for is public
		// agent surface (the run_report catalog, mcp-info.{json,md}) with no
		// concrete use behind it. The rest match the board's own filter dials:
		// partner_sourced and stalled are boolean-valued expressions, which
		// the engine's generic `expr = $n` rendering already handles with no
		// special-casing.
		filters: map[string]string{
			fieldPipelineID:     colPipelineID,
			fieldStatus:         colStatus,
			fieldOwnerID:        colOwnerID,
			fieldOrganizationID: colOrganizationID,
			fieldPartnerSourced: deals.PartnerSourcedSQL("t"),
			// Narrowing to ONE partner, beside the boolean that asks whether
			// there is one. The board's totals are read from this report with
			// the deals screen's own filter dials, so a dial the screen offers
			// and this report refuses answers 422 — and the board then falls
			// back to counting loaded cards, which looks like a working total.
			fieldPartnerOrgID: colPartnerOrgID,
			fieldStalled:      deals.StalledSQL("t"),
			fieldCurrency:     colCurrency,
			fieldProjectID:    colProjectID,
		},
		filterScopes: projectFilterScope,
		// partner_org_id points at an organization, which a normal deal read
		// masks per row when the caller cannot open it.
		referenceScopes: map[string]string{colPartnerOrgID: tableOrganization},
		defaultBy:       moneyDefaultBy(fieldStageID),
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			{Fn: aggFnSum, Field: fieldAmountMinor, As: "amount_minor_sum"},
		},
	},
	"activities-by-kind": {
		entity:       datasource.EntityActivity,
		table:        tableActivity,
		baseWhere:    whereArchivedNull,
		basePlain:    "live (unarchived) activities",
		activityWalk: true,
		dimensions: map[string]string{
			fieldKind:      colKind,
			fieldDirection: colDirection,
			// Grouping by the project an activity is filed under answers
			// which bodies of work consumed the meeting and call effort; an
			// unfiled activity lands in the NULL group, which the wire reads
			// as "no project".
			fieldProjectID: activityProjectIDExpr,
			fieldProject:   activityProjectLabelExpr,
		},
		measures: map[string]string{},
		filters: map[string]string{
			fieldKind:      colKind,
			fieldDirection: colDirection,
			fieldProjectID: activityProjectIDExpr,
		},
		filterScopes: projectFilterScope,
		// The project a filed activity names is row-scoped; grouping by it
		// carries the same obligation the deal reports' company columns do,
		// and naming it at all — id or label — takes the project grant.
		referenceScopes: map[string]string{activityProjectIDExpr: tableProject},
		grants:          map[string]string{fieldProjectID: tableProject, fieldProject: tableProject},
		notes: "project_id admits exactly the activities filed under that project (an activity_link row " +
			"naming it); an activity filed nowhere, or under another project, is excluded",
		defaultBy: []string{fieldKind},
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: "activities"},
		},
	},
	// win-loss (REPORT-KEY-8) is assembled by winLossSpec in reportperiod.go
	// rather than spelled inline: it carries the period-bucket vocabulary with
	// it, and that vocabulary belongs beside the buckets it names.
	"win-loss": winLossSpec(),
	// What is still in play, converted per deal into one currency
	// (reportspecs_pipeline.go). Beside deals-by-stage rather than replacing
	// it: that report is the board's own totals, where a won deal still
	// belongs to the stage it was won in.
	"pipeline-current": pipelineCurrentSpec(),
	// The project keys (reportprojects.go): what a delivery manager asks of
	// the bodies of work in flight.
	"projects-by-phase":   projectsByPhaseSpec(),
	"project-commitments": projectCommitmentsSpec(),
	"projects-gone-quiet": projectsGoneQuietSpec(),
	// The forecast (B-E09.10) is a parameterized report over this same
	// engine, not a separate subsystem. Weighted value follows
	// formulas-and-rules §6: round(amount_minor × stage.win_probability
	// / 100) PER DEAL (half away from zero), so the roll-up total equals
	// the sum of the per-deal weighted values exactly (AC-F1) — the same
	// expression the drill-through rows expose. Stakeholders never join
	// in: the grain is one row per deal, so a multi-stakeholder deal
	// counts once (AC-F2).
	"forecast": {
		entity:    datasource.EntityDeal,
		table:     tableDeal,
		joins:     []string{joinStageForWinProbability},
		baseWhere: whereOpenDeal,
		basePlain: "open, unarchived deals (win probability read live from the deal's current stage; a commit/best_case deal whose close date is past, missing, or provisional reports as 'slipped' instead, per formulas §11)",
		dimensions: map[string]string{
			fieldOwnerID:        colOwnerID,
			fieldStageID:        colStageID,
			fieldPipelineID:     colPipelineID,
			"forecast_category": forecastCategoryExpr,
			fieldCurrency:       colCurrency,
			fieldWinProbability: colWinProbability,
			// The same question deals-by-stage answers, asked of the pipeline
			// rather than of what already landed: what is a partner bringing
			// us THIS quarter. It was on one of the two deal reports and not
			// the other, so "revenue by partner" could be read backwards and
			// never forwards.
			fieldPartnerOrgID: colPartnerOrgID,
		},
		// Both denominations, and both earn their place.
		//
		// The NATIVE pair stays because this spec's default grouping INCLUDES
		// currency: a sum of amount_minor under one currency row is a
		// well-defined figure. pipeline-current drops the native pair precisely
		// because its default grouping does not, and offering it there would
		// let a caller sum minor units across currencies.
		//
		// The BASE pair is what lets the forecast be read as ONE answer instead
		// of one answer per currency. A screen drawing the five categories had
		// to band by currency — a slot carries a single figure, and adding
		// euros to dong is the unit-less total data-semantics §1 r4 forbids —
		// so a manager comparing commit against best case compared them inside
		// each currency and never across the business.
		//
		// Priced through the SAME expressions pipeline-current uses, so the two
		// reports cannot value one deal differently.
		measures: map[string]string{
			fieldAmountMinor:         colAmountMinor,
			fieldWeightedAmountMinor: weightedAmountMinorExpr,
			fieldAmountBaseMinor:     pipelineBaseValueExpr,
			fieldWeightedBaseMinor:   pipelineWeightedBaseExpr,
		},
		filters: map[string]string{
			fieldOwnerID:        colOwnerID,
			fieldStageID:        colStageID,
			fieldPipelineID:     colPipelineID,
			"forecast_category": forecastCategoryExpr,
			fieldCurrency:       colCurrency,
			fieldProjectID:      colProjectID,
			// Narrowing to ONE partner, the dial the deals screen now offers.
			// partner_sourced is deliberately absent here and present on
			// deals-by-stage: that report's filters mirror the board's dials,
			// and a forecast asks which partner rather than whether there is
			// one.
			fieldPartnerOrgID: colPartnerOrgID,
		},
		filterScopes: projectFilterScope,
		// partner_org_id points at an organization a normal deal read masks
		// per row when the caller cannot open it, so grouping by it must not
		// name one they could not have seen.
		referenceScopes: map[string]string{colPartnerOrgID: tableOrganization},
		defaultBy:       moneyDefaultBy("forecast_category"),
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			{Fn: aggFnSum, Field: fieldAmountMinor, As: "unweighted_minor"},
			{Fn: aggFnSum, Field: fieldWeightedAmountMinor, As: "weighted_minor"},
		},
	},
}
