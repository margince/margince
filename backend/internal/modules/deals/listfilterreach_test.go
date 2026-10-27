// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
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
			[]string{filterCurrency, filterValueMax, filterValueMin},
		},
		{
			"a mask that lifts where the reader may write",
			teamReader(dealAndCompanyRead, principal.FieldMask{Object: maskObject, Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}),
			[]string{filterCurrency, filterValueMax, filterValueMin},
		},
		{
			"a role masking the partner",
			teamReader(dealAndCompanyRead, principal.FieldMask{Object: maskObject, Field: filterPartnerCompanyID, Condition: principal.MaskAlways}),
			[]string{filterPartnerAttribution, filterPartnerCompanyID, filterPartnerSourced},
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

// With nobody bound, the probe meets a fault and never a refusal. Read as a
// refusal, it would offer or withhold dials at random.
func TestRefusedListFiltersReturnsAFaultThatIsNoRefusal(t *testing.T) {
	if _, err := RefusedListFilters(context.Background()); err == nil {
		t.Error("probing with no caller bound answered, want the missing actor as an error")
	}
}

// narrowestReader reads deals alone, and every field a deal mask can name is
// withheld on every row.
func narrowestReader() context.Context {
	var masks []principal.FieldMask
	for _, field := range fieldmask.Maskable(maskObject) {
		masks = append(masks, principal.FieldMask{Object: maskObject, Field: field, Condition: principal.MaskAlways})
	}
	return teamReader(map[string]principal.ObjectGrant{maskObject: {Read: true}}, masks...)
}

// The probe walks the list's own filter set, so a filter added there is
// probed and its refusal lands here without a second list to update.
func TestTheReachProbeWalksEveryDealListFilter(t *testing.T) {
	reach, err := ListFilterReach(narrowestReader())
	if err != nil {
		t.Fatal(err)
	}
	var probed, refused []string
	for _, probe := range reach {
		probed = append(probed, probe.Parameter)
		if probe.Refused {
			refused = append(refused, probe.Parameter)
		}
		// An operand a reader of every row is refused would be a fault read as a refusal.
		var in ListDealsInput
		if err := dealListFilters.Apply(&in, map[string]string{probe.Parameter: probe.Operand}); err != nil {
			t.Fatalf("the probe's %s operand does not bind: %v", probe.Parameter, err)
		}
		if _, err := appendDealFilters(systemFilterCtx(), nil, in, func(any) int { return 1 }); err != nil {
			t.Errorf("the probe's %s operand faults for a reader of every row: %v", probe.Parameter, err)
		}
	}
	if !slices.Equal(probed, dealListFilters.Names()) {
		t.Errorf("probed %v, want every filter the list binds: %v", probed, dealListFilters.Names())
	}
	want := []string{
		filterCompanyID, filterCountry, filterCurrency, filterPartnerAttribution, filterPartnerCompanyID,
		filterPartnerSourced, filterProjectID, filterValueMax, filterValueMin,
	}
	if !slices.Equal(refused, want) {
		t.Errorf("the narrowest reader is refused %v, want %v", refused, want)
	}
}
