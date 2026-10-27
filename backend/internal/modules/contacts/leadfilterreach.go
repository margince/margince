// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which lead list filters this caller would be refused, read off the lead
// list's own list resolution and narrowing (storekit.ProbeFilterReach).

import (
	"context"
	"errors"
	"strconv"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadProbeOperands are tried on each filter in turn: one of each kind a
// filter binds, then the codes a clause checks for itself.
var leadProbeOperands = []string{
	ids.Nil.String(), strconv.FormatBool(true), strconv.Itoa(0), string(crmcontracts.ListLeadsParamsCapturedByKindHuman),
	string(crmcontracts.ListLeadsParamsIndustryAgriculture), string(crmcontracts.ListLeadsParamsSizeBandN110),
	storekit.ProbeCountry,
}

// LeadListFilterReach probes every lead list filter for the caller on ctx, through the list's
// own resolution and narrowing. members is the lead list's resolver; nil while lists are off.
func LeadListFilterReach(ctx context.Context, members storekit.ListMemberFilterResolver) ([]storekit.FilterReach, error) {
	return storekit.ProbeFilterReach(ctx, leadListFilters, leadProbeOperands, func(ctx context.Context, in *ListLeadsInput) error {
		membership, err := storekit.ResolveListMembership(ctx, in.MemberOf, members, leadEntity)
		// The probe's list id names no list. Not found answers for that id and refuses no dial.
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		in.Membership = membership
		_, err = leadListWhere(ctx, *in, nil, nil, leadSLAPolicy{}, func(any) int { return 1 })
		return err
	})
}

// RefusedLeadFilters names the lead list filters the caller on ctx is refused,
// with list_id resolved as this handler set's lead list resolves it.
func (h Handlers) RefusedLeadFilters(ctx context.Context) ([]string, error) {
	return refusedLeadFilters(ctx, h.listMembers)
}

func refusedLeadFilters(ctx context.Context, members storekit.ListMemberFilterResolver) ([]string, error) {
	reach, err := LeadListFilterReach(ctx, members)
	if err != nil {
		return nil, err
	}
	return storekit.RefusedFilters(reach), nil
}
