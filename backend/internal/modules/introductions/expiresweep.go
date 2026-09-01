// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// An ask nobody answered runs out of time.
//
// `due_at` was written on every row and read by nobody, so an ask a colleague
// never got to stayed open forever. Two things follow from that, and the second
// is the one that bites:
//
// The requester is never told. A colleague's silence is an answer — the rep has
// to stop waiting and try another route — and a queue that never closes an
// unanswered ask leaves them waiting on somebody who has already moved on.
//
// And the route stays blocked. `intro_request_open_route` is a partial unique
// index over the open statuses, so an ask that can never leave one holds that
// (contact, colleague) pair permanently: the duplicate guard, which exists to
// stop two tabs racing, becomes a permanent refusal instead.
//
// Expiry reaches every state where somebody still owes an action, not only the
// first. An accepted ask nobody completed is exactly the request a queue loses
// quietly — the colleague said yes, nothing happened, and the record still
// reads as a promise being kept.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ExpiryActor names the clock on every audit row this sweep writes.
//
// A system id rather than a person, and the reason is legibility rather than
// ceremony: somebody reading the trail must be able to tell an ask a colleague
// declined from one nobody ever answered. Those are different facts about that
// colleague, and only the actor tells them apart.
const ExpiryActor = "system:introduction-expiry"

// expirySweepBatch bounds one pass. A backlog drains across ticks rather than
// in one transaction: each expiry is its own audited decision, and holding
// thousands open would make the sweep a lock every ask waits behind.
const expirySweepBatch = 200

// ExpireDue closes every ask whose due date has passed, and reports how many.
//
// One transaction per row, deliberately. An expiry is a decision, and a batch
// that half-committed would leave some rows audited and others not — the split
// the write shape exists to prevent. A row that fails is skipped and the next
// tick sees it again, because the predicate is the clock and the clock does not
// move backwards.
//
// The CLOCK may call this and nobody else. Every other entry point on this
// store gates on which party the caller is; this one closes asks in bulk
// without consulting anybody's row scope. Left open it would be an
// authenticated user's way to cancel every open introduction in the
// installation at once — each one audited as though the clock had done it, with
// their name nowhere in the record.
func (s *Store) ExpireDue(ctx context.Context) (int, error) {
	if err := onlyTheExpirySweep(ctx); err != nil {
		return 0, err
	}
	due, err := s.dueForExpiry(ctx)
	if err != nil {
		return 0, err
	}
	// One bad row must not starve the rest. Candidates come back oldest-first
	// and the batch is capped, so returning on the first failure would let a
	// single permanently-failing ask occupy the front of every batch on every
	// tick — and nothing behind it would ever expire again.
	expired := 0
	var failures []error
	for _, id := range due {
		ok, err := s.expireOne(ctx, id)
		if err != nil {
			failures = append(failures, fmt.Errorf("intro request %s: %w", id, err))
			continue
		}
		if ok {
			expired++
		}
	}
	return expired, errors.Join(failures...)
}

