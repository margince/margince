// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/dealrooms"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// timelineWindow bounds how much of the timeline the card reads. The timeline
// is ordered by occurred_at DESC, so the window holds the nearest scheduled
// rows first and then the most recent past ones.
const timelineWindow = 25

// refreshFloor is the one clock in an otherwise fact-keyed cache. A deal whose
// facts churn on every read — a live mail thread, a room somebody is typing in
// — would otherwise rewrite on every page load. Below this age the cached card
// is served even when the fingerprint has moved.
const refreshFloor = 5 * time.Minute

// laneDeadline caps the model call. The card is read inline on the deal page
// and the API's write window is 30s, so the lane must give up early enough
// that the deterministic floor still answers within it.
const laneDeadline = 15 * time.Second

// Service gathers the facts, writes the card, and caches it per reader.
type Service struct {
	pool           *pgxpool.Pool
	deals          *deals.Store
	activities     *activities.Store
	rooms          *dealrooms.Store
	lane           Completer
	routingVersion string
	now            func() time.Time
}

// NewService binds the reads. Each store carries its own gates.
func NewService(pool *pgxpool.Pool, d *deals.Store, a *activities.Store, r *dealrooms.Store, now func() time.Time) *Service {
	return &Service{pool: pool, deals: d, activities: a, rooms: r, now: now}
}

// WithLane binds the deal_health lane that writes the card's words. Without
// it every card is the deterministic floor, which is a working card.
func (s *Service) WithLane(lane Completer, routingVersion string) *Service {
	s.lane = lane
	s.routingVersion = routingVersion
	return s
}

// facts are the inputs the card is written from.
type facts struct {
	deal      crmcontracts.Deal
	health    *deals.DealHealth
	timeline  []crmcontracts.Activity
	openTasks []activities.OpenTask
	// moreTasks says the open-task read was cut at its window, so the count
	// is a floor rather than the number.
	moreTasks bool
	room      *crmcontracts.DealRoom
	threads   []crmcontracts.DealRoomThread
	now       time.Time
}

// stored is the cached envelope. The whole thing round-trips through the
// payload column, version included: unmarshalling into the card alone would
// leave the fingerprint empty, the check could never pass, and the cache would
// never once answer — a rewrite on every page load, invisible to a gate
// because nothing about the served card looks wrong.
type stored struct {
	Fingerprint string                      `json:"-"`
	GeneratedAt time.Time                   `json:"-"`
	GeneratedBy crmcontracts.WrittenBy      `json:"-"`
	Card        crmcontracts.DealStatusCard `json:"card"`
}

// Get returns the deal's status card, writing it when no current one is
// cached. A refresh forces the rewrite: the reader asking for a second
// opinion.
func (s *Service) Get(ctx context.Context, dealID ids.DealID, refresh bool) (crmcontracts.DealStatusCard, error) {
	userID, err := actingUser(ctx)
	if err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	// Gathered FIRST, and under the caller's row scope: a deal they cannot
	// read refuses here, before any cache is consulted.
	f, err := s.gather(ctx, dealID)
	if err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	mv := decideMove(f)
	in := project(f, mv)
	fingerprint, err := Fingerprint(in, userID.UUID, s.routingVersion)
	if err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	if card, ok := s.serveCached(ctx, userID, dealID, fingerprint, refresh, f.now); ok {
		return card, nil
	}
	card := s.write(ctx, f, mv, in)
	if err := s.save(ctx, userID, dealID, stored{
		Fingerprint: fingerprint,
		GeneratedAt: card.GeneratedAt,
		GeneratedBy: card.GeneratedBy,
		Card:        card,
	}); err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	return card, nil
}

