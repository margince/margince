// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The deal-figures pass: a card that names a deal states what the deal is
// worth, when it was meant to land and who answers for it.
//
// Most lanes read the deal themselves and arrive with the numbers already on
// the row. The overnight brief does not — it ranks deal ids and keeps its
// composite and factor vector behind its own endpoint — so its rows reached a
// rep naming a deal and saying nothing else about it: no amount, no close date,
// nothing to act on. That is the row a rep is meant to open their day on, and
// it was the least informative one on the page.
//
// Figures are not that ranking arithmetic. The amount and the close date are
// the deal's own columns, which every other lane already carries onto its rows.
//
// One read for every deal the page names, beside the label pass and for the
// same reason: a follow-up read per card is the N+1 this feed exists to avoid.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nameTheMoney puts a deal's figures on every lane item that names one and
// carries none.
//
// Items that already have facts are left alone: the lane that produced them
// read the deal under this same reader, and asking again could only produce a
// second answer to a settled question.
func (s *Service) nameTheMoney(ctx context.Context, day *crmcontracts.Attention) error {
	if s.dealFacts == nil {
		return nil
	}
	lanes := []*[]crmcontracts.AttentionItem{&day.ThisMorning}
	wanted := make([]ids.UUID, 0, len(day.ThisMorning))
	seen := map[ids.UUID]bool{}
	for _, lane := range lanes {
		for i := range *lane {
			id, ok := needsDealFigures((*lane)[i])
			if !ok || seen[id] {
				continue
			}
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	figures, err := s.dealFacts.Figures(ctx, wanted)
	if err != nil {
		return err
	}
	for _, lane := range lanes {
		for i := range *lane {
			id, ok := needsDealFigures((*lane)[i])
			if !ok {
				continue
			}
			found, ok := figures[id]
			if !ok {
				// The reader may not see this deal, or it is gone. The row
				// keeps its name and says no more, which is the shape an
				// unnamed subject already has.
				continue
			}
			applyDealFigures(&(*lane)[i], found)
		}
	}
	return nil
}

// needsDealFigures answers which deal a lane item names when it carries no
// figures of its own.
func needsDealFigures(item crmcontracts.AttentionItem) (ids.UUID, bool) {
	if item.Deal != nil || item.Subject == nil || item.Subject.Type != subjectDeal {
		return ids.UUID{}, false
	}
	return ids.UUID(item.Subject.Id), true
}

// applyDealFigures writes one answer onto the item.
//
// The close date lands on DueAt as well as on the deal facts, because that is
// where the projection reads a row's deadline from — a date set only on the
// facts would print on the card and order nothing.
func applyDealFigures(item *crmcontracts.AttentionItem, figures DealFigures) {
	facts := &crmcontracts.AttentionDealFacts{AmountMinor: figures.AmountMinor}
	if !figures.StageID.IsZero() {
		stage := openapi_types.UUID(figures.StageID)
		facts.StageId = &stage
	}
	if !figures.OwnerID.IsZero() {
		owner := openapi_types.UUID(figures.OwnerID)
		facts.OwnerId = &owner
	}
	if figures.Currency != "" {
		currency := figures.Currency
		facts.Currency = &currency
	}
	item.Deal = facts
	if figures.ExpectedCloseDate != nil && item.DueAt == nil {
		closes := *figures.ExpectedCloseDate
		item.DueAt = &closes
	}
}
