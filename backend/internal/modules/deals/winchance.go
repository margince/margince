// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The one win chance of an open deal, read from the function every screen,
// report and tool shares (forecasting.ResolveWinChance). The deals module may
// not import forecasting, so the composition injects the two reads it needs.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/winprobability"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// WinChanceReader answers the win chance of each given open deal.
type WinChanceReader func(ctx context.Context, tx pgx.Tx, at time.Time, dealIDs []ids.UUID) (map[ids.UUID]winprobability.Chance, error)

// WinChanceSQL renders the win chance of every deal as a SQL expression over a
// deal alias, binding its operands through arg. A filter on win probability
// reads it, so the filter and the shown number agree.
type WinChanceSQL func(ctx context.Context, tx pgx.Tx, at time.Time, deal string, arg func(any) int) (string, error)

// stampWinChances replaces each open deal's win probability with its one win
// chance and attaches how it was made. A store with no reader keeps the stored
// probability, which is what module tests read.
func (s *Store) stampWinChances(ctx context.Context, tx pgx.Tx, page []crmcontracts.Deal) error {
	if s.winChances == nil {
		return nil
	}
	var open []ids.UUID
	for _, d := range page {
		if d.Status == crmcontracts.DealStatusOpen {
			open = append(open, ids.UUID(d.Id))
		}
	}
	if len(open) == 0 {
		return nil
	}
	chances, err := s.winChances(ctx, tx, s.clock(), open)
	if err != nil {
		return fmt.Errorf("deals: reading the win chance: %w", err)
	}
	for i := range page {
		chance, ok := chances[ids.UUID(page[i].Id)]
		if !ok {
			continue
		}
		percent := chance.Percent
		page[i].WinProbability = &percent
		page[i].WinChance = wireWinChance(chance)
	}
	return nil
}

// getDealWithChance is a deal read that also carries the one win chance.
func (s *Store) getDealWithChance(ctx context.Context, tx pgx.Tx, id ids.DealID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Deal, error) {
	deal, err := getDealInTx(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	return s.stampOne(ctx, tx, deal)
}

// readDealStamped is readDealForCaller for a deal about to leave the store in a
// write reply. It carries the one win chance, as a read does.
func (s *Store) readDealStamped(ctx context.Context, tx pgx.Tx, id ids.DealID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Deal, error) {
	deal, err := readDealForCaller(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	return s.stampOne(ctx, tx, deal)
}

// finishDealStamped is finishDealForCaller with the one win chance.
func (s *Store) finishDealStamped(ctx context.Context, tx pgx.Tx, d crmcontracts.Deal) (crmcontracts.Deal, error) {
	deal, err := finishDealForCaller(ctx, tx, d)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	return s.stampOne(ctx, tx, deal)
}

func (s *Store) stampOne(ctx context.Context, tx pgx.Tx, d crmcontracts.Deal) (crmcontracts.Deal, error) {
	one := []crmcontracts.Deal{d}
	if err := s.stampWinChances(ctx, tx, one); err != nil {
		return crmcontracts.Deal{}, err
	}
	return one[0], nil
}

// bindMembershipChance makes a live list narrowing the list read applies filter
// on the win chance, as the read's own win probability filters do.
func (s *Store) bindMembershipChance(ctx context.Context, tx pgx.Tx, in *ListDealsInput, now time.Time) {
	inner := in.Membership
	if inner == nil || s.winChanceSQL == nil {
		return
	}
	in.Membership = func(idColumn string, arg func(any) int) (string, error) {
		clause, err := inner(idColumn, arg)
		if err != nil {
			return "", err
		}
		return winprobability.Rebind(clause, "t", func() (string, error) {
			return s.winChanceSQL(ctx, tx, now, "t", arg)
		})
	}
}

// wireWinChance carries the chance across to the contract, field for field.
func wireWinChance(c winprobability.Chance) *crmcontracts.DealWinChance {
	out := crmcontracts.DealWinChance{
		Typed: c.Typed, InUse: c.InUse, EvidenceN: c.EvidenceN,
		SelfCorrecting: c.SelfCorrecting, Explanation: c.Line(),
	}
	if c.EvidenceN != nil {
		share := c.WonShare
		out.WonShare = &share
	}
	if c.Effect != nil {
		signal := crmcontracts.DealWinChanceEffectSignal(c.EffectSignal)
		learned, sample := c.EffectLearned, c.EffectSample
		out.EffectSignal, out.EffectDays, out.EffectLearned, out.EffectSample = &signal, c.EffectDays, &learned, &sample
	}
	return &out
}

// restateFigureChances gives each open deal's figures its one win chance, the
// number the deal shows, in place of the stored probability.
func (s *Store) restateFigureChances(ctx context.Context, tx pgx.Tx, figures map[ids.UUID]DealFigures) error {
	if s.winChances == nil {
		return nil
	}
	var open []ids.UUID
	for id, f := range figures {
		if !f.Closed {
			open = append(open, id)
		}
	}
	if len(open) == 0 {
		return nil
	}
	chances, err := s.winChances(ctx, tx, s.clock(), open)
	if err != nil {
		return fmt.Errorf("deals: reading the win chance behind the figures: %w", err)
	}
	for id, chance := range chances {
		if f, ok := figures[id]; ok {
			percent := chance.Percent
			f.WinProbability = &percent
			figures[id] = f
		}
	}
	return nil
}
