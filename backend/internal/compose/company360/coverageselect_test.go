// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheCommitteeIsTheNamedDealsOrElseTheNewest(t *testing.T) {
	newest, older := ids.NewV7(), ids.NewV7()
	open := []crmcontracts.CompanyCoverageDeal{
		{DealId: openapi_types.UUID(newest), Name: "Fleet 2027"},
		{DealId: openapi_types.UUID(older), Name: "Retrofit 2026"},
	}
	named := ids.From[ids.DealKind](older)
	unknown := ids.From[ids.DealKind](ids.NewV7())
	for _, tc := range []struct {
		name  string
		deals []crmcontracts.CompanyCoverageDeal
		named *ids.DealID
		want  int
		err   error
	}{
		{"unnamed reads the newest", open, nil, 0, nil},
		{"a named deal is read", open, &named, 1, nil},
		{"no open deal holds no committee", nil, nil, -1, nil},
		{"a deal outside the open ones is not found", open, &unknown, -1, apperrors.ErrNotFound},
		{"a name on an account with none is not found", nil, &named, -1, apperrors.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectedDeal(tc.deals, tc.named)
			if got != tc.want || !errors.Is(err, tc.err) || (tc.err == nil && err != nil) {
				t.Errorf("selectedDeal = %d, %v; want %d, %v", got, err, tc.want, tc.err)
			}
		})
	}
}
