// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
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

// A filter is a reading of the column it narrows by: a caller who may not read
// which company a deal is on learns it from which rows come back. So the list
// refuses a filter narrowing by a withheld field, in the words a sort over the
// same column is refused in — an empty page would be just as safe and would
// teach the caller the value.
func TestTheDealListRefusesAFilterOverAWithheldField(t *testing.T) {
	t.Parallel()
	over := withheldDealFilters(t)
	if len(over) == 0 {
		t.Fatal("no filter of this list reads a field the deal can withhold, so the loop below asserts nothing")
	}
	for _, name := range over {
		_, err := filterDeals(t, dealSeatMasking(maskableDealFields()...), name)
		var refused *values.ParseError
		if !errors.As(err, &refused) || refused.Code != auth.CodeFieldMasked {
			t.Errorf("narrowing by %s under a role that withholds it → %v, want the masked-field refusal", name, err)
		}
	}
}

// The other direction, or the refusal above could be a list that refuses every
// filter there is: a filter reading nothing a mask reaches, under a seat
// carrying every deal mask. The census renders every filter for a seat that
// withholds nothing, so that direction is asserted where it is derived.
func TestTheDealListRefusesOnlyWhatTheRoleWithholds(t *testing.T) {
	t.Parallel()
	if _, err := filterDeals(t, dealSeatMasking(maskableDealFields()...), filterOwnerID); err != nil {
		t.Errorf("narrowing by owner_id, which no mask names, under every deal mask there is: %v", err)
	}
}

// The census reads SQL because a filter is free to be named for the question it
// asks rather than for the column that answers it. One such filter has to be in
// reach, or a census matching names against the withhold registry would sweep
// the same corpus and nothing here could tell the two apart.
func TestTheDealFilterCensusReachesAFilterNamedForItsQuestion(t *testing.T) {
	t.Parallel()
	for _, name := range withheldDealFilters(t) {
		if _, named := dealWithholds[name]; !named {
			return
		}
	}
	t.Fatal("every filter reading a withheld column is also named after one, so this census proves " +
		"nothing a name match would not, and the filter named for its question is the one that leaks")
}

// The census matches the names a mask spells against rendered SQL, which holds
// only while the wire and the column still coincide. A withheld field the
// deal's own select list does not carry is one withheldDealColumns can no
// longer find in a clause, and the corpus shrinks with nothing failing.
func TestEveryWithheldDealFieldIsTheColumnTheSQLNames(t *testing.T) {
	t.Parallel()
	selected := regexp.MustCompile(`[a-z_]+`).FindAllString(dealColumns, -1)
	for _, field := range slices.Sorted(maps.Keys(dealWithholds)) {
		if !slices.Contains(selected, field) {
			t.Errorf("a mask withholds %s and the deal selects no such column: the filter census reads "+
				"clauses for this name and would stop recognising the filters that narrow by it", field)
		}
	}
}

// maskableDealFields is what an administrator can configure a deal mask on.
func maskableDealFields() []string {
	return slices.Collect(maps.Keys(dealMaskableFields))
}

// withheldDealColumns matches any column a deal mask withholds. Whole words, so
// partner_company_id is not read as a mention of company_id.
var withheldDealColumns = regexp.MustCompile(
	`\b(` + strings.Join(slices.Sorted(maps.Keys(dealWithholds)), "|") + `)\b`)

// withheldDealFilters is the census both directions walk: the filters whose SQL
// reads a column the deal can withhold.
//
// The clause is the subject, never the filter's name. partner_sourced narrows
// partner_company_id under a name of its own, so a census matching names
// against the withhold registry walks past the one filter whose name hides what
// it reads and reports PASS over the smaller corpus.
func withheldDealFilters(t *testing.T) []string {
	t.Helper()
	var over []string
	for _, name := range dealListFilters.Names() {
		clauses, err := filterDeals(t, dealSeatMasking(), name)
		if err != nil {
			t.Fatalf("narrowing by %s for a seat that withholds nothing: %v", name, err)
		}
		if slices.ContainsFunc(clauses, withheldDealColumns.MatchString) {
			over = append(over, name)
		}
	}
	return over
}

// filterDeals narrows a deal list by one filter as this seat, and answers the
// clauses the store made of it. It goes through the real filter bindings, so
// the operand is parsed the way a request's is.
func filterDeals(t *testing.T, p principal.Principal, name string) ([]string, error) {
	t.Helper()
	operand, known := dealFilterOperands[name]
	if !known {
		t.Fatalf("the %q filter has no operand here, so nothing this case asks narrows anything", name)
	}
	// Archived rows stay in, so every clause returned is the filter's own.
	in := ListDealsInput{IncludeArchived: true}
	if err := dealListFilters.Apply(&in, map[string]string{name: operand}); err != nil {
		t.Fatalf("applying %s=%s: %v", name, operand, err)
	}
	var args []any
	return appendDealFilters(principal.WithActor(context.Background(), p), nil, in,
		func(v any) int { args = append(args, v); return len(args) })
}
