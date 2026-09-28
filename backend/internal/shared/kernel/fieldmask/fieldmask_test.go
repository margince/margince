// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package fieldmask

import (
	"slices"
	"testing"
)

// A mask on either money figure withholds both, and the currency with them.
//
// The pair is the point: masking the amount while the ARR stands beside it
// discloses the size of the deal the mask was configured to hide, and the
// currency alone tells a reader the deal is priced. Configured either way
// round, the answer is the same three fields.
func TestMaskingEitherMoneyFigureWithholdsThePair(t *testing.T) {
	t.Parallel()

	want := []string{DealAmountMinor, DealExpectedArrMinor, DealCurrency}
	for _, configured := range []string{DealAmountMinor, DealExpectedArrMinor} {
		got := Withheld(Deal, []string{configured})
		for _, field := range want {
			if !slices.Contains(got, field) {
				t.Errorf("a mask on %q withholds %v, missing %q — the figure it was set to hide is "+
					"recoverable from the one left standing", configured, got, field)
			}
		}
	}
}

// A field this table never grouped withholds ITSELF.
//
// The table records what a field drags along with it, so an object that groups
// nothing has no entry and every object but the deal is in that position today.
// Read as an allowlist it would answer "nothing is withheld" for all of them,
// turning every such mask inert — the same defect one layer further in, and
// silent, because a mask that withholds nothing looks exactly like no mask.
func TestAnUngroupedFieldWithholdsItselfRatherThanNothing(t *testing.T) {
	t.Parallel()

	if got := Withheld("company", []string{"legal_name"}); !slices.Equal(got, []string{"legal_name"}) {
		t.Errorf("an ungrouped field withheld %v, want just itself — read as an allowlist this table "+
			"would make every mask outside it withhold nothing at all", got)
	}
	if !Covers("company", []string{"legal_name"}, "legal_name") {
		t.Error("Covers denied a field against its own mask, so a sort or filter over it would compile")
	}
}

// Maskable is what an administrator may CONFIGURE; Withheld is what goes null,
// and it is wider. Collapsing the two would either offer a mask on a field
// nothing withholds or refuse to configure one that is withheld today.
func TestWhatIsWithheldIsWiderThanWhatIsConfigurable(t *testing.T) {
	t.Parallel()

	maskable := Maskable(Deal)
	if slices.Contains(maskable, dealPartnerAttribution) {
		t.Errorf("%q is offered as configurable; it is withheld WITH the partner it describes and "+
			"has no mask of its own", dealPartnerAttribution)
	}
	if !Covers(Deal, []string{DealPartnerCompanyID}, dealPartnerAttribution) {
		t.Errorf("a withheld partner left %q standing — \"sourced\" beside a null partner discloses "+
			"that some partner sourced the deal", dealPartnerAttribution)
	}
	if len(maskable) == 0 {
		t.Fatal("the deal offers no maskable field at all — this test has stopped reading the table")
	}
}
