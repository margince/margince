// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which deal list filters this caller would be refused, said before they ask.
// A dial the list then refuses shows a failure where a narrowing was promised.
// So the answer is read off the list's own clauses.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// refusableFilterProbes are the list filters a grant or a field mask can
// refuse, each with an operand the clause builder accepts on its shape.
func refusableFilterProbes() []struct {
	parameter string
	input     ListDealsInput
} {
	sourced, zero := true, int64(0)
	attribution, currency, country := "sourced", "EUR", "DE"
	return []struct {
		parameter string
		input     ListDealsInput
	}{
		{filterCompanyID, ListDealsInput{CompanyID: &ids.CompanyID{}}},
		{filterProjectID, ListDealsInput{ProjectID: &ids.ProjectID{}}},
		{filterPartnerCompanyID, ListDealsInput{PartnerCompanyID: &ids.CompanyID{}}},
		{filterPartnerSourced, ListDealsInput{PartnerSourced: &sourced}},
		{filterPartnerAttribution, ListDealsInput{PartnerAttribution: &attribution}},
		{filterValueMin, ListDealsInput{ValueMin: &zero}},
		{filterValueMax, ListDealsInput{ValueMax: &zero}},
		{filterCurrency, ListDealsInput{Currency: &currency}},
		{filterCountry, ListDealsInput{Country: &country}},
	}
}

// RefusedListFilters names the deal list filters the caller bound on ctx is
// refused, by running each through the clause builder the list itself runs.
//
// Only a role-wide refusal counts. A mask that withholds a reference on some
// rows, such as a private company, narrows those rows and refuses no filter.
func RefusedListFilters(ctx context.Context) ([]string, error) {
	refused := []string{}
	for _, probe := range refusableFilterProbes() {
		_, err := appendDealFilters(ctx, nil, probe.input, func(any) int { return 1 })
		switch {
		case err == nil:
		case refusedForReach(err):
			refused = append(refused, probe.parameter)
		default:
			return nil, err
		}
	}
	return refused, nil
}

// refusedForReach tells a refusal of the caller's grant or mask from a fault
// in the probe's own operand. Such a fault is a bug here, and is returned.
func refusedForReach(err error) bool {
	if masked, ok := errors.AsType[*values.ParseError](err); ok {
		return masked.Code == auth.CodeFieldMasked
	}
	return errors.Is(err, apperrors.ErrPermissionDenied)
}
