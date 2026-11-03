// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// deliveryPageMax and deliveryPageDefault bound one page of the history.
const (
	deliveryPageMax     = 200
	deliveryPageDefault = 50
)

// The webhook_delivery status vocabulary lives in the table's CHECK and in
// the SQL below: 'pending' (freshly enqueued) → 'retrying' (failed, with a
// backoff deadline) → 'dead_lettered' (budget spent), or → 'delivered'.
// 'visibility_revoked' is the fifth and is terminal: the subject record left
// the owner's sight between enqueue and the re-attempt.
const deliveryColumns = `id, subscription_id, event_id, event_type, status, attempts,
	last_status_code, last_error, next_retry_at, delivered_at, dead_lettered_at, created_at, updated_at`

// Delivery is the inspectable view of one attempt log (B-E10.13c). The
// signed body is not exposed — it is an internal detail of replay.
type Delivery struct {
	ID             ids.UUID
	SubscriptionID ids.UUID
	EventID        ids.UUID
	EventType      string
	Status         string
	Attempts       int
	LastStatusCode *int
	LastError      *string
	NextRetryAt    *time.Time
	DeliveredAt    *time.Time
	DeadLetteredAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func scanDelivery(r pgx.Row) (Delivery, error) {
	var d Delivery
	err := r.Scan(&d.ID, &d.SubscriptionID, &d.EventID, &d.EventType, &d.Status, &d.Attempts,
		&d.LastStatusCode, &d.LastError, &d.NextRetryAt, &d.DeliveredAt, &d.DeadLetteredAt,
		&d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// deliveryCursor is where a page of the history stopped: both parts of its
// order, because the order has two.
//
// `created_at` ALONE IS NOT A POSITION. A subscription that fans out on one
// event writes its attempts in the same instant, and a retry storm makes that
// ordinary rather than rare — so the id is the tie-break, exactly as the
// ORDER BY has it. Resuming on the timestamp alone would re-serve or skip every
// attempt sharing the boundary second.
type deliveryCursor struct {
	CreatedAt time.Time `json:"at"`
	ID        ids.UUID  `json:"id"`
}

// ListDeliveries returns a subscription's delivery history newest-first —
// the dead-letter inspection surface (B-E10.13c). Read-gated, and the
// subscription is existence-hidden if the caller may not see it.
//
// PAGED, because hasMore alone made the tail unreachable. The view was honest
// that it had been cut and offered no way to ask for the rest: past the limit,
// an operator inspecting why a subscription is failing saw the newest attempts
// and could not reach the parked ones behind them — which are the ones they
// opened the page for.
func (s *Store) ListDeliveries(
	ctx context.Context, subID ids.UUID, limit int, cursor string,
) ([]Delivery, storekit.Page, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	if _, err := s.GetSubscription(ctx, subID); err != nil {
		return nil, storekit.Page{}, err
	}
	if limit <= 0 || limit > deliveryPageMax {
		limit = deliveryPageDefault
	}
	var out []Delivery
	var page storekit.Page
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{subID}
		arg := func(v any) int { args = append(args, v); return len(args) }
		where, err := deliveryPageClause(cursor, arg)
		if err != nil {
			return err
		}
		// Fetch one past the page so a full page is distinguishable from a
		// truncated one without a second count query.
		query := "SELECT " + deliveryColumns +
			" FROM webhook_delivery WHERE subscription_id = $1" + where +
			" ORDER BY created_at DESC, id DESC LIMIT " + storekit.SQLf("$%d", arg(limit+1))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDelivery(rows)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) <= limit {
			return nil
		}
		out = out[:limit]
		last := out[limit-1]
		token, err := storekit.EncodeOpaque(deliveryCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return err
		}
		page = storekit.Page{HasMore: true, NextCursor: token}
		return nil
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return out, page, nil
}

// deliveryPageClause is the keyset arm, spelled as a row comparison rather than
// an OR of two: it is the order the index reads, and one fewer place to get a
// tie-break wrong. `<` because the walk runs newest-first.
func deliveryPageClause(cursor string, arg func(any) int) (string, error) {
	if cursor == "" {
		return "", nil
	}
	after, err := storekit.DecodeOpaque[deliveryCursor](cursor)
	// BOTH parts, not the id alone. The envelope proves the token is one of
	// ours, not that it names a position in THIS history: a cursor from any
	// other paged route spells `id` the same way and decodes cleanly here,
	// leaving the instant at its zero value — which, on a DESC walk, pages from
	// before every row and answers an empty page. An operator would read that
	// as "no more attempts" on the surface they opened precisely because
	// deliveries were failing.
	if err != nil || after.CreatedAt.IsZero() || after.ID.IsZero() {
		return "", &storekit.MalformedCursorError{}
	}
	return storekit.SQLf(" AND (created_at, id) < ($%d, $%d)",
		arg(after.CreatedAt), arg(after.ID)), nil
}

// getDelivery reads one delivery by id in the caller's workspace.
func (s *Store) getDelivery(ctx context.Context, deliveryID ids.UUID) (Delivery, error) {
	var out Delivery
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanDelivery(tx.QueryRow(ctx,
			"SELECT "+deliveryColumns+" FROM webhook_delivery WHERE id = $1", deliveryID))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, apperrors.ErrNotFound
	}
	return out, err
}

// requireReplay authorizes a replay: the caller must hold update on the
// config surface, the subscription must be visible (existence-hiding), the
// delivery must belong to it, and the action is audited to the acting
// human before the re-attempt runs.
func (s *Store) requireReplay(ctx context.Context, subID, deliveryID ids.UUID) error {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return err
	}
	if _, err := s.GetSubscription(ctx, subID); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := checkReplayable(ctx, tx, subID, deliveryID); err != nil {
			return err
		}
		_, err := storekit.AuditEvent(ctx, tx, "update", rbacObject, subID,
			map[string]any{"replayed_delivery": deliveryID.String()})
		return err
	})
}

