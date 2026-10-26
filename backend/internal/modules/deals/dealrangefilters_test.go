// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// rangeClauses renders the commercial dials for the system principal, which
// reads every company, and answers the clauses with their bound operands.
func rangeClauses(ctx context.Context, t *testing.T, f RangeFilters) ([]string, []any, error) {
	t.Helper()
	var args []any
	f.valuation = dealValuation{base: "EUR", day: "2026-10-26", zone: "Europe/Berlin"}
	clauses, err := appendRangeFilters(ctx, nil, f, func(v any) int { args = append(args, v); return len(args) })
	return clauses, args, err
}

// A lower bound above its upper one selects nothing, and an empty page would
// read as "no deals". So the list refuses it and names the lower bound.
func TestAnInvertedRangeIsRefusedByName(t *testing.T) {
	from, to := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	low, high := int64(500), int64(100)
	for _, tc := range []struct {
		name  string
		in    RangeFilters
		field string
	}{
		{"close days", RangeFilters{CloseFrom: &from, CloseTo: &to}, filterCloseFrom},
		{"values", RangeFilters{ValueMin: &low, ValueMax: &high}, filterValueMin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := rangeClauses(systemFilterCtx(), t, tc.in)
			var refused *values.ParseError
			if !errors.As(err, &refused) || refused.Field != tc.field || refused.Code != "range_inverted" {
				t.Errorf("an inverted range → %v, want range_inverted on %s", err, tc.field)
			}
		})
	}
}

// One bound is a range open at the other end. The clause compares only the
// side that was given, and a nil bound never reaches SQL as a NULL operand.
func TestAnOpenEndedRangeBindsOnlyItsOwnBound(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	clauses, args, err := rangeClauses(systemFilterCtx(), t, RangeFilters{CloseFrom: &from})
	if err != nil {
		t.Fatal(err)
	}
	if len(clauses) != 1 || !strings.Contains(clauses[0], ">=") {
		t.Fatalf("close_from alone rendered %q, want one lower bound", clauses)
	}
	if args[len(args)-1] != "2026-10-01" {
		t.Errorf("the lower bound bound %v, want the calendar day 2026-10-01", args[len(args)-1])
	}
}

// The country belongs to the company. A seat that reads deals and not
// companies is refused, not told where a company it cannot open is.
func TestTheCountryFilterNeedsCompanyRead(t *testing.T) {
	dealsOnly := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects:  map[string]principal.ObjectGrant{maskObject: {Read: true}},
		},
	})
	country := "DE"
	if _, _, err := rangeClauses(dealsOnly, t, RangeFilters{Country: &country}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("country for a seat without company.read → %v, want permission denied", err)
	}
}

// The columns store codes upper-cased. The HTTP list and the agent's filter
// both reach the store, so a lower-case code matches on either path.
func TestACodeFilterMatchesWhateverCaseItIsGivenIn(t *testing.T) {
	currency, country := "usd", "de"
	_, args, err := rangeClauses(systemFilterCtx(), t, RangeFilters{Currency: &currency, Country: &country})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"USD", "DE"} {
		if !slices.Contains(args, any(want)) {
			t.Errorf("the dials bound %v, want the upper-cased code %s", args, want)
		}
	}
}

// A code of the wrong shape matches no deal, and the empty page would read as
// "none". The HTTP list and the agent's filter both reach the store, so both refuse it by name.
func TestAMalformedCodeFilterIsRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		field string
		in    RangeFilters
	}{
		{filterCurrency, RangeFilters{Currency: new("US")}},
		{filterCurrency, RangeFilters{Currency: new("EU1")}},
		{filterCountry, RangeFilters{Country: new("DEU")}},
		{filterCountry, RangeFilters{Country: new("d-")}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			_, _, err := rangeClauses(systemFilterCtx(), t, tc.in)
			var refused *values.ParseError
			if !errors.As(err, &refused) || refused.Field != tc.field || refused.Code != tc.field+"_malformed" {
				t.Errorf("a malformed %s → %v, want %s_malformed on it", tc.field, err, tc.field)
			}
		})
	}
}

// No deal is worth less than nothing. A negative bound is refused by name
// rather than answered with a page that reads as every deal or none.
func TestANegativeValueBoundIsRefusedByName(t *testing.T) {
	negative := int64(-1)
	for _, tc := range []struct {
		field string
		in    RangeFilters
	}{
		{filterValueMin, RangeFilters{ValueMin: &negative}},
		{filterValueMax, RangeFilters{ValueMax: &negative}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			_, _, err := rangeClauses(systemFilterCtx(), t, tc.in)
			var refused *values.ParseError
			if !errors.As(err, &refused) || refused.Field != tc.field || refused.Code != "out_of_range" {
				t.Errorf("a negative %s → %v, want out_of_range on it", tc.field, err)
			}
		})
	}
}
