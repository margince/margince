// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *Service) gather(ctx context.Context, dealID ids.DealID) (facts, error) {
	var out facts
	err := database.WithWorkspaceSnapshot(ctx, s.pool, func(ctx context.Context) error {
		// Each read carries kilobytes of row scope and plans for longer than it
		// runs, so the card's reads keep their generic plans.
		return database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			var err error
			out, err = s.gatherSnapshot(ctx, dealID)
			return err
		})
	})
	return out, err
}

func (s *Service) gatherSnapshot(ctx context.Context, dealID ids.DealID) (facts, error) {
	f := facts{now: s.now()}
	deal, err := s.deals.GetDeal(ctx, dealID, storekit.LiveOnly)
	if err != nil {
		return facts{}, err
	}
	f.deal = deal
	health, err := s.deals.DealHealth(ctx, dealID, f.now)
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading the deal's health: %w", err)
	}
	f.health = &health
	entityType := "deal"
	limit := timelineWindow
	timeline, _, err := s.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: &limit, CollapseSeatCopies: true,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading the deal's timeline: %w", err)
	}
	f.timeline = timeline
	if err := s.gatherLastExchange(ctx, dealID, &f); err != nil {
		return facts{}, err
	}
	requestKind := string(crmcontracts.ActivityKindEmail)
	f.currentRequests, _, err = s.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: &limit,
		RequestCurrentAsOf: &f.now, Kind: &requestKind, ReadableOnly: true, CollapseSeatCopies: true,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading current requests: %w", err)
	}
	f.requests, _, err = s.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: &limit,
		RequestReviewAsOf: &f.now, Kind: &requestKind, ReadableOnly: true, CollapseSeatCopies: true,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading outstanding requests: %w", err)
	}
	// The card reads an open task as a step somebody agreed. A system-minted
	// review or reminder is housekeeping. The module's predicate drops it before
	// the page is cut.
	open, more, err := s.activities.ListOpenTasks(ctx, activities.ListOpenTasksInput{
		AsOf:       f.now,
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: timelineWindow,
		ExcludeSystemMinted: true,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading the deal's open tasks: %w", err)
	}
	f.openTasks, f.moreTasks = open, more
	// Asked of the database, not read off the timeline page above. That page
	// holds the newest rows only, and a busy deal hides its booked meeting there.
	meeting, booked, err := s.activities.NextBookedMeeting(ctx, activities.NextBookedMeetingInput{
		EntityType: entityType, EntityID: dealID.UUID, After: f.now,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading the deal's next booked meeting: %w", err)
	}
	if booked {
		f.nextMeeting = &meeting
	}
	if f.followUps, err = s.activities.NextMeetingFollowUpOnDeal(ctx, f.now, dealID.UUID); err != nil {
		return facts{}, fmt.Errorf("deal status: reading the meetings owed a follow-up: %w", err)
	}
	s.gatherMailCoverage(ctx, &f)
	if err := s.gatherRoom(ctx, dealID, &f); err != nil {
		return facts{}, err
	}
	if s.seatsOf != nil {
		seats, err := s.seatsOf(ctx, dealID, f.now)
		if err != nil {
			return facts{}, fmt.Errorf("deal status: reading who is on the deal: %w", err)
		}
		f.seats = seats
	}
	if err = s.gatherInstallation(ctx, &f); err != nil {
		return facts{}, err
	}
	return f, nil
}

// gatherRoom reads the deal's room, when there is one, with its conversation.
// A caller without the deal_room grant reads no room; the card then simply
// says nothing about one, which is the truth for them.
func (s *Service) gatherRoom(ctx context.Context, dealID ids.DealID, f *facts) error {
	rooms, _, err := s.rooms.ListRooms(ctx, dealrooms.ListRoomsInput{DealID: &dealID})
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return fmt.Errorf("deal status: reading the deal's room: %w", err)
	}
	if len(rooms) == 0 {
		return nil
	}
	room := rooms[0]
	f.room = &room
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	threads, err := s.rooms.ListThreads(ctx, roomID, nil)
	if err != nil {
		return fmt.Errorf("deal status: reading the room's conversation: %w", err)
	}
	f.threads = threads
	return nil
}

func (s *Service) gatherLastExchange(ctx context.Context, dealID ids.DealID, f *facts) error {
	entityType := "deal"
	one := 1
	contactRows, _, err := s.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: &one, InteractionsAsOf: &f.now, CollapseSeatCopies: true,
	})
	if err != nil {
		return fmt.Errorf("deal status: reading the last recorded exchange: %w", err)
	}
	f.contactChecked = true
	if len(contactRows) > 0 {
		f.lastExchange = &contactRows[0]
	}
	return nil
}
