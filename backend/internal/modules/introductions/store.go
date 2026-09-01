// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	ID              ids.UUID
	PersonID        ids.UUID
	RequesterUserID ids.UUID
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
	Status          Status
	DecisionReason  *string
	SuggestedUserID *ids.UUID
	DueAt           time.Time
	RequestedAt     time.Time
	DecidedAt       *time.Time
	IntroducedAt    *time.Time
	NameDroppedAt   *time.Time
	RepliedAt       *time.Time
	Version         int
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
	return s.move(ctx, id, answer, version, func(cur *Request) error {
		return May(cur.Status, answer, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) error {
		_, err := tx.Exec(ctx, `
			UPDATE intro_request
			   SET status = $2, decision_reason = $3, suggested_user_id = $4,
			       decided_at = $5, version = version + 1, updated_at = now(),
			       closed_at = CASE WHEN $2 IN ('declined','suggest_other') THEN $5 ELSE closed_at END
			 WHERE id = $1 AND version = $6`,
			id, string(answer), nullIfEmpty(reason), suggested, s.now(), version)
		return err
	}, func(ctx context.Context, tx pgx.Tx, auditID ids.UUID, cur *Request) error {
		return storekit.EmitEvent(ctx, tx, auditID, cur.PersonID,
			crmcontracts.PublicEventIntroRequestDecided{
				IntroRequestId:   openapi_types.UUID(id),
				PersonId:         openapi_types.UUID(cur.PersonID),
				IntroducerUserId: openapi_types.UUID(cur.IntroducerUser),
				Decision:         crmcontracts.PublicEventIntroRequestDecidedDecision(answer),
			})
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
	return s.move(ctx, id, StatusIntroduced, version, func(cur *Request) error {
		outcome = StatusIntroduced
		if cur.Status == StatusNameDropApproved {
			outcome = StatusNameDropped
		}
		return May(cur.Status, outcome, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) error {
		column := "introduced_at"
		if outcome == StatusNameDropped {
			column = "name_dropped_at"
		}
		// The column name is a compile-time literal chosen from two, never a
		// value off a request: a formatted identifier from a body is how a
		// write becomes an injection.
		_, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE intro_request
			   SET status = $2, %s = $3, source_activity_id = COALESCE($4, source_activity_id),
			       version = version + 1, updated_at = now()
			 WHERE id = $1 AND version = $5`, column),
			id, string(outcome), s.now(), activity, version)
		return err
	}, func(ctx context.Context, tx pgx.Tx, auditID ids.UUID, cur *Request) error {
		return storekit.EmitEvent(ctx, tx, auditID, cur.PersonID,
			crmcontracts.PublicEventIntroRequestCompleted{
				IntroRequestId: openapi_types.UUID(id),
				PersonId:       openapi_types.UUID(cur.PersonID),
				Outcome:        crmcontracts.PublicEventIntroRequestCompletedOutcome(outcome),
			})
	})
}

// Cancel withdraws the ask.
func (s *Store) Cancel(ctx context.Context, id ids.UUID, reason string, version int) error {
	return s.move(ctx, id, StatusCancelled, version, func(cur *Request) error {
		return May(cur.Status, StatusCancelled, s.roleOf(ctx, cur))
	}, func(ctx context.Context, tx pgx.Tx, cur *Request) error {
		_, err := tx.Exec(ctx, `
			UPDATE intro_request
			   SET status = 'cancelled', decision_reason = $2, closed_at = $3,
			       version = version + 1, updated_at = now()
			 WHERE id = $1 AND version = $4`,
			id, nullIfEmpty(reason), s.now(), version)
		return err
	}, func(ctx context.Context, tx pgx.Tx, auditID ids.UUID, cur *Request) error {
		return storekit.EmitEvent(ctx, tx, auditID, cur.PersonID,
			crmcontracts.PublicEventIntroRequestClosed{
				IntroRequestId: openapi_types.UUID(id),
				PersonId:       openapi_types.UUID(cur.PersonID),
				Reason:         crmcontracts.IntroRequestClosedCancelled,
			})
	})
}

// move is the one write path every transition takes.
//
// Read the row, ask the lifecycle whether this actor may make this move, write
// it under the caller's version, then audit and emit — all in one transaction,
// so an ask cannot change state without the trail that explains it.
func (s *Store) move(
	ctx context.Context,
	id ids.UUID,
	to Status,
	version int,
	permit func(*Request) error,
	write func(context.Context, pgx.Tx, *Request) error,
	emit func(context.Context, pgx.Tx, ids.UUID, *Request) error,
) error {
	if err := auth.Require(ctx, "introduction", principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := s.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := permit(cur); err != nil {
			return err
		}
		if cur.Version != version {
			return fmt.Errorf(
				"introductions: this ask moved since you read it: %w", apperrors.ErrVersionSkew)
		}
		if err := write(ctx, tx, cur); err != nil {
			return fmt.Errorf("introductions: recording the answer: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "update", "intro_request", id,
			map[string]any{"status": string(to)})
		if err != nil {
			return err
		}
		return emit(ctx, tx, auditID, cur)
	})
}

// load reads one ask, refusing it to anybody it is not about.
//
// A stranger gets ErrNotFound rather than a refusal: telling them the ask
// exists but is not theirs discloses that this colleague was asked about this
// contact, which is the fact the row is for.
func (s *Store) load(ctx context.Context, tx pgx.Tx, id ids.UUID) (*Request, error) {
	var r Request
	err := tx.QueryRow(ctx, `
		SELECT id, person_id, requester_user_id, introducer_user_id, route_type,
		       through_person_id, internal_reason, value_for_target, forwardable_note,
		       note_generated_by, note_ai_generated, fallback_policy, name_drop_allowed,
		       status, decision_reason, suggested_user_id, due_at, requested_at,
		       decided_at, introduced_at, name_dropped_at, replied_at, version
		  FROM intro_request WHERE id = $1 AND archived_at IS NULL`, id).Scan(
		&r.ID, &r.PersonID, &r.RequesterUserID, &r.IntroducerUser, &r.RouteType,
		&r.ThroughPersonID, &r.InternalReason, &r.ValueForTarget, &r.ForwardableNote,
		&r.NoteGeneratedBy, &r.NoteAIGenerated, &r.FallbackPolicy, &r.NameDropAllowed,
		&r.Status, &r.DecisionReason, &r.SuggestedUserID, &r.DueAt, &r.RequestedAt,
		&r.DecidedAt, &r.IntroducedAt, &r.NameDroppedAt, &r.RepliedAt, &r.Version)
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
		rows, err := tx.Query(ctx, `
			SELECT id, person_id, requester_user_id, introducer_user_id, route_type,
			       through_person_id, status, due_at, requested_at, version
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
			if err := rows.Scan(&r.ID, &r.PersonID, &r.RequesterUserID, &r.IntroducerUser,
				&r.RouteType, &r.ThroughPersonID, &r.Status, &r.DueAt, &r.RequestedAt,
				&r.Version); err != nil {
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
