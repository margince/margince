// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The contract's maxLength for each text field, in characters. A reason is a
// paragraph and a note is a short mail; the colleague deciding is the one who
// pays for a wall of text.
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
	ContactID        ids.UUID
	RequesterUserID  ids.UUID
	IntroducerUser   ids.UUID
	RouteType        string
	ThroughContactID *ids.UUID
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
	ContactID        ids.UUID
	IntroducerUser   ids.UUID
	RouteType        string
	ThroughContactID *ids.UUID
	InternalReason   string
	ValueForTarget   string
	ForwardableNote  string
	NoteGeneratedBy  string
	NoteAIGenerated  bool
	FallbackPolicy   string
	NameDropAllowed  bool
	DueAt            time.Time
}

// Create records one ask.
//
// The requester is the authenticated contact and never a field on the request:
// a body that could name its own requester would let one rep put an ask in
// another's name, and the colleague answering would be answering the wrong
// contact.
func (s *Store) Create(ctx context.Context, req NewRequest) (ids.UUID, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	actor, err := requesterOf(ctx, req)
	if err != nil {
		return ids.UUID{}, err
	}
	// The contact has to be one this rep can actually see. Without it an ask
	// would name a contact the requester cannot open, and the colleague's
	// screen would disclose them.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return ids.UUID{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, err
	}

	// Unstated is a default; stated and unknown is refused.
	if req.NoteGeneratedBy == "" {
		req.NoteGeneratedBy = "human"
	}
	if req.FallbackPolicy == "" {
		req.FallbackPolicy = "none"
	}
	if err := validateAsk(req); err != nil {
		return ids.UUID{}, err
	}

	id := ids.NewV7()
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", req.ContactID); err != nil {
			return err
		}
		// The intermediary is named by the caller too, and naming a record is
		// reading it: without this a rep could learn that a contact exists by
		// routing an ask through them and reading which error came back.
		if req.ThroughContactID != nil {
			if err := auth.EnsureVisibleLive(ctx, tx, "contact", *req.ThroughContactID); err != nil {
				return err
			}
		}
		if err := lockNamedContacts(ctx, tx, req.ContactID, req.ThroughContactID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO intro_request (
				id, contact_id, requester_user_id, introducer_user_id,
				route_type, through_contact_id,
				internal_reason, value_for_target, forwardable_note,
				note_generated_by, note_ai_generated,
				fallback_policy, name_drop_allowed, due_at, captured_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			id, req.ContactID, actor.UserID, req.IntroducerUser,
			req.RouteType, req.ThroughContactID,
			req.InternalReason, req.ValueForTarget, req.ForwardableNote,
			req.NoteGeneratedBy, req.NoteAIGenerated,
			req.FallbackPolicy, req.NameDropAllowed,
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
				"contact_id":         req.ContactID.String(),
				"introducer_user_id": req.IntroducerUser.String(),
				"route_type":         req.RouteType,
			})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, req.ContactID,
			crmcontracts.PublicEventIntroRequestCreated{
				IntroRequestId:   openapi_types.UUID(id),
				ContactId:        openapi_types.UUID(req.ContactID),
				RequesterUserId:  openapi_types.UUID(actor.UserID),
				IntroducerUserId: openapi_types.UUID(req.IntroducerUser),
			})
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return id, nil
}

// requesterOf answers the human making an ask, refusing an ask nobody could
// honestly have made.
func requesterOf(ctx context.Context, req NewRequest) (principal.Principal, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		// Asking a colleague for a favour is a human's act. An agent holding
		// a human's id is not that human deciding to spend their goodwill.
		return principal.Principal{}, fmt.Errorf(
			"introductions: asking for an introduction needs an authenticated contact: %w",
			apperrors.ErrPermissionDenied)
	}
	if req.IntroducerUser == actor.UserID {
		// An introduction has two contacts on our side, and the requester is
		// already one of them. The graph ranks every colleague who corresponds
		// with the contact and the reader is among them, so this is the shape a
		// client sends when it has offered its own reader as the way in — the
		// row it would write puts a favour in the asker's own queue and records
		// a handoff that never happened.
		return principal.Principal{}, fmt.Errorf(
			"introductions: an introduction is asked of somebody else: %w",
			apperrors.ErrInvalidArgument)
	}
	if strings.TrimSpace(req.InternalReason) == "" {
		return principal.Principal{}, &values.ParseError{
			Field: "internal_reason", Code: "required",
			Message: "an ask says why it is worth making; give the colleague a reason",
		}
	}
	return actor, nil
}

