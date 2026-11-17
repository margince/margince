// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A reply's lease and life: a pulled reply is offered again after replyLease,
// and one nobody acked within replyLifetime expires.
const (
	replyLease    = 5 * time.Minute
	replyLifetime = 7 * 24 * time.Hour
	maxPull       = 50
)

// A reply's states, as personal_channel_reply_state_check lists them.
const (
	stateWaiting = "waiting"
	statePulled  = "pulled"
	stateFailed  = "failed"
	stateExpired = "expired"
)

// What a client may say about a reply it pulled.
const (
	OutcomeSent   = "sent"
	OutcomeFailed = "failed"
)

// What an ack answers.
const (
	AckRecorded        = "recorded"
	AckUnknownDelivery = "unknown_delivery"
	AckAlreadySettled  = "already_settled"
	AckSuperseded      = "superseded"
)

const (
	replyEntity   = "personal_channel_reply"
	messageEntity = "personal_channel_message"
)

// auditState is the audited field a reply's state changes in.
const auditState = "state"

// notDeliveredByClient is the reason a failed ack that named none is given.
const notDeliveredByClient = "the client could not send it"

// notDeliveredByExpiry is the reason an expired reply is given.
var notDeliveredByExpiry = fmt.Sprintf("no client picked the reply up within %d days", int(replyLifetime/(24*time.Hour)))

// errNoSettler is a wiring fault: compose always supplies a settler.
var errNoSettler = errors.New("personal channels: no settler is composed")

// ReplyInput is one reply a rep wrote, as the send path queues it.
type ReplyInput struct {
	Connection       ids.UUID
	ActivityID       ids.UUID
	DeliveryKey      string
	Network, Account string
	GroupID, ReplyTo string
	Body             string
}

// Reply is one queued reply as a client pulls it.
type Reply struct {
	DeliveryID       ids.UUID
	Network, Account string
	GroupID, ReplyTo string
	Body             string
	Attempt          int
}

// PullAnswer is the replies handed out, and the connection's state so a client
// holding only personal_channels can show it.
type PullAnswer struct {
	Replies              []Reply
	Enabled              bool
	RepliesWaiting       int
	OldestReplyWaitingAt *time.Time
}

// Ack is a client's word on one reply it pulled.
type Ack struct {
	DeliveryID ids.UUID
	// Attempt is the attempt the pull handed out; zero means the client did not say.
	Attempt           int
	Outcome           string
	ProviderMessageID string
	Reason            string
}

// AckResult answers one Ack.
type AckResult struct {
	DeliveryID ids.UUID
	Result     string
}

// Settler marks a reply's delivery as not delivered on the timeline, in the
// transaction that settled the reply. compose supplies it, because the delivery
// belongs to comms.
type Settler interface {
	NotDelivered(ctx context.Context, tx pgx.Tx, activity ids.UUID, reason string) error
}

// WithSettler returns a copy of the store that settles failed and expired replies through t.
func (s *Store) WithSettler(t Settler) *Store {
	out := *s
	out.settler = t
	return &out
}

// ErrReplyClosed answers a send retried after its reply was already settled as
// not delivered, so the timeline is not told it was sent.
var ErrReplyClosed = errors.New("personal channels: this reply was already settled as not delivered")

// QueueReply keeps one reply for the member's client to pull. It runs in the
// send path, as the rep whose connection carries the thread. A retry of a reply
// already queued keeps it. A retry whose reply failed or expired meanwhile is
// refused, because the delivery the caller is about to mark sent never arrived.
func QueueReply(ctx context.Context, tx pgx.Tx, in ReplyInput) error {
	var queued ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO personal_channel_reply (connection_id, activity_id, delivery_key, network, thread_id,
			recipient_account, reply_to, body)
		VALUES ($1, $2, $3, $4, $5, $6, nullif($7, ''), $8)
		ON CONFLICT (delivery_key) DO NOTHING
		RETURNING id`,
		in.Connection, in.ActivityID, in.DeliveryKey, in.Network, in.GroupID, in.Account, in.ReplyTo, in.Body).Scan(&queued)
	if err == nil {
		_, err = storekit.Audit(ctx, tx, "create", replyEntity, queued, nil, map[string]any{auditState: stateWaiting, "network": in.Network})
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM personal_channel_reply WHERE delivery_key = $1`, in.DeliveryKey).Scan(&state); err != nil {
		return err
	}
	if state == stateFailed || state == stateExpired {
		return ErrReplyClosed
	}
	return nil
}

