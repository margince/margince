// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const focusLimit = 6

// Focus selects from the assembled day before a queue page can hide a candidate.
// It is a projection of the same ranking, not a second scoring system.
func focusOf(rows, considered []ranked, asOf time.Time, reader ids.UUID) crmcontracts.WorklistFocus {
	ordered := slices.Clone(rows)
	for i := range ordered {
		if ordered[i].item.Source == sourceDecay && ordered[i].pinned {
			ordered[i].item.Level = semanticLevelOf(ordered[i])
			ordered[i].pinned = false
		}
	}
	ordered = markCrowding(ordered)
	sortByRank(ordered)
	eligible := make([]ranked, 0, len(ordered))
	for _, row := range ordered {
		if focusEligible(row, asOf) {
			eligible = append(eligible, row)
		}
	}
	total := len(eligible)
	if len(eligible) > focusLimit {
		eligible = eligible[:focusLimit]
	}
	// Batch membership is not evidence that each member is urgent. Discount
	// only obligations individually named by a focus card.
	visible := make(map[RowRef]bool, len(eligible))
	for _, row := range eligible {
		visible[refOf(row)] = true
	}
	remaining := 0
	for _, row := range considered {
		if urgentWork(row) && !visible[refOf(row)] {
			remaining++
		}
	}
	return crmcontracts.WorklistFocus{
		Items:           renderInOrder(stampAsOf(eligible, asOf), reader),
		Total:           total,
		UrgentRemaining: remaining,
	}
}

func focusEligible(row ranked, asOf time.Time) bool {
	// An agreed future date is not a request to act today. Legal preparation and
	// deal close windows already have their own semantic classifier horizons.
	switch row.item.Source {
	case sourceDecay:
		return hasReason(row.item, "pinned")
	case "notice":
		return urgentWork(row)
	case "task":
		if row.reminded {
			return true
		}
		if !row.deadlineAt.IsZero() && row.deadlineAt.After(asOf) {
			return row.item.DueGroup != nil && *row.item.DueGroup == "today"
		}
	case sourceMeeting:
		return row.item.Kind != nil && *row.item.Kind == meetingKindUnprepared &&
			!row.deadlineAt.IsZero() && row.deadlineAt.Sub(asOf) <= 24*time.Hour
	}
	return semanticLevelOf(row) < levelRoutine
}

// nameWorklistRows names the queue, Focus and the optional list as one batch,
// so each naming read runs once per page rather than once per list. A failed
// advice read keeps the rows and is named on the page.
func (s *Service) nameWorklistRows(
	ctx context.Context, page *crmcontracts.Worklist, findings map[ids.UUID]string, day crmcontracts.Attention,
) error {
	lists := []*[]crmcontracts.WorklistItem{&page.Queue}
	if page.Focus != nil {
		lists = append(lists, &page.Focus.Items)
	}
	if page.OptionalRelationships != nil {
		lists = append(lists, page.OptionalRelationships)
	}
	var rows []crmcontracts.WorklistItem
	for _, list := range lists {
		rows = append(rows, *list...)
	}
	advised, adviceErr := s.readingDealMoves(ctx, rows, day)
	if adviceErr != nil {
		page.SourcesUnavailable = append(page.SourcesUnavailable, s.stampSources([]crmcontracts.WorklistSourceUnavailable{*adviceErr})...)
	}
	if err := advised.nameRows(ctx, rows, findings); err != nil {
		return err
	}
	for _, list := range lists {
		rows = rows[copy(*list, rows):]
	}
	return nil
}

// Both projections expose the same actions, standing, and ownership labels.
func (s *Service) nameRows(ctx context.Context, rows []crmcontracts.WorklistItem, findings map[ids.UUID]string) error {
	if err := s.nameTheStep(ctx, rows); err != nil {
		return err
	}
	if err := s.nameTheStanding(ctx, rows, findings); err != nil {
		return err
	}
	if err := s.nameTheContacts(ctx, rows); err != nil {
		return err
	}
	if err := s.nameTheCompanies(ctx, rows); err != nil {
		return err
	}
	if err := s.nameTheEmployers(ctx, rows); err != nil {
		return err
	}
	return s.nameTheOwners(ctx, rows)
}
