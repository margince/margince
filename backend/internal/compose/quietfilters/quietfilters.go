// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package quietfilters declares the three deal filters that answer "which deals
// have gone quiet" without a second record kind in the query grammar. They
// read the stored activity clocks of the deal and of its company, and the
// deal list's own meeting-ahead rule.
//
// Held by: TestQuietDealQuestionsAnswerThroughDerivedFilters
// (backend/internal/compose/integration)
package quietfilters

import (
	"maps"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// The three expressions render against a deal report aliased t.
var (
	// DaysSinceActivitySQL is whole days since the deal's last activity. A deal
	// with no activity yet has been quiet for ever. It matches every "at least
	// N days" question and no "fewer than N days" one.
	DaysSinceActivitySQL = daysSince("t.last_activity_at")
	// MeetingAheadSQL is the deal list's own answer, so a report and a screen
	// agree on which deals have a meeting still to come.
	MeetingAheadSQL = deals.MeetingAheadSQL("t")
	// CompanyDaysSinceActivitySQL is the same count for the deal's company. A
	// deal with no company reads NULL and matches no comparison.
	CompanyDaysSinceActivitySQL = "(SELECT " + daysSince("c.last_activity_at") + " FROM company c WHERE c.id = t.company_id)"
)

const companyID = "t.company_id"

func daysSince(column string) string {
	return "coalesce(floor(extract(epoch FROM (now() - " + column + ")) / 86400)::float8, 'Infinity'::float8)"
}

// Extend adds the three filters to a deal report's vocabulary. The maps are
// copied, because every request shares the prebuilt catalog.
//
// The company filter reads another record's attribute, so it carries the
// company's row scope through company_id and owes the company read grant. The
// meeting filter reads activities and owes that grant.
func Extend(filters, scopeVia, grants map[string]string) (outFilters, outScopeVia, outGrants map[string]string) {
	outFilters = with(filters, map[string]string{
		analyticsquery.FilterDaysSinceActivity:        DaysSinceActivitySQL,
		analyticsquery.FilterMeetingAhead:             MeetingAheadSQL,
		analyticsquery.FilterCompanyDaysSinceActivity: CompanyDaysSinceActivitySQL,
	})
	outScopeVia = with(scopeVia, map[string]string{analyticsquery.FilterCompanyDaysSinceActivity: companyID})
	outGrants = with(grants, map[string]string{
		analyticsquery.FilterMeetingAhead:             "activity",
		analyticsquery.FilterCompanyDaysSinceActivity: "company",
	})
	return outFilters, outScopeVia, outGrants
}

func with(base, add map[string]string) map[string]string {
	out := maps.Clone(base)
	if out == nil {
		out = make(map[string]string, len(add))
	}
	maps.Copy(out, add)
	return out
}