// Pull leases up to limit of the caller's waiting replies, oldest first.
func (s *Store) Pull(ctx context.Context, limit int) (PullAnswer, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return PullAnswer{}, err
	}
	if limit < 1 || limit > maxPull {
		return PullAnswer{}, invalid("pull 1 to %d replies at a time", maxPull)
	}
	var out PullAnswer
	err := s.tx(ctx, func(tx pgx.Tx, user ids.UUID) error {
		conn, err := s.RequireEnabled(ctx, tx, user)
		if err != nil {
			return err
		}
		now := s.now()
		if out.Replies, err = leaseReplies(ctx, tx, conn.ID, now, limit); err != nil {
			return err
		}
		for _, r := range out.Replies {
			if _, err := storekit.Audit(ctx, tx, "update", replyEntity, r.DeliveryID,
				map[string]any{auditState: stateWaiting}, map[string]any{auditState: statePulled, "attempt": r.Attempt}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE personal_channel_connection SET last_pull_at = $2 WHERE id = $1`, conn.ID, now); err != nil {
			return err
		}
		after, err := readConnection(ctx, tx, user)
		out.Enabled, out.RepliesWaiting, out.OldestReplyWaitingAt = after.Enabled, after.RepliesWaiting, after.OldestReplyWaitingAt
		return err
	})
	return out, err
}

func leaseReplies(ctx context.Context, tx pgx.Tx, connection ids.UUID, now time.Time, limit int) ([]Reply, error) {
	rows, err := tx.Query(ctx, `
		WITH picked AS MATERIALIZED (
			SELECT id FROM personal_channel_reply
			 WHERE connection_id = $1
			   AND (state = 'waiting' OR (state = 'pulled' AND leased_until <= $2))
			 ORDER BY created_at, id LIMIT $4 FOR UPDATE SKIP LOCKED)
		, leased AS (
			UPDATE personal_channel_reply r
			   SET state = 'pulled', leased_until = $3, attempt = r.attempt + 1, updated_at = $2
			 WHERE r.id IN (SELECT id FROM picked)
			RETURNING r.id, r.network, r.recipient_account, r.thread_id, r.reply_to, r.body, r.attempt, r.created_at)
		SELECT id, network, recipient_account, thread_id, coalesce(reply_to, ''), coalesce(body, ''), attempt
		  FROM leased ORDER BY created_at, id`,
		connection, now, now.Add(replyLease), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reply
	for rows.Next() {
		var r Reply
		if err := rows.Scan(&r.DeliveryID, &r.Network, &r.Account, &r.GroupID, &r.ReplyTo, &r.Body, &r.Attempt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Ack records the client's word on each reply. A failed one is settled as not
// delivered in the same transaction; the core does not retry it. It runs while
// the connection is off too: a reply already sent must not be offered again.
func (s *Store) Ack(ctx context.Context, acks []Ack) ([]AckResult, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	if len(acks) == 0 || len(acks) > maxPull {
		return nil, invalid("ack 1 to %d replies at a time", maxPull)
	}
	for _, a := range acks {
		if a.Outcome != OutcomeSent && a.Outcome != OutcomeFailed {
			return nil, invalid("an ack's outcome is sent or failed, not %q", a.Outcome)
		}
	}
	if s.settler == nil {
		return nil, errNoSettler
	}
	out := make([]AckResult, 0, len(acks))
	err := s.tx(ctx, func(tx pgx.Tx, user ids.UUID) error {
		conn, err := readConnection(ctx, tx, user)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOff
		}
		if err != nil {
			return err
		}
		for _, a := range acks {
			res, err := s.ackOne(ctx, tx, conn.ID, a)
			if err != nil {
				return err
			}
			out = append(out, res)
		}
		return nil
	})
	return out, err
}

func (s *Store) ackOne(ctx context.Context, tx pgx.Tx, connection ids.UUID, a Ack) (AckResult, error) {
	res := AckResult{DeliveryID: a.DeliveryID}
	var activity ids.UUID
	var state string
	var attempt int
	err := tx.QueryRow(ctx, `SELECT activity_id, state, attempt FROM personal_channel_reply WHERE id = $1 AND connection_id = $2 FOR UPDATE`,
		a.DeliveryID, connection).Scan(&activity, &state, &attempt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		res.Result = AckUnknownDelivery
		return res, nil
	case err != nil:
		return res, err
	case state != statePulled && state != stateWaiting:
		res.Result = AckAlreadySettled
		return res, nil
	case a.Attempt != 0 && a.Attempt != attempt:
		res.Result = AckSuperseded
		return res, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE personal_channel_reply
		   SET state = $2, reason = nullif($3, ''), provider_message_id = nullif($4, ''), body = NULL,
		       recipient_account = NULL, leased_until = NULL, settled_at = $5, updated_at = $5
		 WHERE id = $1`, a.DeliveryID, a.Outcome, a.Reason, a.ProviderMessageID, s.now()); err != nil {
		return res, err
	}
	if _, err := storekit.Audit(ctx, tx, "update", replyEntity, a.DeliveryID,
		map[string]any{auditState: state}, map[string]any{auditState: a.Outcome, "reason": a.Reason}); err != nil {
		return res, err
	}
	res.Result = AckRecorded
	if a.Outcome == OutcomeFailed {
		return res, s.settler.NotDelivered(ctx, tx, activity, firstNonEmpty(a.Reason, notDeliveredByClient))
	}
	return res, nil
}

// expireReplies settles every reply no client acked within replyLifetime, each
// with its delivery in one transaction.
func expireReplies(ctx context.Context, db *database.DB, settler Settler, now time.Time) (int, error) {
	if settler == nil {
		return 0, errNoSettler
	}
	var expired int
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		dues, err := expireDue(ctx, tx, now)
		if err != nil {
			return err
		}
		for _, d := range dues {
			if _, err := storekit.Audit(ctx, tx, "update", replyEntity, d.reply,
				map[string]any{auditState: d.was}, map[string]any{auditState: stateExpired}); err != nil {
				return err
			}
			if err := settler.NotDelivered(ctx, tx, d.activity, notDeliveredByExpiry); err != nil {
				return err
			}
		}
		expired = len(dues)
		return nil
	})
	return expired, err
}

