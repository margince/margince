// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A deal's trail withholds the base amount and rate a close froze into its
// image wherever the money or the currency is masked. The two give both back.
func TestADealTrailWithholdsTheBaseValueUnderAMoneyMask(t *testing.T) {
	t.Parallel()

	for _, masked := range fieldmask.DealBaseValueInputs() {
		p := maskedPrincipal(principal.FieldMask{Object: "deal", Field: masked})
		withheld := auth.MaskedHistoryFields(p, "deal", false)
		for _, column := range []string{"amount_minor_base", "fx_rate_to_base"} {
			if !slices.Contains(withheld, column) {
				t.Errorf("a mask on deal.%s withholds %v from the trail, missing %s", masked, withheld, column)
			}
		}
	}
}

// Every derived column the catalog declares follows its inputs into the trail,
// so a column added there is withheld here without a second list.
func TestEveryDerivedColumnFollowsItsInputsIntoTheTrail(t *testing.T) {
	t.Parallel()

	for _, column := range fieldmask.DerivedFrom(fieldmask.Deal, fieldmask.Maskable(fieldmask.Deal)) {
		inputs, _ := fieldmask.DerivedInputs(fieldmask.Deal, column)
		for _, input := range inputs {
			p := maskedPrincipal(principal.FieldMask{Object: fieldmask.Deal, Field: input})
			if withheld := auth.MaskedHistoryFields(p, fieldmask.Deal, false); !slices.Contains(withheld, column) {
				t.Errorf("a mask on deal.%s leaves %s in the trail: %v", input, column, withheld)
			}
		}
	}
}

// Without a money mask the trail keeps the base value: the withholding is the
// mask's, not a blanket removal.
func TestAnUnmaskedTrailKeepsTheBaseValue(t *testing.T) {
	t.Parallel()

	for _, p := range []principal.Principal{
		maskedPrincipal(),
		maskedPrincipal(principal.FieldMask{Object: "deal", Field: fieldmask.DealCompanyID}),
	} {
		withheld := auth.MaskedHistoryFields(p, "deal", false)
		if slices.Contains(withheld, "amount_minor_base") || slices.Contains(withheld, "fx_rate_to_base") {
			t.Errorf("masks %v withhold the base value from the trail: %v", p.Permissions.FieldMasks, withheld)
		}
	}
}

func maskedPrincipal(masks ...principal.FieldMask) principal.Principal {
	return principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: masks,
		},
	}
}
