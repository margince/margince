// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every per-row money measure is DECLARED as one.
//
// The engine refuses summing a native amount across a grouping that does not
// split by currency, and it decides which measures those are from
// `spec.nativeMeasures`. A spec that offers `amount_minor` and forgets to name
// it there gets no refusal at all — the rule reads as held and covers nothing,
// which is the failure a declaration replacing a convention invites.
//
// So the convention is kept as the CHECK rather than as the rule: a measure
// named `*_minor` and not `*_base_minor` is money in the row's own currency, and
// must be declared. Both directions, because a declaration naming a measure the
// spec does not offer is a rule aimed at nothing.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// convertedDespiteTheName are the measures whose name says `_minor` and whose
// EXPRESSION already converts to the installation's base currency.
//
// The naming convention is the check, not the rule, and this is what that
// distinction is for: these two fold deals through OpenDealBaseValueSQL over
// the base-currency token, so a total spanning currencies is already
// well-defined and grouping by currency would split an answer that needs no
// splitting. Their default plan groups by phase alone, correctly.
//
// Worth knowing rather than only waiving: the frontend's own rule reads the
// same `_minor` suffix, so it believes these are native too.
var convertedDespiteTheName = gatekit.Waive(map[string]string{
	"projects-by-phase.open_deal_value_minor": "openDealValueBaseExpr converts through " +
		"deals.OpenDealBaseValueSQL over the base-currency token; the name predates the conversion",
	"projects-by-phase.won_deal_value_minor": "wonDealValueBaseExpr, the same fold on the won side",
})

func TestEveryPerRowMoneyMeasureIsDeclaredNative(t *testing.T) {
	t.Parallel()
	defer convertedDespiteTheName.AssertAllMatched(t)
	if len(prebuiltReports) == 0 {
		t.Fatal("no prebuilt reports, so this gate compared nothing")
	}
	checked := 0
	for name, spec := range prebuiltReports {
		for measure := range spec.measures {
			if !looksLikeNativeMoney(measure) {
				continue
			}
			if convertedDespiteTheName.Waived(t, name+"."+measure) {
				continue
			}
			checked++
			if !spec.nativeMeasures[measure] {
				t.Errorf("report %q offers %q, which is money in each row's own currency, "+
					"and does not declare it in nativeMeasures — so summing it across a "+
					"grouping with no currency split adds one currency to another and "+
					"answers a plain integer with no unit",
					name, measure)
			}
		}
		for declared := range spec.nativeMeasures {
			if _, offered := spec.measures[declared]; !offered {
				t.Errorf("report %q declares %q native and offers no such measure — "+
					"the rule is aimed at a field no caller can name", name, declared)
			}
		}
	}
	if checked == 0 {
		t.Error("no per-row money measure found in any report, so this gate is reading " +
			"for a naming shape the catalog no longer uses and would pass over anything")
	}
}

// looksLikeNativeMoney reads the catalog's own naming: `_minor` is a minor-unit
// amount, and `_base_minor` is one already converted to the installation's
// currency. The frontend relies on the same distinction.
func looksLikeNativeMoney(measure string) bool {
	return strings.HasSuffix(measure, "_minor") && !strings.HasSuffix(measure, "_base_minor")
}