// serveCached decides whether the stored card still stands. Two ways it does:
// its fingerprint matches the facts, or it is younger than the refresh floor
// and so not worth rewriting yet.
func (s *Service) serveCached(
	ctx context.Context, userID ids.UserID, dealID ids.DealID, fingerprint string, refresh bool, now time.Time,
) (crmcontracts.DealStatusCard, bool) {
	if refresh {
		return crmcontracts.DealStatusCard{}, false
	}
	cached, found, err := s.cached(ctx, userID, dealID)
	if err != nil || !found {
		// A cache that cannot be read is a miss, never a failed request: the
		// card is derived content and writing it again is the whole fallback.
		return crmcontracts.DealStatusCard{}, false
	}
	if cached.Fingerprint == fingerprint || now.Sub(cached.GeneratedAt) < refreshFloor {
		return cached.Card, true
	}
	return crmcontracts.DealStatusCard{}, false
}

// write asks the lane for the card's words and falls back to the floor. A
// refused, absent or over-budget lane is the declared degrade posture, not an
// error to surface: the reader gets the deterministic card and generated_by
// says so.
func (s *Service) write(
	ctx context.Context, f facts, mv crmcontracts.DealStatusCardMove, in StatusInput,
) crmcontracts.DealStatusCard {
	floor := composeDeterministic(f, mv)
	if s.lane == nil {
		return floor
	}
	laneCtx, cancel := context.WithTimeout(ctx, laneDeadline)
	defer cancel()
	written, err := s.ask(laneCtx, in)
	if err != nil {
		return floor
	}
	return foldWritten(floor, written, f, mv)
}

func (s *Service) ask(ctx context.Context, in StatusInput) (WrittenStatus, error) {
	resp, err := s.lane.Complete(ctx, StatusRequest(in))
	if err != nil {
		return WrittenStatus{}, err
	}
	return ParseStatus(resp.Text, in)
}

func (s *Service) gather(ctx context.Context, dealID ids.DealID) (facts, error) {
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
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: &limit,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading the deal's timeline: %w", err)
	}
	f.timeline = timeline
	open, more, err := s.activities.ListOpenTasks(ctx, activities.ListOpenTasksInput{
		EntityType: &entityType, EntityID: &dealID.UUID, Limit: timelineWindow,
	})
	if err != nil {
		return facts{}, fmt.Errorf("deal status: reading the deal's open tasks: %w", err)
	}
	f.openTasks, f.moreTasks = open, more
	if err := s.gatherRoom(ctx, dealID, &f); err != nil {
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

// cached reads this user's card for this deal. The user_id predicate is
// explicit: the workspace binding is the transaction's, so without it one rep
// would read another's card — written from records they may not share.
func (s *Service) cached(ctx context.Context, userID ids.UserID, dealID ids.DealID) (stored, bool, error) {
	var out stored
	var payload []byte
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT fingerprint, generated_at, generated_by, payload FROM deal_status_card
			WHERE user_id = $1 AND deal_id = $2`,
			userID, dealID).Scan(&out.Fingerprint, &out.GeneratedAt, &out.GeneratedBy, &payload)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, false, nil
	}
	if err != nil {
		return stored{}, false, err
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		// A payload this build cannot read is a cache MISS, not a failure: the
		// card is derived content, writing it again is cheap, and the new row
		// replaces the unreadable one.
		//nolint:nilerr // an unreadable cache entry is a miss by design; the caller regenerates
		return stored{}, false, nil
	}
	return out, true, nil
}

func (s *Service) save(ctx context.Context, userID ids.UserID, dealID ids.DealID, card stored) error {
	payload, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("encode the deal status payload: %w", err)
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO deal_status_card (user_id, deal_id, fingerprint,
			                              generated_at, generated_by, payload)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, deal_id) DO UPDATE
			SET fingerprint = EXCLUDED.fingerprint,
			    generated_at = EXCLUDED.generated_at,
			    generated_by = EXCLUDED.generated_by,
			    payload = EXCLUDED.payload`,
			userID, dealID, card.Fingerprint, card.GeneratedAt, card.GeneratedBy, payload)
		return err
	})
}

// actingUser resolves the human this card belongs to. That the card is
// per-user IS the security posture, so a principal with no user id has no
// card rather than a shared one.
func actingUser(ctx context.Context) (ids.UserID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return ids.UserID{}, fmt.Errorf("the deal status card is per-user and this call carries no user: %w",
			apperrors.ErrPermissionDenied)
	}
	return ids.From[ids.UserKind](p.UserID), nil
}
