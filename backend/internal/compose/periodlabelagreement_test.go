// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

// The report engine's filter check and a kept period's span read one label
// the same way. A label the check refuses names no dates. One it admits names
// dates under the calendar that spells it.
func TestTheFilterCheckAndAKeptPeriodAgreeOnEveryLabel(t *testing.T) {
	t.Parallel()
	labels := map[string][]string{
		fieldPeriodYear:    {"2026", "FY2025/26", "FY2099/00", "FY2025/99", "26", "FY2025/2026"},
		fieldPeriodQuarter: {"2026-Q1", "FY2025/26-Q4", "FY2025/99-Q1", "2026-Q0"},
		fieldPeriodMonth:   {"2026-03", "2026-13", "2026-3"},
	}
	for field, values := range labels {
		for _, value := range values {
			admitted := checkPeriodValue(field, value) == nil
			spans := false
			for _, start := range []int{1, 4} {
				_, _, err := analyticsquery.PeriodSpan(periodGrains[field], value, analyticsquery.CalendarBasis{Timezone: "UTC", FiscalStartMonth: start})
				spans = spans || err == nil
			}
			if admitted != spans {
				t.Errorf("%s %s: the filter check admits it %v, a kept period spans it %v", field, value, admitted, spans)
			}
		}
	}
}
