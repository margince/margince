// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// timelineWindow bounds how much of the timeline the card reads. The timeline
// is ordered by occurred_at DESC, so the window holds the nearest scheduled
// rows first and then the most recent past ones.
const timelineWindow = 25

// modelCallFloor is the one clock in an otherwise fact-keyed cache, and it
// bounds MODEL CALLS rather than the card's freshness.
//
// A deal whose facts churn on every read — a live mail thread, a room somebody
// is typing in — would otherwise pay for a model call on every page load. Below
// this age the card is rewritten deterministically instead: the reader still
// sees the change immediately, and generated_by tells them a composition wrote
// it. Serving the STALE card here would be the mistake an hourly refresh makes,
// which is describing the deal as it was before the thing that just happened.
const modelCallFloor = 5 * time.Minute

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
	seatsOf        SeatReader
	lane           Completer
	routingVersion string
	now            func() time.Time
}

// NewService binds the reads. Each store carries its own gates.
func NewService(pool *pgxpool.Pool, d *deals.Store, a *activities.Store, r *dealrooms.Store, now func() time.Time) *Service {
	return &Service{pool: pool, deals: d, activities: a, rooms: r, now: now}
}

// WithSeats binds the read that says who is on the deal. Without it the card
// is written from the deal and its timeline alone, which is what it did before
// the seats were reachable — a working card that cannot name a first move on a
// deal nobody has contacted yet.
func (s *Service) WithSeats(read SeatReader) *Service {
	s.seatsOf = read
	return s
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
	// seats are the people on the deal with their roles. Empty when nobody is
	// named AND when the reader may not read the stakeholder edge — the card
	// cannot tell those apart and says nothing about seats in either case,
	// which is the honest answer for both.
	seats []Seat
	now   time.Time
	// lang is the installation's base language, which the card is written in.
	// A status card is filed on the deal and read by anyone who opens it, so it
	// takes the shared language rather than the reader's — the same rule that
	// separates a record from a piece of correspondence.
	lang string
}

// stored is the cached envelope: the card in the payload column, and the three
// fields the cache decision needs in columns of their own.
//
// The split is deliberate. Deciding whether a stored card still stands means
// comparing a fingerprint and an age, and those are answered from the row
// without decoding the payload at all — a card this build cannot unmarshal is
// then a MISS rather than a failed request, because the fields that decide
// were never inside the thing that failed to decode.
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
	fingerprint, err := Fingerprint(in, userID.UUID, s.routingVersion, f.now, f.lang)
	if err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	verdict := s.decideFromCache(ctx, userID, dealID, fingerprint, refresh, f.now)
	if verdict.serve {
		return verdict.card, nil
	}
	card, laneFailed := s.write(ctx, f, mv, in, verdict.askModel)
	if laneFailed {
		// The lane was wired and did not answer: a timeout, a budget, an
		// unparseable reply. The reader gets the floor, which is a working
		// card — but storing it under the CURRENT fingerprint would make one
		// transient outage permanent, because a matching fingerprint is served
		// without ever asking again. The card would stand until some unrelated
		// fact moved.
		//
		// This is reachable in a way it was not before: the language is part of
		// the fingerprint now, so an installation that switches to German mints
		// a new key, and a lane that happens to be down while it does would
		// freeze the ENGLISH floor as that installation's German card.
		return card, nil
	}
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

// cacheVerdict says what to do with the stored card.
type cacheVerdict struct {
	// card is served as-is when serve is true: the facts have not moved.
	card  crmcontracts.DealStatusCard
	serve bool
	// askModel is false when the facts moved but a model call is inside the
	// floor. The card is still rewritten — deterministically — so the reader
	// sees the change rather than the world as it was.
	askModel bool
}

// decideFromCache reads the stored card and says whether it still stands.
func (s *Service) decideFromCache(
	ctx context.Context, userID ids.UserID, dealID ids.DealID, fingerprint string, refresh bool, now time.Time,
) cacheVerdict {
	if refresh {
		return cacheVerdict{askModel: true}
	}
	cached, found, err := s.cached(ctx, userID, dealID)
	if err != nil || !found {
		// A cache that cannot be read is a miss, never a failed request: the
		// card is derived content and writing it again is the whole fallback.
		return cacheVerdict{askModel: true}
	}
	if cached.Fingerprint == fingerprint {
		return cacheVerdict{card: cached.Card, serve: true}
	}
	return cacheVerdict{askModel: now.Sub(cached.GeneratedAt) >= modelCallFloor}
}

// write asks the lane for the card's words and falls back to the floor. A
// refused, absent or over-budget lane is the declared degrade posture, not an
// error to surface: the reader gets the deterministic card and generated_by
// says so.
// The second return says the lane FAILED, as opposed to being absent or held
// back by the call floor. Both of those produce the same card, and only the
// first is a reason not to cache it: a lane nobody wired will answer no
// differently next time, so its floor is the real answer and belongs in the
// cache, while a lane that timed out will.
func (s *Service) write(
	ctx context.Context, f facts, mv crmcontracts.DealStatusCardMove, in StatusInput, askModel bool,
) (card crmcontracts.DealStatusCard, laneFailed bool) {
	floor := composeDeterministic(f, mv)
	if s.lane == nil || !askModel {
		return floor, false
	}
	// A deal with nothing to cite cannot be written by the model, so it is not
	// asked. Every story sentence must carry a citation that resolves against
	// the timeline or the open tasks (keepGrounded), and ParseStatus refuses a
	// reply whose story is then empty — so with neither, the lane's answer is
	// discarded whatever it says. Asking anyway spent a model call and up to
	// laneDeadline of the reader's wait to arrive at the floor that was
	// already in hand.
	//
	// This is a REFUSAL TO ASK, not a new degrade: the card served here is the
	// same floor the same deal got before, and generated_by already says
	// deterministic. What changes is that the reader gets it at once.
	if len(citableIDs(in)) == 0 {
		return floor, false
	}
	laneCtx, cancel := context.WithTimeout(ctx, laneDeadline)
	defer cancel()
	written, err := s.ask(laneCtx, in, f.lang)
	if err != nil {
		// The degrade is declared, but a SILENT one is indistinguishable from
		// a lane nobody wired: the reader sees a deterministic card either
		// way, and only this line says which. It carries the reason rather
		// than the reply, because the reply is the buyer's words.
		slog.WarnContext(ctx, "deal status fell back to the deterministic card",
			"deal_id", f.deal.Id.String(), "reason", err)
		return floor, true
	}
	return foldWritten(floor, written, f, mv), false
}

func (s *Service) ask(ctx context.Context, in StatusInput, lang string) (WrittenStatus, error) {
	resp, err := ai.Ask(ctx, s.lane, StatusRequest(in, lang), func(text string) error {
		_, err := ParseStatus(text, in)
		return err
	})
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
	if s.seatsOf != nil {
		seats, err := s.seatsOf(ctx, dealID, f.now)
		if err != nil {
			return facts{}, fmt.Errorf("deal status: reading who is on the deal: %w", err)
		}
		f.seats = seats
	}
	f.lang = identity.BaseLanguageForPrompt(ctx, s.pool)
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
