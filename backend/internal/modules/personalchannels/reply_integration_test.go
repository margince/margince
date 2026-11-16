// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannels

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type notDelivered struct{ settled map[ids.UUID]string }

func (n *notDelivered) NotDelivered(_ context.Context, _ pgx.Tx, activity ids.UUID, reason string) error {
	n.settled[activity] = reason
	return nil
}

// queued puts one reply for user on an activity the test seeds, as the send path does.
func (e *channelEnv) queued(t *testing.T, user ids.UUID, key string) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by) VALUES ($1, 'note', 'reply', now(), 'seed', 'system')`, activity)
	ctx := e.as(user, repGrant)
	c, err := e.store.Connection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return QueueReply(ctx, tx, ReplyInput{
			Connection: c.ID, ActivityID: activity, DeliveryKey: key,
			Network: "whatsapp", Account: "8490", Body: "Yes, the 2nd works",
		})
	}); err != nil {
		t.Fatal(err)
	}
	return activity
}

func (e *channelEnv) settling() *notDelivered {
	n := &notDelivered{settled: map[ids.UUID]string{}}
	e.store = e.store.WithSettler(n)
	return n
}

func TestAPulledReplyIsLeasedThenOfferedAgain(t *testing.T) {
	e := setupChannels(t)
	e.on(t, e.rep)
	e.queued(t, e.rep, "d1")
	first, err := e.store.Pull(e.as(e.rep, repGrant), 10)
	if err != nil || len(first.Replies) != 1 || first.Replies[0].Attempt != 1 || first.Replies[0].Account != "8490" {
		t.Fatalf("first pull = %+v, %v", first, err)
	}
	if again, _ := e.store.Pull(e.as(e.rep, repGrant), 10); len(again.Replies) != 0 {
		t.Fatal("a leased reply was handed out twice within its lease")
	}
	e.now = e.now.Add(replyLease + time.Second)
	later, err := e.store.Pull(e.as(e.rep, repGrant), 10)
	if err != nil || len(later.Replies) != 1 || later.Replies[0].Attempt != 2 {
		t.Fatalf("after the lease = %+v, %v; want it again with attempt 2", later, err)
	}
	if later.RepliesWaiting != 1 {
		t.Fatalf("replies waiting = %d, want 1 while it is still unacked", later.RepliesWaiting)
	}
}

func TestAnAckedReplyIsSettledAndAFailedOneNotDelivered(t *testing.T) {
	e := setupChannels(t)
	settled := e.settling()
	e.on(t, e.rep)
	sent := e.queued(t, e.rep, "d-sent")
	failed := e.queued(t, e.rep, "d-failed")
	pulled, err := e.store.Pull(e.as(e.rep, repGrant), 10)
	if err != nil || len(pulled.Replies) != 2 {
		t.Fatalf("pull = %+v, %v", pulled, err)
	}
	acks := []Ack{{DeliveryID: pulled.Replies[0].DeliveryID, Outcome: OutcomeSent}, {DeliveryID: pulled.Replies[1].DeliveryID, Outcome: OutcomeFailed, Reason: "blocked"}}
	res, err := e.store.Ack(e.as(e.rep, repGrant), acks)
	if err != nil || res[0].Result != AckRecorded || res[1].Result != AckRecorded {
		t.Fatalf("acks = %+v, %v", res, err)
	}
	if _, ok := settled.settled[sent]; ok || settled.settled[failed] != "blocked" {
		t.Fatalf("settled = %v; want only the failed reply, with its reason", settled.settled)
	}
	if n := e.count(t, `SELECT count(*) FROM personal_channel_reply WHERE body IS NOT NULL OR recipient_account IS NOT NULL`); n != 0 {
		t.Fatal("a settled reply kept its body or recipient")
	}
	again, err := e.store.Ack(e.as(e.rep, repGrant), acks[:1])
	if err != nil || again[0].Result != AckAlreadySettled {
		t.Fatalf("second ack = %+v, %v", again, err)
	}
}

func TestAnotherMembersDeliveryIsUnknown(t *testing.T) {
	e := setupChannels(t)
	e.settling()
	e.on(t, e.rep)
	e.on(t, e.other)
	e.queued(t, e.rep, "d-rep")
	pulled, err := e.store.Pull(e.as(e.rep, repGrant), 10)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.store.Ack(e.as(e.other, repGrant), []Ack{{DeliveryID: pulled.Replies[0].DeliveryID, Outcome: OutcomeSent}})
	if err != nil || res[0].Result != AckUnknownDelivery {
		t.Fatalf("ack by another member = %+v, %v", res, err)
	}
}

func TestAnUnpulledReplyExpiresAtSevenDays(t *testing.T) {
	e := setupChannels(t)
	settled := e.settling()
	e.on(t, e.rep)
	activity := e.queued(t, e.rep, "d-old")
	e.exec(t, `UPDATE personal_channel_reply SET created_at = $1`, e.now.Add(-replyLifetime+time.Hour))
	if n, err := expire(e.system(), e, settled); err != nil || n != 0 {
		t.Fatalf("before 7 days expired %d, %v", n, err)
	}
	e.exec(t, `UPDATE personal_channel_reply SET created_at = $1`, e.now.Add(-replyLifetime-time.Minute))
	if n, err := expire(e.system(), e, settled); err != nil || n != 1 {
		t.Fatalf("after 7 days expired %d, %v", n, err)
	}
	if settled.settled[activity] != notDeliveredByExpiry {
		t.Fatalf("settled = %v", settled.settled)
	}
	if _, err := expire(e.as(e.rep, repGrant), e, settled); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member ran the expiry: %v", err)
	}
}

func TestPullIsRefusedWhileOffButAPulledReplyCanStillBeAcked(t *testing.T) {
	e := setupChannels(t)
	e.settling()
	if _, err := e.store.Ack(e.as(e.rep, repGrant), []Ack{{DeliveryID: ids.NewV7(), Outcome: OutcomeSent}}); !errors.Is(err, ErrOff) {
		t.Fatalf("ack with no connection: %v", err)
	}
	e.on(t, e.rep)
	e.queued(t, e.rep, "d-sent-then-off")
	pulled, err := e.store.Pull(e.as(e.rep, repGrant), 10)
	if err != nil || len(pulled.Replies) != 1 {
		t.Fatalf("pull = %+v, %v", pulled, err)
	}
	if _, err := e.store.SetEnabled(e.as(e.rep, repGrant), false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Pull(e.as(e.rep, repGrant), 10); !errors.Is(err, ErrOff) {
		t.Fatalf("pull while off: %v", err)
	}
	res, err := e.store.Ack(e.as(e.rep, repGrant), []Ack{{DeliveryID: pulled.Replies[0].DeliveryID, Outcome: OutcomeSent}})
	if err != nil || res[0].Result != AckRecorded {
		t.Fatalf("ack after turning off = %+v, %v; want it recorded so the reply is not offered again", res, err)
	}
}

func TestAReplyBeingSentIsNotExpiredUnderItsLease(t *testing.T) {
	e := setupChannels(t)
	settled := e.settling()
	e.on(t, e.rep)
	e.queued(t, e.rep, "d-leased")
	e.exec(t, `UPDATE personal_channel_reply SET created_at = $1`, e.now.Add(-replyLifetime+time.Minute))
	if _, err := e.store.Pull(e.as(e.rep, repGrant), 10); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(2 * time.Minute)
	if n, err := expire(e.system(), e, settled); err != nil || n != 0 {
		t.Fatalf("expired %d, %v while the client holds the lease", n, err)
	}
	e.now = e.now.Add(replyLease)
	if n, err := expire(e.system(), e, settled); err != nil || n != 1 {
		t.Fatalf("expired %d, %v once the lease lapsed", n, err)
	}
}

func expire(ctx context.Context, e *channelEnv, settler Settler) (int, error) {
	_, replies, err := Sweep(ctx, e.store.db, settler, e.now)
	return replies, err
}
