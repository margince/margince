// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The at-risk deals below the ranked rows. The lane reads two sweeps of fifty;
// the tail reads the same candidate set as one predicate, by expected close.

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RiskBacklog continues the at-risk deals past the lane's sweeps. OPTIONAL:
// unbound, scrolling ends at the ranked rows for the at-risk source.
type RiskBacklog interface {
	// RiskAfter reads up to limit at-risk deals after a position, by expected
	// close, earliest first and undated last, each with the position after
	// it. more says deals lie past the batch.
	RiskAfter(ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, after string, limit int) (
		deals []Positioned[RiskyDeal], more bool, err error)
	// RiskAmong answers which of these deals are at risk under owners.
	RiskAmong(ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, deals []ids.UUID) (map[ids.UUID]bool, error)
}

// WithRiskBacklog binds the at-risk continuation, an option for the reason
// WithWaiting is one.
func (s *Service) WithRiskBacklog(b RiskBacklog) *Service {
	s.backlog.risk = b
	return s
}

// moreRisk reads the next at-risk deals as of the walk's own instant. A deal's
// place in the tail is its expected close date. It is drawn as the lane draws
// it: labelled, priced and judged against the ranked day's material bar.
func (s *Service) moreRisk(ctx context.Context, page tailPage, walk tailWindow, after string, limit int) (tailBatch, error) {
	owners, err := s.recordOwners(ctx)
	if err != nil {
		return tailBatch{}, err
	}
	read, more, err := s.backlog.risk.RiskAfter(ctx, walk.AsOf, owners, after, limit)
	if err != nil {
		return tailBatch{}, err
	}
	items := make([]crmcontracts.AttentionItem, len(read))
	subjects := make([]*crmcontracts.AttentionSubject, len(read))
	for i, deal := range read {
		items[i] = riskItem(deal.Row)
		subjects[i] = items[i].Subject
	}
	if err := s.labelSubjects(ctx, subjects); err != nil {
		return tailBatch{}, err
	}
	money, err := s.priceTheDay(ctx, crmcontracts.Attention{AsOf: walk.AsOf, AtRisk: &items})
	if err != nil {
		return tailBatch{}, err
	}
	dealIDs := make([]ids.UUID, len(read))
	for i, deal := range read {
		dealIDs[i] = deal.Row.DealID
	}
	elsewhere, err := s.dealsElsewhere(ctx, walk.AsOf, dealIDs, page.dealsAbove)
	if err != nil {
		return tailBatch{}, err
	}
	// appendLane draws one row per deal, in order.
	rows := appendLane(nil, sourceAtRisk, &items, walk.AsOf, riskClassifier(materialBarOf(page.day, s.money), money))
	batch := tailBatch{more: more, rows: make([]tailRow, len(read))}
	for i, deal := range read {
		batch.rows[i] = tailRow{
			row: rows[i], at: deal.Row.ExpectedCloseDate, after: deal.After, elsewhere: elsewhere[deal.Row.DealID],
		}
	}
	return batch, nil
}

// dealsElsewhere are the deals the at-risk tail never serves as at risk: those
// a waiting card names anywhere under this page's narrowing (step B), and those
// a ranked row above reports (step E, dealsReported). The tail and the Done
// check both ask it.
func (s *Service) dealsElsewhere(
	ctx context.Context, asOf time.Time, deals []ids.UUID, reported map[ids.UUID]bool,
) (map[ids.UUID]bool, error) {
	elsewhere := maps.Clone(reported)
	if elsewhere == nil {
		elsewhere = map[ids.UUID]bool{}
	}
	if s.backlog.waiting == nil || len(deals) == 0 {
		return elsewhere, nil
	}
	waiting, err := s.backlog.waiting.DealsWaiting(ctx, asOf, s.waitingFor(ctx), deals)
	if err != nil {
		return nil, err
	}
	maps.Copy(elsewhere, waiting)
	return elsewhere, nil
}

// riskInTail is, for a Done check, the asked deals this scope's tail would
// serve: at risk under its owners, and in no other row (dealsElsewhere). A failed read
// answers none, so the Done is refused rather than granted blind.
func (s *Service) riskInTail(
	ctx context.Context, asOf time.Time, bounded map[crmcontracts.WorklistItemSource]bool, ranking []ranked,
) []ranked {
	asked := s.marked.asked[sourceAtRisk]
	if len(asked) == 0 || s.backlog.risk == nil || !bounded[sourceAtRisk] {
		return nil
	}
	owners, err := s.recordOwners(ctx)
	if err != nil {
		return nil
	}
	held, err := s.backlog.risk.RiskAmong(ctx, asOf, owners, asked)
	if err != nil {
		slog.Log(ctx, laneFailureLevel(ctx), "a Done check could not read the at-risk tail", "error", err)
		return nil
	}
	elsewhere, err := s.dealsElsewhere(ctx, asOf, slices.Collect(maps.Keys(held)), dealsReported(ranking, s.aboveTheTail(ranking)))
	if err != nil {
		slog.Log(ctx, laneFailureLevel(ctx), "a Done check could not read who is waiting", "error", err)
		return nil
	}
	var rows []ranked
	for deal := range held {
		if !elsewhere[deal] {
			rows = append(rows, ranked{item: crmcontracts.WorklistItem{Source: sourceAtRisk, Id: deal.String()}})
		}
	}
	return rows
}

// dealsReported are the deals a ranked row above the tail reports in place of
// an at-risk row. Step E folds a brief row onto it; step B prefers a waiting
// card. A task about a deal sits beside its at-risk row.
func dealsReported(ranking []ranked, above map[worklistsnap.Row]bool) map[ids.UUID]bool {
	reported := map[ids.UUID]bool{}
	for _, row := range ranking {
		source := row.item.Source
		if source != crmcontracts.WorklistItemSourceBriefItem && source != sourceWaiting {
			continue
		}
		if subject := row.item.Subject; subject != nil && subject.Type == subjectDeal && above[rowIdentity(row)] {
			reported[ids.UUID(subject.Id)] = true
		}
	}
	return reported
}