type dueReply struct {
	reply, activity ids.UUID
	was             string
}

func expireDue(ctx context.Context, tx pgx.Tx, now time.Time) ([]dueReply, error) {
	rows, err := tx.Query(ctx, `
		WITH due AS MATERIALIZED (
			SELECT id, state FROM personal_channel_reply
			 WHERE created_at < $3
			   AND (state = 'waiting' OR (state = 'pulled' AND leased_until <= $1)) FOR UPDATE)
		UPDATE personal_channel_reply r
		   SET state = 'expired', body = NULL, recipient_account = NULL, leased_until = NULL,
		       reason = $2, settled_at = $1, updated_at = $1
		  FROM due WHERE r.id = due.id
		RETURNING r.id, r.activity_id, due.state`, now, notDeliveredByExpiry, now.Add(-replyLifetime))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dueReply
	for rows.Next() {
		var d dueReply
		if err := rows.Scan(&d.reply, &d.activity, &d.was); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// settleSentByEcho settles the reply a pushed outbound message names as sent by
// that message, and answers the reply's activity. A reply of another connection,
// or none, answers nil.
func settleSentByEcho(ctx context.Context, tx pgx.Tx, connection ids.UUID, m PushMessage, now time.Time) (*ids.UUID, error) {
	var activity ids.UUID
	var state string
	err := tx.QueryRow(ctx, `SELECT activity_id, state FROM personal_channel_reply WHERE id = $1 AND connection_id = $2 FOR UPDATE`,
		m.DeliveryID, connection).Scan(&activity, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if state != statePulled && state != stateWaiting {
		return &activity, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE personal_channel_reply
		   SET state = 'sent', provider_message_id = $2, body = NULL, recipient_account = NULL,
		       leased_until = NULL, settled_at = $3, updated_at = $3
		 WHERE id = $1`, m.DeliveryID, m.MessageID, now); err != nil {
		return nil, err
	}
	_, err = storekit.Audit(ctx, tx, "update", replyEntity, m.DeliveryID,
		map[string]any{auditState: state}, map[string]any{auditState: OutcomeSent})
	return &activity, err
}
