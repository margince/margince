// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which deal list filters this caller would be refused, said before they ask.
// A dial the list then refuses shows a failure where a narrowing was promised.
// So the answer is read off the list's own list narrowing and clauses.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ISO reserves these codes for no currency and for user assignment, so a
// probe that sends them names no market.
const (
	probeCurrency = "XXX"
	probeAlpha2   = "ZZ"
)

// probeOperands are tried on each filter in turn: one of each kind a filter
// binds, then the words a clause checks for itself.
var probeOperands = []string{
	ids.Nil.String(), strconv.FormatBool(true), strconv.Itoa(0), time.Time{}.Format(time.DateOnly),
	attributionSourced, probeCurrency, probeAlpha2,
}

// FilterReach is one deal list filter as the probe sent it, and whether the
// caller is refused it.
type FilterReach struct {
	Parameter string
	Operand   string
	Refused   bool
}

// ListFilterReach probes every filter the deal list binds for the caller on
// ctx, through the list narrowing and the clause builder the list itself runs.
// The members resolver is the one the deal list holds; nil while lists are off.
//
// Only a role-wide refusal counts. A mask that withholds a reference on some
// rows, such as a private company, narrows those rows and refuses no filter.
func ListFilterReach(ctx context.Context, members storekit.ListMemberFilterResolver) ([]FilterReach, error) {
	reach := make([]FilterReach, 0, len(dealListFilters))
	for _, parameter := range dealListFilters.Names() {
		probe, err := probeListFilter(ctx, parameter, members)
		if err != nil {
			return nil, err
		}
		reach = append(reach, probe)
	}
	return reach, nil
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
	refused := []string{}
	for _, probe := range reach {
		if probe.Refused {
			refused = append(refused, probe.Parameter)
		}
	}
	return refused, nil
}

// probeListFilter sends the filter the first operand its clause takes. An
// operand of the wrong shape faults, so the next one is tried.
func probeListFilter(ctx context.Context, parameter string, members storekit.ListMemberFilterResolver) (FilterReach, error) {
	fault := fmt.Errorf("deal list filter %s binds none of the probe operands", parameter)
	for _, operand := range probeOperands {
		var in ListDealsInput
		if dealListFilters.Apply(&in, map[string]string{parameter: operand}) != nil {
			continue
		}
		err := narrowToList(ctx, &in, members)
		// The probe's list id names no list. Not found answers for that id and refuses no dial.
		if errors.Is(err, apperrors.ErrNotFound) {
			err = nil
		}
		if err == nil {
			_, err = appendDealFilters(ctx, nil, in, func(any) int { return 1 })
		}
		if err == nil || refusedForReach(err) {
			return FilterReach{Parameter: parameter, Operand: operand, Refused: err != nil}, nil
		}
		fault = fmt.Errorf("probing deal list filter %s: %w", parameter, err)
	}
	return FilterReach{}, fault
}

// refusedForReach tells a refusal of the caller's grant or mask from a fault
// in the probe's own operand.
func refusedForReach(err error) bool {
	if masked, ok := errors.AsType[*values.ParseError](err); ok {
		return masked.Code == auth.CodeFieldMasked
	}
	return errors.Is(err, apperrors.ErrPermissionDenied)
}
