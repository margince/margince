// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// dealSeatMasking is a seat whose role withholds the given deal fields. Row
// scope team and not all, deliberately: a principal reading every row is
// withheld nothing, so a fixture on that scope would assert nothing.
func dealSeatMasking(fields ...string) principal.Principal {
	masks := make([]principal.FieldMask, 0, len(fields))
	for _, field := range fields {
		masks = append(masks, principal.FieldMask{
			Object: maskObject, Field: field, Condition: principal.MaskAlways,
		})
	}
	return principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeTeam,
			Objects:    map[string]principal.ObjectGrant{maskObject: {Read: true}},
			FieldMasks: masks,
		},
	}
}

// Every name a configured mask can produce is a name this module knows how to
// null. ApplyFieldMasks DROPS one it does not: the value goes out and nothing
// names it, which is the reading a client cannot tell from a field nobody
// filled in. The group's members are the half that goes missing — they are
// named under the deal without being configured on it.
func TestEveryNameAConfiguredDealMaskProducesIsWithheld(t *testing.T) {
	t.Parallel()
	for field := range dealMaskableFields {
		names := auth.MaskedFields(dealSeatMasking(field), maskObject, false)
		if len(names) == 0 {
			t.Fatalf("a mask on %s withholds nothing at all, so the check below reads an empty list", field)
		}
		for _, name := range names {
			if _, known := dealWithholds[name]; !known {
				t.Errorf("a mask on %s withholds %s and this module cannot null it: the field goes "+
					"out with its value and masked_fields never names it", field, name)
			}
		}
	}
}

// Each withhold nulls its OWN field and no other. What travels with a withheld
// field is the closure's answer, given once for every path that withholds — a
// func reaching further would null a field the report never names.
func TestAWithheldPartnerNullsItsOwnColumnAlone(t *testing.T) {
	t.Parallel()
	partner, attribution := openapi_types.UUID(ids.NewV7()), crmcontracts.DealPartnerAttribution("sourced")
	deal := crmcontracts.Deal{PartnerCompanyId: &partner, PartnerAttribution: &attribution}

	dealWithholds[filterPartnerCompanyID](&deal)

	if deal.PartnerCompanyId != nil {
		t.Errorf("partner_company_id = %v, want it withheld", deal.PartnerCompanyId)
	}
	if deal.PartnerAttribution == nil {
		t.Error("withholding the partner nulled the attribution too, and only what is NAMED may be nulled")
	}
}

// maskFilterOperands is one operand per filter these cases narrow by, so each
// narrows by something real rather than by a zero value.
//
// gatekit:fixture the value each filter narrows by — test input, not a cost.
var maskFilterOperands = map[string]string{
	filterCompanyID:          ids.NewV7().String(),
	filterProjectID:          ids.NewV7().String(),
	filterPartnerCompanyID:   ids.NewV7().String(),
	filterPartnerAttribution: "sourced",
	filterOwnerID:            ids.NewV7().String(),
}

// A filter is a reading of the column it narrows by: a caller who may not read
// which company a deal is on learns it from which rows come back. So the list
// refuses a filter naming a withheld field, in the words a sort over the same
// column is refused in — an empty page would be just as safe and would teach
// the caller the value.
//
// The subject is DERIVED: every filter this module offers over a field it can
// withhold. A filter added over a maskable column joins the census with no
// second edit.
func TestTheDealListRefusesAFilterOverAWithheldField(t *testing.T) {
	t.Parallel()
	over := withheldDealFilters()
	if len(over) == 0 {
		t.Fatal("no filter of this list names a field the deal can withhold, so the loop below asserts nothing")
	}
	for _, name := range over {
		err := filterDeals(t, dealSeatMasking(maskableDealFields()...), name)
		var refused *values.ParseError
		if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
			t.Errorf("narrowing by %s under a role that withholds it → %v, want the masked-field refusal", name, err)
		}
	}
}

// The other direction, twice over, or the refusal above could be a list that
// refuses every filter there is: the same filters for a seat whose role
// withholds nothing, and a filter no mask names for a seat carrying them all.
func TestTheDealListRefusesOnlyWhatTheRoleWithholds(t *testing.T) {
	t.Parallel()
	for _, name := range withheldDealFilters() {
		if err := filterDeals(t, dealSeatMasking(), name); err != nil {
			t.Errorf("narrowing by %s for a seat that reads it: %v", name, err)
		}
	}
	if err := filterDeals(t, dealSeatMasking(maskableDealFields()...), filterOwnerID); err != nil {
		t.Errorf("narrowing by owner_id, which no mask names, under every deal mask there is: %v", err)
	}
}

// maskableDealFields is what an administrator can configure a deal mask on.
func maskableDealFields() []string {
	return slices.Collect(maps.Keys(dealMaskableFields))
}

// withheldDealFilters is the census both directions walk: the filters this
// list offers over a field the deal can withhold.
func withheldDealFilters() []string {
	var over []string
	for _, name := range dealListFilters.Names() {
		if _, withholdable := dealWithholds[name]; withholdable {
			over = append(over, name)
		}
	}
	return over
}

// filterDeals narrows a deal list by one filter as this seat, and answers what
// the store made of it. It goes through the real filter bindings, so the
// operand is parsed the way a request's is.
func filterDeals(t *testing.T, p principal.Principal, name string) error {
	t.Helper()
	operand, known := maskFilterOperands[name]
	if !known {
		t.Fatalf("the %q filter has no operand here, so nothing this case asks narrows anything", name)
	}
	var in ListDealsInput
	if err := dealListFilters.Apply(&in, map[string]string{name: operand}); err != nil {
		t.Fatalf("applying %s=%s: %v", name, operand, err)
	}
	var args []any
	_, err := appendDealFilters(principal.WithActor(context.Background(), p), nil, in,
		func(v any) int { args = append(args, v); return len(args) })
	return err
}
