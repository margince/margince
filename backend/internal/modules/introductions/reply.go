// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// The one transition no person can make.
//
// Every other move on an ask is somebody's decision, reached through an
// endpoint and checked against who they are. `replied` is not: it is the
// contact answering, and the product knows it only because a message arrived.
// A checkbox for it would make the workflow's best outcome the one claim
// nobody had to prove, so this file is its whole reachability — reached by the
// capture consumer (compose/introadvance.go) and by nothing else.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordReply marks the contact as having answered, on the evidence of one
// captured activity.
//
// It does NOT go through move, and the three differences are the point.
//
// There is no version to check: nobody read this ask before the message
// arrived, so there is no reader whose view could be stale. What move's version
// check buys — that a second writer cannot land the same transition twice — is
// bought here by the status predicate in the WHERE clause, which matches only
// from the two states a reply can follow.
//
// Zero rows is therefore not a failure. The bus is at-least-once and a thread
// carries many messages, so a second qualifying activity for an ask already
// replied is the ordinary case, not an error: it matches nothing, writes
// nothing, emits nothing, and answers false so the caller stays quiet too.
// Returning an error would wedge the consumer group on a message that will
// never stop being a duplicate.
//
// And the actor is the clock's cousin, not a party: only ActorCapture may make
// this move, so no endpoint can reach it. `replied` is the product's best
// outcome, which is exactly why a person must not be able to assert it.
func (s *Store) RecordReply(
	ctx context.Context, id ids.UUID, activity ids.UUID, at time.Time,
) (bool, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionUpdate); err != nil {
		return false, err
	}
	replied := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// ONE guard, and it is this statement's own WHERE clause.
		//
		// A read-then-check in Go would be a second answer to the same question
		// and a weaker one: two consumers holding two messages from the same
		// thread both pass a read-then-check, and only one passes this. The
		// admissible statuses are spelled in SQL because SQL cannot ask the
		// transition table — TestTheReplyPredicateMatchesTheLifecycle holds the
		// two spellings together instead of hoping they stay aligned.
		//
		// `before` comes out of the CTE, which is the only way to read the
		// status this reply followed in the same statement that replaces it.
		// The audit trail needs it: a handshake answered and a lent name
		// answered are different stories, and the before-image is what tells
		// them apart.
		var before Status
		var personID ids.UUID
		err := tx.QueryRow(ctx, `
			WITH prior AS (
				SELECT id, status FROM intro_request
				 WHERE id = $1 AND archived_at IS NULL
				   AND status IN ('introduced', 'name_dropped')
			)
			UPDATE intro_request r
			   SET status = 'replied', replied_at = $2,
			       source_activity_id = COALESCE(r.source_activity_id, $3),
			       version = r.version + 1, updated_at = now()
			  FROM prior
			 WHERE r.id = prior.id
			 RETURNING prior.status, r.person_id`, id, at, activity).Scan(&before, &personID)
		// No row is the ordinary outcome rather than a fault. The ask is already
		// replied, was never introduced, or is archived — each a message that
		// answers nothing, which the caller reads off the false. Erroring here
		// would wedge the consumer group on traffic that will never stop
		// arriving.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("introductions: recording the reply: %w", err)
		}
		auditID, auditErr := storekit.Audit(ctx, tx, "update", "intro_request", id,
			map[string]any{auditedField: string(before)},
			map[string]any{auditedField: string(StatusReplied)})
		if auditErr != nil {
			return auditErr
		}
		evidence := openapi_types.UUID(activity)
		if emitErr := storekit.EmitEvent(ctx, tx, auditID, personID,
			crmcontracts.PublicEventIntroRequestReplied{
				IntroRequestId:   openapi_types.UUID(id),
				PersonId:         openapi_types.UUID(personID),
				SourceActivityId: &evidence,
			}); emitErr != nil {
			return emitErr
		}
		replied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return replied, nil
}

// AwaitingReply lists the asks one contact has open that a message from them
// would answer: introduced or name-dropped, and not yet replied.
//
// `since` is the instant the handshake happened, returned so the caller can
// refuse a message that predates it. A mail sent last Tuesday is not an answer
// to an introduction made this morning, and counting it would let any old
// thread close an ask nobody has actually answered.
//
// The system principal reads here. It is unbounded by design, which is the
// only way a consumer can see an ask between two colleagues it is not party
// to — and the reason this method takes no filter a caller could widen.
func (s *Store) AwaitingReply(ctx context.Context, personID ids.UUID) ([]Pending, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []Pending
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			-- Exactly one of the two is set: introduced_at on the handshake
			-- path, name_dropped_at on the lent-name one, and the status
			-- filter below admits only those two. So the COALESCE picks the
			-- one that exists rather than choosing between two candidates.
			SELECT id, COALESCE(introduced_at, name_dropped_at)
			  FROM intro_request
			 WHERE person_id = $1 AND archived_at IS NULL
			   AND status IN ('introduced', 'name_dropped')`, personID)
		if err != nil {
			return fmt.Errorf("introductions: listing the asks awaiting a reply: %w", err)
		}
		defer rows.Close()
		out = []Pending{}
		for rows.Next() {
			var p Pending
			if err := rows.Scan(&p.ID, &p.Since); err != nil {
				return fmt.Errorf("introductions: reading an ask awaiting a reply: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Pending is one ask a reply could answer, and the instant it started waiting.
type Pending struct {
	ID    ids.UUID
	Since time.Time
}
