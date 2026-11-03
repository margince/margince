// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Service) currentMoveFacts(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]facts, error) {
	now := s.now()
	out, err := s.moveDeals(ctx, tx, dealIDs, now)
	if err != nil {
		return nil, err
	}
	if s.seatsOf != nil && s.seatBatch == nil {
		return nil, errors.New("the deal seat batch reader is not configured")
	}
	if err := s.moveActivities(ctx, tx, dealIDs, out, now); err != nil {
		return nil, err
	}
	followUps, err := s.activities.NextMeetingFollowUpsByDealTx(ctx, tx, now, dealIDs)
	if err != nil {
		return nil, err
	}
	var seats map[ids.UUID][]Seat
	if s.seatBatch != nil {
		seats, err = s.seatBatch(ctx, tx, dealIDs, now)
		if err != nil {
			return nil, err
		}
	}
	for id, f := range out {
		f.followUps, f.seats = followUps[id], seats[id]
		out[id] = f
	}
	return out, nil
}

func (s *Service) moveDeals(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time) (map[ids.UUID]facts, error) {
	loc, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	lang := string(identity.BaseLanguageForRecord(ctx, tx))
	limit := 200
	in := deals.ListDealsInput{IDs: dealIDs, Limit: &limit}
	out := map[ids.UUID]facts{}
	for {
		page, next, err := s.deals.ListDealsTx(ctx, tx, in, deals.CustomColumns{})
		if err != nil {
			return nil, err
		}
		for _, deal := range page {
			out[ids.UUID(deal.Id)] = facts{deal: deal, now: now, loc: loc, lang: lang, contactChecked: true}
		}
		if !next.HasMore {
			return out, nil
		}
		in.Cursor = &next.NextCursor
	}
}

func (s *Service) moveActivities(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, out map[ids.UUID]facts, now time.Time) error {
	one := 1
	mail := string(crmcontracts.ActivityKindEmail)
	current, err := activities.ListActivitiesByDealTx(ctx, tx, dealIDs, activities.ListActivitiesInput{
		Kind: &mail, Limit: &one, RequestCurrentAsOf: &now, ReadableOnly: true,
	})
	if err != nil {
		return err
	}
	review, err := activities.ListActivitiesByDealTx(ctx, tx, dealIDs, activities.ListActivitiesInput{
		Kind: &mail, Limit: &one, RequestReviewAsOf: &now, ReadableOnly: true,
	})
	if err != nil {
		return err
	}
	exchanges, err := activities.ListActivitiesByDealTx(ctx, tx, dealIDs, activities.ListActivitiesInput{
		Limit: &one, InteractionsAsOf: &now,
	})
	if err != nil {
		return err
	}
	open, err := activities.OpenTasksByDealTx(ctx, tx, dealIDs, activities.ListOpenTasksInput{
		AsOf: now, Limit: one, ExcludeSystemMinted: true,
	})
	if err != nil {
		return err
	}
	meetings, err := activities.NextBookedMeetingsByDealTx(ctx, tx, dealIDs, now)
	if err != nil {
		return err
	}
	for id, f := range out {
		f.currentRequests, f.requests = current[id].Activities, review[id].Activities
		f.openTasks, f.moreTasks = open[id].Tasks, open[id].Truncated
		if rows := exchanges[id].Activities; len(rows) > 0 {
			f.lastExchange = &rows[0]
		}
		if meeting, ok := meetings[id]; ok {
			f.nextMeeting = &meeting
		}
		out[id] = f
	}
	return nil
}
