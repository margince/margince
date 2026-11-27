// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// dealTerms renders a deal list's own where terms on the counter it is handed.
type dealTerms = func(arg func(any) int) ([]string, error)

// dealListQuery is what both deal list entry points share: the sort refusal,
// the shared prelude and the deal's own filters. A page total renders those
// filters again. The valuation is read in the list's own transaction, and only
// when the request sorts or filters by Value or close day.
func (s *Store) dealListQuery(ctx context.Context, tx pgx.Tx, in ListDealsInput, active []fieldcatalog.Column) (*storekit.ListPrelude, []string, dealTerms, error) {
	if err := refuseMaskedSort(ctx, in.Sort); err != nil {
		return nil, nil, nil, err
	}
	vocab := dealListFields
	if sortsByValue(in.Sort) || sortsByCloseDay(in.Sort) || in.readsValuation() {
		valuation, err := s.valuationTx(ctx, tx)
		if err != nil {
			return nil, nil, nil, err
		}
		vocab = dealListVocabulary(valuation)
		in.valuation = valuation
	}
	pre, err := storekit.BuildListPrelude(ctx, "deal", "deals", vocab, active,
		in.Sort, in.Limit, in.Cursor, in.CustomFilters)
	if err != nil {
		return nil, nil, nil, err
	}
	if in.positions != nil {
		pre.CollectPositions(in.positions)
	}
	now := s.clock()
	terms := func(arg func(any) int) ([]string, error) { return s.dealFilterTerms(ctx, tx, in, now, arg) }
	own, err := terms(pre.Arg)
	if err != nil {
		return nil, nil, nil, err
	}
	return pre, append(pre.Where(), own...), terms, nil
}

// dealFilterTerms is the deal's own filters, the win chance and at-risk dials
// included, judged at now.
func (s *Store) dealFilterTerms(ctx context.Context, tx pgx.Tx, in ListDealsInput, now time.Time, arg func(any) int) ([]string, error) {
	if s.winChanceSQL != nil && (in.WinProbabilityMin != nil || in.WinProbabilityMax != nil) {
		chance, err := s.winChanceSQL(ctx, tx, now, dealRef(""), arg)
		if err != nil {
			return nil, err
		}
		in.winChance = chance
	}
	s.bindMembershipChance(ctx, tx, &in, now)
	where, err := appendDealFilters(ctx, nil, in, arg)
	if err != nil {
		return nil, err
	}
	return in.atRiskTerms(arg, where), nil
}