// dueForExpiry reads the candidates, oldest deadline first.
//
// A plain read outside any lock: each row is re-checked under its own lock
// before being written, so a candidate somebody answers between the two is
// simply skipped.
//
// `<` and not `<=`, which is the tree's one answer to "is a deadline at this
// exact instant already late". It is not: deadline.Passed says so, every other
// surface follows it, and a row counted late here while a card called it
// upcoming would be the same ask answering two ways depending on which surface
// assembled it.
//
// The open statuses are spelled here and in the partial unique index and in
// Open(). TestOpenMatchesTheDuplicateGuardIndex holds the first two together;
// this query is checked against Open() by TestTheSweepReachesEveryOpenStatus,
// because a status added to the lifecycle and forgotten here is an ask that can
// never expire — which is the exact defect this file was written to fix.
func (s *Store) dueForExpiry(ctx context.Context) ([]ids.UUID, error) {
	var due []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id
			  FROM intro_request
			 WHERE archived_at IS NULL
			   AND status IN ('requested', 'accepted', 'name_drop_approved')
			   AND due_at < $1
			 ORDER BY due_at
			 LIMIT $2`, s.now().UTC(), expirySweepBatch)
		if err != nil {
			return fmt.Errorf("introductions: reading the asks whose time ran out: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			due = append(due, id)
		}
		return rows.Err()
	})
	return due, err
}

// expireOne closes one ask in the write shape, and reports whether it did.
//
// false with no error is the ordinary outcome for a row somebody answered
// between the read and this write: the UPDATE's own status predicate matches
// nothing, and the ask is left exactly as its answerer left it. The clock lost
// that race and the human won it, which is the right way round.
func (s *Store) expireOne(ctx context.Context, id ids.UUID) (bool, error) {
	swept := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// ONE guard, the same shape RecordReply uses and for the same reason:
		// the CTE locks the row and the UPDATE re-checks it, so a decision
		// committing while this transaction waited cannot be overwritten. The
		// before-image comes out of the CTE, because the trail has to say which
		// state ran out of time — an ask nobody answered and one a colleague
		// accepted and then dropped are different stories about that colleague.
		var before Status
		var personID ids.UUID
		err := tx.QueryRow(ctx, `
			WITH prior AS (
				SELECT id, status FROM intro_request
				 WHERE id = $1 AND archived_at IS NULL
				   AND status IN ('requested', 'accepted', 'name_drop_approved')
				   AND due_at < $2
				 FOR UPDATE
			)
			UPDATE intro_request r
			   SET status = 'expired', closed_at = $2,
			       version = r.version + 1, updated_at = now()
			  FROM prior
			 WHERE r.id = prior.id
			   AND r.status IN ('requested', 'accepted', 'name_drop_approved')
			 RETURNING prior.status, r.person_id`, id, s.now().UTC()).Scan(&before, &personID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("introductions: closing an ask that ran out of time: %w", err)
		}
		// The lifecycle is asked rather than assumed. The query's status list
		// and the transition table are two spellings of one rule, and this is
		// what makes a disagreement fail here instead of writing a move the
		// state machine forbids.
		if err := May(before, StatusExpired, ActorClock); err != nil {
			return fmt.Errorf("introductions: the sweep read %q as expirable: %w", before, err)
		}
		auditID, auditErr := storekit.Audit(ctx, tx, "update", "intro_request", id,
			map[string]any{auditedField: string(before)},
			map[string]any{auditedField: string(StatusExpired)})
		if auditErr != nil {
			return auditErr
		}
		// The same event a cancellation emits, with `reason` telling them
		// apart. A rep withdrawing and a queue timing out call for different
		// follow-ups, and a consumer that could not distinguish them would
		// treat a colleague's silence as the rep's own decision.
		if emitErr := storekit.EmitEvent(ctx, tx, auditID, personID,
			crmcontracts.PublicEventIntroRequestClosed{
				IntroRequestId: openapi_types.UUID(id),
				PersonId:       openapi_types.UUID(personID),
				Reason:         crmcontracts.IntroRequestClosedExpired,
			}); emitErr != nil {
			return emitErr
		}
		swept = true
		return nil
	})
	return swept, err
}

// onlyTheExpirySweep admits the scheduled sweep and refuses everyone else.
//
// It checks the ACTOR ID as well as the principal type, because "some system
// principal" is not the claim being made: the audit rows this pass writes are
// attributed to ExpiryActor, and a caller who cannot present that id would be
// closing introductions under a name that is not theirs.
func onlyTheExpirySweep(ctx context.Context) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf(
			"introductions: the expiry sweep ran with no bound actor: %w",
			apperrors.ErrPermissionDenied)
	}
	if p.Type != principal.PrincipalSystem || p.ID != ExpiryActor {
		return fmt.Errorf(
			"introductions: %s may not expire introductions — the sweep runs as %s: %w",
			p.ID, ExpiryActor, apperrors.ErrPermissionDenied)
	}
	return nil
}
