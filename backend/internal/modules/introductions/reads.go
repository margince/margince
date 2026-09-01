// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// Reading an ask, and reading the asks about one contact.
//
// The refusal is the same on both paths and it is a NOT-FOUND rather than a
// denial: telling a stranger that the ask exists but is not theirs discloses
// that this colleague was asked about this contact, which is the fact the row
// exists to keep.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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

// AwaitingMyAnswer lists the asks waiting on the CALLER to answer, the ones
// closest to lapsing first.
//
// Bound to the acting person and to no other, exactly as a notice is: an ask
// names one colleague, and there is no wider scope for it to widen to. A
// manager asking for `team` does not reach a colleague's asks, because the
// question "whose favour was somebody asked for" is not shared record-bearing
// work — it is between the two of them until one of them answers.
//
// Ordered by due date rather than by age, because the one about to expire is
// the one a colleague most needs to see: a lapsed ask reads to the requester
// exactly like a refusal, and the difference is whether anybody looked.
func (s *Store) AwaitingMyAnswer(ctx context.Context, limit int) ([]Request, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// No human, no queue. A caller with no person behind it has nobody
		// whose favour was asked for, and answering with somebody else's asks
		// would be handing an agent a colleague's inbox.
		return nil, apperrors.ErrPermissionDenied
	}
	var out []Request
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, requestColumns+`
			  FROM intro_request
			 WHERE introducer_user_id = $1 AND archived_at IS NULL
			   AND status = 'requested'
			 ORDER BY due_at, id
			 LIMIT $2`, actor.UserID, limit)
		if err != nil {
			return fmt.Errorf("introductions: listing the asks waiting on you: %w", err)
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
