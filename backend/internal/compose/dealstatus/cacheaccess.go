// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// cardAccess records the input context, independently of citations a model selects.
type cardAccess struct {
	Activities []ids.UUID `json:"activities"`
	Seats      []ids.UUID `json:"seats"`
	Contacts   []ids.UUID `json:"contacts"`
	Room       bool       `json:"room"`
	Masks      []string   `json:"masks,omitempty"`
}

func accessForCard(f facts, in StatusInput, move crmcontracts.DealStatusCardMove) *cardAccess {
	out := &cardAccess{Room: in.Room != nil}
	if f.deal.MaskedFields != nil {
		out.Masks = slices.Clone(*f.deal.MaskedFields)
	}
	for _, row := range factActivities(f) {
		if !withheld(row) {
			out.Activities = append(out.Activities, ids.UUID(row.Id))
		}
	}
	for _, task := range f.openTasks {
		out.Activities = append(out.Activities, task.ID)
	}
	if id, ok := NamedActivity(move); ok {
		out.Activities = append(out.Activities, id)
	}
	for _, evidence := range move.Evidence {
		if evidence.ActivityId != nil {
			out.Activities = append(out.Activities, ids.UUID(*evidence.ActivityId))
		}
	}
	for _, seat := range f.seats {
		out.Seats = append(out.Seats, seat.ContactID)
		if seat.Name != "" {
			out.Contacts = append(out.Contacts, seat.ContactID)
		}
	}
	return out
}

type assessmentSeatAccess struct {
	visible map[ids.UUID]bool
	named   map[ids.UUID]bool
}

func (s *Service) readableAssessmentSeats(ctx context.Context, tx pgx.Tx, cards map[ids.UUID]CachedCard) (map[ids.UUID]assessmentSeatAccess, error) {
	out := map[ids.UUID]assessmentSeatAccess{}
	var wanted []ids.UUID
	for id, card := range cards {
		if card.access != nil && len(card.access.Seats) > 0 {
			wanted = append(wanted, id)
		}
	}
	if len(wanted) == 0 || s.seatBatch == nil || !auth.ReadGranted(ctx, "relationship") {
		return out, nil
	}
	seats, err := s.seatBatch(ctx, tx, wanted, s.now())
	if err != nil {
		return nil, err
	}
	for id, rows := range seats {
		access := assessmentSeatAccess{visible: map[ids.UUID]bool{}, named: map[ids.UUID]bool{}}
		for _, seat := range rows {
			access.visible[seat.ContactID] = true
			if seat.Name != "" {
				access.named[seat.ContactID] = true
			}
		}
		out[id] = access
	}
	return out, nil
}

func (s *Service) assessmentMasks(ctx context.Context, tx pgx.Tx, cards map[ids.UUID]CachedCard) (map[ids.UUID]bool, error) {
	allowed := make(map[ids.UUID]bool, len(cards))
	if s.deals == nil || len(cards) == 0 {
		return allowed, nil
	}
	wanted := make([]ids.UUID, 0, len(cards))
	for id := range cards {
		wanted = append(wanted, id)
	}
	limit := 200
	input := deals.ListDealsInput{IDs: wanted, Limit: &limit}
	for {
		page, next, err := s.deals.ListDealsTx(ctx, tx, input, deals.CustomColumns{})
		if err != nil {
			return nil, err
		}
		for _, deal := range page {
			id := ids.UUID(deal.Id)
			card := cards[id]
			if card.access == nil {
				continue
			}
			allowed[id] = true
			if deal.MaskedFields != nil {
				for _, field := range *deal.MaskedFields {
					if !slices.Contains(card.access.Masks, field) {
						allowed[id] = false
						break
					}
				}
			}
		}
		if !next.HasMore {
			return allowed, nil
		}
		input.Cursor = &next.NextCursor
	}
}
