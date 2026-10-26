// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamReader reads deals and companies on team scope, under the given masks.
func teamReader(objects map[string]principal.ObjectGrant, masks ...principal.FieldMask) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeTeam,
			Objects:    objects,
			FieldMasks: masks,
		},
	})
}

var dealAndCompanyRead = map[string]principal.ObjectGrant{maskObject: {Read: true}, "company": {Read: true}}

func TestRefusedListFiltersNamesWhatTheListWouldRefuse(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want []string
	}{
		{"a reader of every row", systemFilterCtx(), []string{}},
		{"a role withholding nothing", teamReader(dealAndCompanyRead), []string{}},
		// The amount's mask withholds the currency beside it, so the currency dial goes too.
		{
			"a role masking the amount",
			teamReader(dealAndCompanyRead, principal.FieldMask{Object: maskObject, Field: "amount_minor", Condition: principal.MaskAlways}),
			[]string{filterValueMin, filterValueMax, filterCurrency},
		},
		{
			"a mask that lifts where the reader may write",
			teamReader(dealAndCompanyRead, principal.FieldMask{Object: maskObject, Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}),
			[]string{filterValueMin, filterValueMax, filterCurrency},
		},
		{
			"a role masking the partner",
			teamReader(dealAndCompanyRead, principal.FieldMask{Object: maskObject, Field: filterPartnerCompanyID, Condition: principal.MaskAlways}),
			[]string{filterPartnerCompanyID, filterPartnerSourced, filterPartnerAttribution},
		},
		{"a seat without company.read", teamReader(map[string]principal.ObjectGrant{maskObject: {Read: true}}), []string{filterCountry}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RefusedListFilters(tc.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("refused %v, want %v", got, tc.want)
			}
		})
	}
}

// The probe runs the list's clauses with nobody bound, which is a fault and
// never a refusal: reading it as one would offer or withhold dials at random.
func TestRefusedListFiltersReturnsAFaultThatIsNoRefusal(t *testing.T) {
	if _, err := RefusedListFilters(context.Background()); err == nil {
		t.Error("probing with no caller bound answered, want the missing actor as an error")
	}
}
