// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which deal list filters this caller would be refused, read off the deal
// list's own list narrowing and clauses (storekit.ProbeFilterReach).

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ISO reserves this code for no currency, so a probe that sends it names no market.
const probeCurrency = "XXX"

// probeOperands are tried on each filter in turn: one of each kind a filter
// binds, then the words a clause checks for itself.
var probeOperands = []string{
	ids.Nil.String(), strconv.FormatBool(true), strconv.Itoa(0), time.Time{}.Format(time.DateOnly),
	attributionSourced, probeCurrency, storekit.ProbeCountry,
}

// FilterReach is one deal list filter as the probe sent it, and whether the
// caller is refused it.
type FilterReach = storekit.FilterReach

// ListFilterReach probes every filter the deal list binds for the caller on
// ctx, through the list narrowing and the clause builder the list itself runs.
// The members resolver is the one the deal list holds; nil while lists are off.
//
// Only a role-wide refusal counts. A mask that withholds a reference on some
// rows, such as a private company, narrows those rows and refuses no filter.
func ListFilterReach(ctx context.Context, members storekit.ListMemberFilterResolver) ([]FilterReach, error) {
	return storekit.ProbeFilterReach(ctx, dealListFilters, probeOperands, func(ctx context.Context, in *ListDealsInput) error {
		err := narrowToList(ctx, in, members)
		// The probe's list id names no list. Not found answers for that id and refuses no dial.
		if errors.Is(err, apperrors.ErrNotFound) {
			err = nil
		}
		if err != nil {
			return err
		}
		_, err = appendDealFilters(ctx, nil, *in, func(any) int { return 1 })
		return err
	})
}

// RefusedListFilters names the deal list filters the caller on ctx is refused,
// with list_id resolved as this handler set's deal list resolves it.
func (h Handlers) RefusedListFilters(ctx context.Context) ([]string, error) {
	return refusedListFilters(ctx, h.listMembers)
}

func refusedListFilters(ctx context.Context, members storekit.ListMemberFilterResolver) ([]string, error) {
	reach, err := ListFilterReach(ctx, members)
	if err != nil {
		return nil, err
	}
	return storekit.RefusedFilters(reach), nil
}
