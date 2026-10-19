// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

// reportableActivityKinds are the only activity kinds a report counts. Mail
// and messages are most of the activity table and grow with every connected
// mailbox, so a count over them scans the largest data in the installation;
// counting them needs its own pre-aggregated design, not a dashboard query.
var reportableActivityKinds = []string{"meeting", "task"}

// activityKindBound is the predicate every activity population carries. The
// kinds are the literals above, never a caller's value.
var activityKindBound = colKind + " IN ('" + strings.Join(reportableActivityKinds, "', '") + "')"

var activityKindDomain = analyticsquery.ValueDomain{
	Values: reportableActivityKinds,
	Why:    "reports count meetings and tasks only; email, messages, calls and notes are not counted",
}

// populationWhere is the spec's base predicate plus the activity bound. Every
// door onto a spec reads its population here, so an activity report its author
// left unbounded is bounded anyway.
func (s reportSpec) populationWhere() string {
	if s.table != tableActivity {
		return s.baseWhere
	}
	if s.baseWhere == "" {
		return activityKindBound
	}
	return s.baseWhere + " AND " + activityKindBound
}

// filterDomain is the closed set a filter over expr may name on this spec.
// Keyed on the table as well as the column, because other tables have a kind
// column of their own.
func filterDomain(spec reportSpec, expr string) analyticsquery.ValueDomain {
	if spec.table == tableActivity && expr == colKind {
		return activityKindDomain
	}
	return analyticsquery.ValueDomain{}
}