// attemptTarget is one deliverable unit: the sealed secret and body the
// signer needs, plus the identity to record the outcome against.
type attemptTarget struct {
	deliveryID   ids.UUID
	subID        ids.UUID
	targetURL    string
	sealedSecret string
	eventType    string
	eventID      ids.UUID
	payload      []byte
	contentType  string
	// claim is the lease this sender holds, nil for a first attempt; the
	// outcome lands only while the row still carries it.
	claim         *time.Time
	headers       []storedHeader
	priorAttempts int
	// entityType, entityID and ownerID are what a RE-attempt needs and a first
	// attempt does not: the enqueue path has already asked whether this owner
	// may see this record, and a retry has to ask again because the answer can
	// have changed. entityType is empty on a row written before those columns
	// existed, and an empty subject is refused rather than sent.
	entityType string
	entityID   ids.UUID
	ownerID    ids.UUID
}

// subCandidate is one active subscription matching an event's type, with
// the owning principal the fan-out is bounded to (B-E10.15/BYO-EVT-4).
type subCandidate struct {
	id              ids.UUID
	ownerID         ids.UUID
	bodyTemplate    *string
	bodyContentType *string
}

// matchingSubscriptions returns the active subscriptions in the envelope's
// workspace whose event_types include this type, each with its owner —
// the fan-out candidate set BEFORE the owner-visibility filter. Runs in
// the envelope's workspace under the tenant GUC.
func (s *Store) matchingSubscriptions(ctx context.Context, eventType string) ([]subCandidate, error) {
	var out []subCandidate
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, owner_id, body_template, body_content_type FROM webhook_subscription
			WHERE state = 'active' AND archived_at IS NULL
			  AND event_types @> ARRAY[$1]::text[]`, eventType)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c subCandidate
			if err := rows.Scan(&c.id, &c.ownerID, &c.bodyTemplate, &c.bodyContentType); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// claimDueRetry claims the next due retrying delivery in the ctx's workspace
// whose subscription is live and active. The claim is a lease on claimed_until
// that a second sweep or a replay respects. The returned instant is the token
// the sender records its outcome under. Runs under the tenant GUC.
func (s *Store) claimDueRetry(ctx context.Context, now time.Time) (ids.UUID, time.Time, bool, error) {
	var id ids.UUID
	var claim time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE webhook_delivery SET claimed_until = now() + make_interval(secs => $2)
			WHERE id = (
				SELECT d.id
				FROM webhook_delivery d
				JOIN webhook_subscription s ON s.id = d.subscription_id
				WHERE d.status = 'retrying' AND d.next_retry_at <= $1
				  AND (d.claimed_until IS NULL OR d.claimed_until < now())
				  AND s.state = 'active' AND s.archived_at IS NULL
				ORDER BY d.next_retry_at
				LIMIT 1
				FOR UPDATE OF d SKIP LOCKED)
			RETURNING id, claimed_until`, now, claimLease.Seconds()).Scan(&id, &claim)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, time.Time{}, false, nil
	}
	return id, claim, err == nil, err
}

// loadTarget rehydrates a delivery into an attemptTarget for retry/replay:
// the stored body plus the subscription's current target URL and sealed
// secret (so a rotation between attempts takes effect).
func (s *Store) loadTarget(ctx context.Context, deliveryID ids.UUID) (attemptTarget, error) {
	var t attemptTarget
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var entityType, contentType *string
		var entityID *ids.UUID
		if err := tx.QueryRow(ctx, `
			SELECT d.id, d.subscription_id, s.target_url, s.signing_secret_ref,
			       d.event_type, d.event_id, d.payload, d.content_type, s.custom_headers,
			       d.attempts, d.entity_type, d.entity_id, s.owner_id
			FROM webhook_delivery d
			JOIN webhook_subscription s ON s.id = d.subscription_id
			WHERE d.id = $1`, deliveryID).
			Scan(&t.deliveryID, &t.subID, &t.targetURL, &t.sealedSecret,
				&t.eventType, &t.eventID, &t.payload, &contentType, &t.headers,
				&t.priorAttempts, &entityType, &entityID, &t.ownerID); err != nil {
			return err
		}
		t.contentType = deref(contentType)
		// A NULL subject scans to a nil pointer and stays the zero value, which
		// refuseUnverifiable reads as "cannot be checked". Widening it to an
		// empty string here rather than at the call site keeps the one meaning
		// in one place.
		if entityType != nil {
			t.entityType = *entityType
		}
		if entityID != nil {
			t.entityID = *entityID
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return attemptTarget{}, apperrors.ErrNotFound
	}
	return t, err
}