// lockNamedContacts holds both contacts an ask names until it commits, so a
// merge retiring either waits for the ask and carries it rather than
// committing first and leaving it on the retired record. In id order, so two
// writers naming the same pair cannot take them in opposite orders.
func lockNamedContacts(ctx context.Context, tx pgx.Tx, about ids.UUID, through *ids.UUID) error {
	named := []ids.UUID{about}
	if through != nil && *through != about {
		named = append(named, *through)
	}
	slices.SortFunc(named, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	for _, id := range named {
		if err := auth.LockSubjectLive(ctx, tx, "contact", id); err != nil {
			return err
		}
	}
	return nil
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
	if err := tooLong("reason", reason, reasonBound); err != nil {
		return err
	}
	if answer == StatusSuggestOther && suggested == nil {
		return &values.ParseError{
			Field: "suggested_user_id", Code: "required",
			Message: "name the colleague to suggest in suggested_user_id",
		}
	}
	return s.move(ctx, id, func() Status { return answer }, version, func(cur *Request) error {
		if err := May(cur.Status, answer, s.roleOf(ctx, cur)); err != nil {
			return err
		}
		// Handing the ask back to either party sends the requester to someone
		// they have already dealt with.
		if suggested != nil && (*suggested == cur.IntroducerUser || *suggested == cur.RequesterUserID) {
			return &values.ParseError{
				Field: "suggested_user_id", Code: "invalid",
				Message: "suggest a colleague other than the one answering or the one asking",
			}
		}
		return nil
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
			ContactId:        openapi_types.UUID(cur.ContactID),
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
					 WHERE activity_id = $1 AND entity_type = 'contact' AND contact_id = $2)`,
				*activity, cur.ContactID).Scan(&linked); err != nil {
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
			ContactId:      openapi_types.UUID(cur.ContactID),
			Outcome:        crmcontracts.PublicEventIntroRequestCompletedOutcome(outcome),
		}
	})
}

// Cancel withdraws the ask.
func (s *Store) Cancel(ctx context.Context, id ids.UUID, reason string, version int) error {
	if err := tooLong("reason", reason, reasonBound); err != nil {
		return err
	}
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
			ContactId:      openapi_types.UUID(cur.ContactID),
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
		// The before-image is the status this ask was in, which is exactly what
		// a dispute about an introduction asks: an accepted ask that ended up
		// declined and one that was declined outright are different stories,
		// and only the pair tells them apart.
		auditID, err := storekit.Audit(ctx, tx, "update", "intro_request", id,
			map[string]any{auditedField: string(cur.Status)},
			map[string]any{auditedField: string(to())})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, cur.ContactID, event(cur))
	})
}

// tooLong refuses text past the contract's limit. It counts runes because
// the contract and Postgres both count characters: a byte count refuses text
// they accept, and cutting at a byte can split a character into invalid UTF-8.
func tooLong(field, text string, limit int) error {
	if utf8.RuneCountInString(text) <= limit {
		return nil
	}
	return &values.ParseError{
		Field: field, Code: "too_long",
		Message: fmt.Sprintf("%s is at most %d characters; shorten it", field, limit),
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// validateAsk refuses what the contract does not admit. The enums are
// refused rather than defaulted: an unknown note origin recorded as `human`
// would disclose model prose as typed.
func validateAsk(req NewRequest) error {
	if req.InternalReason == "" {
		return &values.ParseError{Field: "internal_reason", Code: "required", Message: "internal_reason is required"}
	}
	if err := tooLong("internal_reason", req.InternalReason, reasonBound); err != nil {
		return err
	}
	if err := tooLong("value_for_target", req.ValueForTarget, reasonBound); err != nil {
		return err
	}
	if err := tooLong("forwardable_note", req.ForwardableNote, noteBound); err != nil {
		return err
	}
	if !crmcontracts.IntroNoteOrigin(req.NoteGeneratedBy).Valid() {
		return notAllowed("note_generated_by", "human, model or deterministic")
	}
	if !crmcontracts.IntroFallbackPolicy(req.FallbackPolicy).Valid() {
		return notAllowed("fallback_policy", "none, name_drop or next_route")
	}
	return nil
}

func notAllowed(field, allowed string) error {
	return &values.ParseError{
		Field: field, Code: "value_not_allowed",
		Message: fmt.Sprintf("%s must be %s", field, allowed),
	}
}
