// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The bounds a reader actually reads. A reason is a paragraph and a note is a
// short mail; past these nobody reads either, and the colleague deciding is
// the person who pays for a wall of text.
const (
	reasonBound = 2000
	noteBound   = 4000
)

// Store owns the intro_request table.
type Store struct {
	db  *database.DB
	now func() time.Time
}

// NewStore binds the store to db.
func NewStore(db *database.DB, now func() time.Time) *Store {
	return &Store{db: db, now: now}
}

// Request is one ask, as the surfaces read it.
type Request struct {
	ID               ids.UUID
	PersonID         ids.UUID
	RequesterUserID  ids.UUID
	IntroducerUser   ids.UUID
	RouteType        string
	ThroughPersonID  *ids.UUID
	InternalReason   string
	ValueForTarget   string
	ForwardableNote  string
	NoteGeneratedBy  string
	NoteAIGenerated  bool
	FallbackPolicy   string
	NameDropAllowed  bool
	Status           Status
	DecisionReason   *string
	SuggestedUserID  *ids.UUID
	SourceActivityID *ids.UUID
	DueAt            time.Time
	RequestedAt      time.Time
	DecidedAt        *time.Time
	IntroducedAt     *time.Time
	NameDroppedAt    *time.Time
	RepliedAt        *time.Time
	Version          int
}

// NewRequest is an ask being made.
type NewRequest struct {
	PersonID        ids.UUID
	IntroducerUser  ids.UUID
	RouteType       string
	ThroughPersonID *ids.UUID
	InternalReason  string
	ValueForTarget  string
	ForwardableNote string
	NoteGeneratedBy string
	NoteAIGenerated bool
	FallbackPolicy  string
	NameDropAllowed bool
	DueAt           time.Time
}