// outcome is the result of one HTTP attempt, translated to the next row
// state by recordOutcome.
type outcome struct {
	statusCode int    // 0 when the request never got a response (dial/timeout)
	failure    string // empty on success
}

// recordOutcome advances the delivery state machine in the target's
// workspace: success → delivered; failure with budget left → retrying
// with the next backoff deadline; budget spent → dead_lettered. Timestamps
// come from the injected clock so the schedule is testable.
func (s *Store) recordOutcome(ctx context.Context, t attemptTarget, res outcome, now time.Time) error {
	attempts := t.priorAttempts + 1
	var statusCode *int
	if res.statusCode != 0 {
		statusCode = &res.statusCode
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if res.failure == "" {
			_, err := tx.Exec(ctx, `
				UPDATE webhook_delivery
				SET status = 'delivered', attempts = $2, last_status_code = $3,
				    last_error = NULL, next_retry_at = NULL, delivered_at = $4, claimed_until = NULL
				WHERE id = $1 AND status <> 'visibility_revoked' AND claimed_until IS NOT DISTINCT FROM $5`,
				t.deliveryID, attempts, statusCode, now, t.claim)
			return err
		}
		if attempts >= maxAttempts {
			_, err := tx.Exec(ctx, `
				UPDATE webhook_delivery
				SET status = 'dead_lettered', attempts = $2, last_status_code = $3,
				    last_error = $4, next_retry_at = NULL, dead_lettered_at = $5, claimed_until = NULL
				WHERE id = $1 AND status <> 'visibility_revoked' AND claimed_until IS NOT DISTINCT FROM $6`,
				t.deliveryID, attempts, statusCode, res.failure, now, t.claim)
			return err
		}
		next := now.Add(backoff(attempts))
		_, err := tx.Exec(ctx, `
			UPDATE webhook_delivery
			SET status = 'retrying', attempts = $2, last_status_code = $3,
			    last_error = $4, next_retry_at = $5, claimed_until = NULL
			WHERE id = $1 AND status <> 'visibility_revoked' AND claimed_until IS NOT DISTINCT FROM $6`,
			t.deliveryID, attempts, statusCode, res.failure, next, t.claim)
		return err
	})
}

// markVisibilityRevoked parks a delivery whose subject the owner may no longer
// see. Terminal, and deliberately not dead_lettered: dead_lettered is the store
// an operator replays FROM, and a revoked delivery must not be in it. The reason
// goes in last_error for the operator, and stays there — resetForReplay clears
// that column, so a status that shared the replay path would destroy it.
//
// Every writer of this table carries `status <> 'visibility_revoked'`, and that
// clause is DEFENCE rather than a reachable race. Three things already stop a
// revoked row being written over. The visibility check refuses it again, this
// method clears next_retry_at, and claimDueRetry claims only 'retrying'.
// Removing any one of them leaves the behaviour unchanged, so no test drives
// this clause.
//
// It earns its line on the one case none of those three covers: a sweep already
// mid-POST when a revocation commits, whose recordOutcome would otherwise land
// 'delivered' on top. And it means the next caller added to this store inherits
// the invariant rather than having to know it.
func (s *Store) markVisibilityRevoked(ctx context.Context, deliveryID ids.UUID, reason string) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE webhook_delivery
			SET status = 'visibility_revoked', last_error = $2, next_retry_at = NULL, claimed_until = NULL
			WHERE id = $1 AND status <> 'visibility_revoked'`, deliveryID, reason)
		return err
	})
}

// resetForReplay claims a parked delivery and clears it back to pending. A
// revoked row is never reset (ErrNotFound), which stops a concurrent revocation
// being undone. A row another sender holds is errDeliveryInFlight.
func (s *Store) resetForReplay(ctx context.Context, deliveryID ids.UUID) (time.Time, error) {
	var claim time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE webhook_delivery
			SET status = 'pending', next_retry_at = NULL, dead_lettered_at = NULL, last_error = NULL,
			    claimed_until = now() + make_interval(secs => $2)
			WHERE id = $1 AND status IN ('dead_lettered', 'retrying')
			  AND (claimed_until IS NULL OR claimed_until < now())
			RETURNING claimed_until`, deliveryID, claimLease.Seconds()).Scan(&claim)
		if err == nil || !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// A revoked row is never reset; anything else lost the claim to a sender.
		var revoked bool
		if err := tx.QueryRow(ctx, "SELECT status = 'visibility_revoked' FROM webhook_delivery WHERE id = $1",
			deliveryID).Scan(&revoked); err != nil {
			return err
		}
		if revoked {
			return apperrors.ErrNotFound
		}
		return errDeliveryInFlight
	})
	return claim, err
}
