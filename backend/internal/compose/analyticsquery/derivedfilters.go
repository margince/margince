// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"fmt"
	"strings"
)

// The deal filters that answer a question about another record kind. Their SQL
// is declared in the quietfilters package. The names and what each means live
// here so the vocabulary document a model reads can say it.
const (
	FilterDaysSinceActivity        = "days_since_activity"
	FilterMeetingAhead             = "meeting_ahead"
	FilterCompanyDaysSinceActivity = "company_days_since_activity"
)

// derivedFilterMeaning is one sentence per derived filter, in the order the
// vocabulary lists them.
var derivedFilterMeaning = []struct{ name, meaning string }{
	{FilterDaysSinceActivity, "whole days since the deal's last activity; a deal with no activity counts as longer than any number"},
	{FilterMeetingAhead, "true when a meeting is still to come on the deal"},
	{FilterCompanyDaysSinceActivity, "whole days since the deal's company's last activity; a company with none counts as longer than any number; a deal with no company matches nothing"},
}

// describeDerivedFilters names the derived filters the caller's seat holds.
func describeDerivedFilters(out *strings.Builder, entity Entity) {
	header := false
	for _, derived := range derivedFilterMeaning {
		if _, held := entity.Fields[derived.name]; !held {
			continue
		}
		if !header {
			out.WriteString("  filters about related records:\n")
			header = true
		}
		fmt.Fprintf(out, "    %s: %s\n", derived.name, derived.meaning)
	}
}