// Create records one ask.
//
// The requester is the authenticated person and never a field on the request:
// a body that could name its own requester would let one rep put an ask in
// another's name, and the colleague answering would be answering the wrong
// person.
func (s *Store) Create(ctx context.Context, req NewRequest) (ids.UUID, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		// Asking a colleague for a favour is a person's act. An agent holding
		// a human's id is not that human deciding to spend their goodwill.
		return ids.UUID{}, fmt.Errorf(
			"introductions: asking for an introduction needs an authenticated person: %w",
			apperrors.ErrPermissionDenied)
	}
	if req.InternalReason == "" {
		return ids.UUID{}, errors.New("introductions: an ask says why it is worth making")
	}
	// The contact has to be one this rep can actually see. Without it an ask
	// would name a person the requester cannot open, and the colleague's
	// screen would disclose them.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return ids.UUID{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, err
	}

	id := ids.NewV7()
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, "person", req.PersonID); err != nil {
			return err
		}
		// The intermediary is named by the caller too, and naming a record is
		// reading it: without this a rep could learn that a contact exists by
		// routing an ask through them and reading which error came back.
		if req.ThroughPersonID != nil {
			if err := auth.EnsureVisibleLive(ctx, tx, "person", *req.ThroughPersonID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO intro_request (
				id, person_id, requester_user_id, introducer_user_id,
				route_type, through_person_id,
				internal_reason, value_for_target, forwardable_note,
				note_generated_by, note_ai_generated,
				fallback_policy, name_drop_allowed, due_at, captured_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			id, req.PersonID, actor.UserID, req.IntroducerUser,
			req.RouteType, req.ThroughPersonID,
			truncate(req.InternalReason, reasonBound),
			truncate(req.ValueForTarget, reasonBound),
			truncate(req.ForwardableNote, noteBound),
			noteOrigin(req.NoteGeneratedBy), req.NoteAIGenerated,
			fallbackOrNone(req.FallbackPolicy), req.NameDropAllowed,
			req.DueAt, capturedBy)
		if err != nil {
			// The partial unique index is the duplicate guard, not a
			// read-then-write check: two tabs pressing send at once both pass
			// a check and only one may pass the index.
			if storekit.IsUniqueViolation(err) {
				return fmt.Errorf(
					"introductions: this colleague has already been asked about this contact: %w",
					apperrors.ErrConflict)
			}
			return fmt.Errorf("introductions: recording the ask: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", "intro_request", id,
			map[string]any{
				"person_id":          req.PersonID.String(),
				"introducer_user_id": req.IntroducerUser.String(),
				"route_type":         req.RouteType,
			})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, req.PersonID,
			crmcontracts.PublicEventIntroRequestCreated{
				IntroRequestId:   openapi_types.UUID(id),
				PersonId:         openapi_types.UUID(req.PersonID),
				RequesterUserId:  openapi_types.UUID(actor.UserID),
				IntroducerUserId: openapi_types.UUID(req.IntroducerUser),
			})
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return id, nil
}

// Decide records the colleague's answer.
//
// The version is the caller's: two tabs open on the same ask, one accepting
// and one declining, must not both win. The loser is told the record moved
// rather than silently overwriting an answer already given.
func (s *Store) Decide(
	ctx context.Context, id ids.UUID, answer Status, reason string,
	suggested *ids.UUID, version int,
) error {
	if answer == StatusSuggestOther && suggested == nil {
		return errors.New("introductions: suggesting somebody else names them")
	}
	return s.move(ctx, id, func() Status { return answer }, version, func(cur *Request) error {
		return May(cur.Status, answer, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `
			UPDATE intro_request
			   SET status = $2, decision_reason = $3, suggested_user_id = $4,
			       decided_at = $5, version = version + 1, updated_at = now(),
			       closed_at = CASE WHEN $2 IN ('declined','suggest_other') THEN $5 ELSE closed_at END
			 WHERE id = $1 AND version = $6`,
			id, string(answer), nullIfEmpty(reason), suggested, s.now(), version)
	}, func(cur *Request) events.Payload {
		return crmcontracts.PublicEventIntroRequestDecided{
			IntroRequestId:   openapi_types.UUID(id),
			PersonId:         openapi_types.UUID(cur.PersonID),
			IntroducerUserId: openapi_types.UUID(cur.IntroducerUser),
			Decision:         crmcontracts.PublicEventIntroRequestDecidedDecision(answer),
		}
	})
}

// Complete records the handshake, or the rep having used a lent name.
//
// Which one it is comes from the state the ask is IN, never from the caller:
// an accepted ask completes as an introduction and a name-drop-approved one as
// a name-drop, so no caller can report a handshake that only had permission
// behind it.
func (s *Store) Complete(ctx context.Context, id ids.UUID, activity *ids.UUID, version int) error {
	var outcome Status
	// The audit after-image is read from `outcome`, not from a status fixed at
	// call time: a name-drop audited as `introduced` would put the claimed
	// handshake back into the trail that exists to deny it.
	return s.move(ctx, id, func() Status { return outcome }, version, func(cur *Request) error {
		outcome = StatusIntroduced
		if cur.Status == StatusNameDropApproved {
			outcome = StatusNameDropped
		}
		return May(cur.Status, outcome, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) (pgconn.CommandTag, error) {
		// The activity is the evidence the claim rests on, and the caller names
		// it. Bound it to this ask's own contact: without that, a party could
		// cite any message id as proof of an introduction that never happened.
		//
		// Audience is what governs who may READ an activity's content, and it
		// lives in another module — so this refuses on the one relation this
		// module owns the question for, and stores no content it cannot show.
		if activity != nil {
			var linked bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM activity_link
					 WHERE activity_id = $1 AND entity_type = 'person' AND person_id = $2)`,
				*activity, cur.PersonID).Scan(&linked); err != nil {
				return pgconn.CommandTag{}, fmt.Errorf(
					"introductions: checking the evidence: %w", err)
			}
			if !linked {
				return pgconn.CommandTag{}, fmt.Errorf(
					"introductions: that message is not about this contact: %w",
					apperrors.ErrNotFound)
			}
		}
		column := "introduced_at"
		if outcome == StatusNameDropped {
			column = "name_dropped_at"
		}
		// The column name is a compile-time literal chosen from two, never a
		// value off a request: a formatted identifier from a body is how a
		// write becomes an injection.
		return tx.Exec(ctx, fmt.Sprintf(`
			UPDATE intro_request
			   SET status = $2, %s = $3, source_activity_id = COALESCE($4, source_activity_id),
			       version = version + 1, updated_at = now()
			 WHERE id = $1 AND version = $5`, column),
			id, string(outcome), s.now(), activity, version)
	}, func(cur *Request) events.Payload {
		return crmcontracts.PublicEventIntroRequestCompleted{
			IntroRequestId: openapi_types.UUID(id),
			PersonId:       openapi_types.UUID(cur.PersonID),
			Outcome:        crmcontracts.PublicEventIntroRequestCompletedOutcome(outcome),
		}
	})
}

// Cancel withdraws the ask.
func (s *Store) Cancel(ctx context.Context, id ids.UUID, reason string, version int) error {
	return s.move(ctx, id, func() Status { return StatusCancelled }, version, func(cur *Request) error {
		return May(cur.Status, StatusCancelled, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `
			UPDATE intro_request
			   SET status = 'cancelled', decision_reason = $2, closed_at = $3,
			       version = version + 1, updated_at = now()
			 WHERE id = $1 AND version = $4`,
			id, nullIfEmpty(reason), s.now(), version)
	}, func(cur *Request) events.Payload {
		return crmcontracts.PublicEventIntroRequestClosed{
			IntroRequestId: openapi_types.UUID(id),
			PersonId:       openapi_types.UUID(cur.PersonID),
			Reason:         crmcontracts.IntroRequestClosedCancelled,
		}
	})
}

// move is the one write path every transition takes.
//
// Read the row, check the caller's version, ask the lifecycle whether this
// actor may make this move, write it, then audit and emit — all in one
// transaction, so an ask cannot change state without the trail that explains
// it. The event is built by the caller and emitted HERE rather than through a
// callback: the audit-and-emit pair is the write shape, and a pair split
// across an injected function is a pair a reader has to go looking for.
func (s *Store) move(
	ctx context.Context,
	id ids.UUID,
	to func() Status,
	version int,
	permit func(*Request) error,
	write func(context.Context, pgx.Tx, *Request) (pgconn.CommandTag, error),
	event func(*Request) events.Payload,
) error {
	if err := auth.Require(ctx, "introduction", principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := s.load(ctx, tx, id)
		if err != nil {
			return err
		}
		// Version BEFORE the transition rules. A tab that read the ask while it
		// was open and pressed decline after somebody accepted is looking at a
		// stale record, not attempting an illegal move — telling it the move
		// was impossible sends the reader to check the state machine when what
		// they need to do is reload.
		if cur.Version != version {
			return fmt.Errorf(
				"introductions: this ask moved since you read it: %w", apperrors.ErrVersionSkew)
		}
		if err := permit(cur); err != nil {
			return err
		}
		tag, err := write(ctx, tx, cur)
		if err != nil {
			return fmt.Errorf("introductions: recording the answer: %w", err)
		}
		// The read above took no row lock, so the version check can pass in two
		// transactions at once. Only `AND version = $N` inside the UPDATE is
		// atomic, and it reports a loss by matching zero rows. Without this the
		// loser writes an audit row and emits an event for a move it never made.
		if tag.RowsAffected() == 0 {
			return fmt.Errorf(
				"introductions: this ask moved since you read it: %w", apperrors.ErrVersionSkew)
		}
		// The read above took no lock, so the version check at line 304 can pass
		// in two transactions at once. Only `AND version = $N` inside the UPDATE
		// is atomic, and it reports a loss by matching zero rows. Without this,
		// the loser writes an audit row and emits an event for a transition that
		// did not happen — the trail would say an ask was declined when it was
		// accepted.
		// The before-image is the status this ask was in, which is exactly what
		// a dispute about an introduction asks: an accepted ask that ended up
		// declined and one that was declined outright are different stories,
		// and only the pair tells them apart.
		auditID, err := storekit.Audit(ctx, tx, "update", "intro_request", id,
			map[string]any{"status": string(cur.Status)},
			map[string]any{"status": string(to())})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, cur.PersonID, event(cur))
	})
}

// load reads one ask, refusing it to anybody it is not about.
//
// A stranger gets ErrNotFound rather than a refusal: telling them the ask
// exists but is not theirs discloses that this colleague was asked about this
// contact, which is the fact the row is for.
func (s *Store) load(ctx context.Context, tx pgx.Tx, id ids.UUID) (*Request, error) {
	var r Request
	err := scanRequest(
		tx.QueryRow(ctx, requestColumns+` FROM intro_request
			 WHERE id = $1 AND archived_at IS NULL`, id), &r)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("introductions: reading the ask: %w", err)
	}
	if s.roleOf(ctx, &r) == "" {
		return nil, apperrors.ErrNotFound
	}
	return &r, nil
}

// requestColumns and scanRequest are ONE list read twice. Two spellings of the
// same SELECT drift the moment a column is added, and the reader that still
// works is the one whose scan happens to line up.
const requestColumns = `
	SELECT id, person_id, requester_user_id, introducer_user_id, route_type,
	       through_person_id, internal_reason, value_for_target, forwardable_note,
	       note_generated_by, note_ai_generated, fallback_policy, name_drop_allowed,
	       status, decision_reason, suggested_user_id, source_activity_id,
	       due_at, requested_at, decided_at, introduced_at, name_dropped_at,
	       replied_at, version`

// row is what pgx.Row and pgx.Rows both are, for the one scan they share.
type row interface{ Scan(dest ...any) error }

func scanRequest(src row, r *Request) error {
	return src.Scan(
		&r.ID, &r.PersonID, &r.RequesterUserID, &r.IntroducerUser, &r.RouteType,
		&r.ThroughPersonID, &r.InternalReason, &r.ValueForTarget, &r.ForwardableNote,
		&r.NoteGeneratedBy, &r.NoteAIGenerated, &r.FallbackPolicy, &r.NameDropAllowed,
		&r.Status, &r.DecisionReason, &r.SuggestedUserID, &r.SourceActivityID,
		&r.DueAt, &r.RequestedAt, &r.DecidedAt, &r.IntroducedAt, &r.NameDroppedAt,
		&r.RepliedAt, &r.Version)
}

// roleOf answers which party the caller is, or "" for anybody else.
func (s *Store) roleOf(ctx context.Context, r *Request) Actor {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ""
	}
	switch actor.UserID {
	case r.RequesterUserID:
		return ActorRequester
	case r.IntroducerUser:
		return ActorIntroducer
	default:
		return ""
	}
}

// Get reads one ask for a party to it. Anybody else gets ErrNotFound, for the
// reason load states.
func (s *Store) Get(ctx context.Context, id ids.UUID) (*Request, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return nil, err
	}
	var out *Request
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		r, err := s.load(ctx, tx, id)
		if err != nil {
			return err
		}
		out = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ForPerson lists the asks about one contact, newest first.
func (s *Store) ForPerson(ctx context.Context, personID ids.UUID, limit int) ([]Request, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []Request
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Only the asks this caller is party to. A rep browsing a contact does
		// not learn which of their colleagues has been asked about them and
		// refused: that is the introducer's answer to give, not the record's.
		// The full row, not a summary: the colleague deciding reads the reason
		// and the note on this payload, and a second round-trip per ask to
		// fetch what the list already had in hand would be one.
		rows, err := tx.Query(ctx, requestColumns+`
			  FROM intro_request
			 WHERE person_id = $1 AND archived_at IS NULL
			   AND (requester_user_id = $2 OR introducer_user_id = $2)
			 ORDER BY requested_at DESC, id DESC
			 LIMIT $3`, personID, actor.UserID, limit)
		if err != nil {
			return fmt.Errorf("introductions: listing the asks about this contact: %w", err)
		}
		defer rows.Close()
		out = []Request{}
		for rows.Next() {
			var r Request
			if err := scanRequest(rows, &r); err != nil {
				return fmt.Errorf("introductions: reading an ask: %w", err)
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// noteOrigin defaults to human. A note whose origin nobody stated was typed by
// somebody, and claiming a machine wrote it would be a false disclosure in the
// direction that matters least — but claiming a human wrote a model's prose is
// the one that matters, so the caller states it explicitly when it is true.
func noteOrigin(v string) string {
	switch v {
	case "model", "deterministic":
		return v
	default:
		return "human"
	}
}

func fallbackOrNone(v string) string {
	switch v {
	case "name_drop", "next_route":
		return v
	default:
		return "none"
	}
}
