// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type receiptClient struct {
	messaging.Client
	sends   atomic.Int64
	sendErr error
	started chan struct{}
	release chan struct{}
}

func (c *receiptClient) Identity(context.Context) (messaging.Identity, error) {
	return messaging.Identity{Workspace: "TEAM", Member: messaging.Member{ID: "REP", Name: "Rep", Active: true}}, nil
}

func (c *receiptClient) Conversation(context.Context, string) (messaging.Conversation, error) {
	return messaging.Conversation{ID: "ROOM", Kind: "group", AudienceKnown: true, Members: []messaging.Member{{ID: "REP", Active: true}, {ID: "COLLEAGUE", Active: true}}}, nil
}

func (c *receiptClient) Send(_ context.Context, in messaging.Send) (messaging.Receipt, error) {
	c.sends.Add(1)
	if c.started != nil {
		close(c.started)
		<-c.release
	}
	if c.sendErr != nil {
		return messaging.Receipt{}, c.sendErr
	}
	return messaging.Receipt{Conversation: in.Conversation, ID: "PROVIDER-MESSAGE"}, nil
}

func setupMessengerSend(t *testing.T, client *receiptClient) (*linkEnv, MessengerSend) {
	t.Helper()
	e := setupSlackMemberLinks(t)
	vault := keyvault.NewMemory()
	e.store.WithMessenger(func() keyvault.Vault { return vault }, func(context.Context, MemberGrant, []byte) (messaging.Client, error) { return client, nil })
	grant, err := e.store.ConnectMember(e.seat(e.rep), MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "REP"}, []byte("member-token"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := client.Conversation(context.Background(), "ROOM")
	if err != nil {
		t.Fatal(err)
	}
	audience, err := messaging.AudienceKey(room)
	if err != nil {
		t.Fatal(err)
	}
	return e, MessengerSend{RequestID: ids.NewV7(), GrantVersion: grant.Version, Conversation: "ROOM", Text: "Can you check the policy?", AudienceKey: audience}
}

func TestMessengerSendHasOneReceiptAndRejectsChangedReplay(t *testing.T) {
	client := &receiptClient{}
	e, in := setupMessengerSend(t, client)
	ctx := e.seat(e.rep)
	first, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in)
	if err != nil || first.State != "accepted" || first.Message != "PROVIDER-MESSAGE" {
		t.Fatalf("first receipt: %+v %v", first, err)
	}
	second, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in)
	if err != nil || second.ID != first.ID || client.sends.Load() != 1 {
		t.Fatalf("replayed send: %+v %v sends=%d", second, err, client.sends.Load())
	}
	in.Text = "Different text with the same request"
	if _, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("changed replay accepted: %v", err)
	}
	if _, err := e.store.MessengerAttempt(e.seat(e.other), RecordDeal, e.deal, "slack", in.RequestID); err == nil {
		t.Fatal("another rep read the private receipt")
	}
	if n := e.count(t, "SELECT count(*) FROM activity"); n != 0 {
		t.Fatalf("send implicitly filed %d CRM entries", n)
	}
}

func TestUnknownMessengerDeliveryIsNeverResentByReplay(t *testing.T) {
	client := &receiptClient{sendErr: messaging.ErrDeliveryUnknown}
	e, in := setupMessengerSend(t, client)
	for range 2 {
		receipt, err := e.store.SendMessenger(e.seat(e.rep), RecordDeal, e.deal, "slack", in)
		if err != nil || receipt.State != "unknown" {
			t.Fatalf("unknown delivery: %+v %v", receipt, err)
		}
	}
	if client.sends.Load() != 1 {
		t.Fatalf("unknown message resent %d times", client.sends.Load())
	}
}

func TestUnknownDeliveryRequiresReviewAfterReconnectAndAcrossRequests(t *testing.T) {
	client := &receiptClient{sendErr: messaging.ErrDeliveryUnknown}
	e, in := setupMessengerSend(t, client)
	ctx := e.seat(e.rep)
	original, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in)
	if err != nil || original.State != "unknown" {
		t.Fatalf("original: %+v %v", original, err)
	}
	grant, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "REP"}, []byte("renewed-member-token"))
	if err != nil {
		t.Fatal(err)
	}
	context, err := e.store.MessengerConversation(ctx, RecordDeal, e.deal, "slack", "ROOM")
	if err != nil || context.Unresolved == nil || context.Unresolved.RequestID != in.RequestID {
		t.Fatalf("reconnect hid an unresolved delivery: %+v %v", context, err)
	}
	replacement := in
	replacement.RequestID = ids.NewV7()
	replacement.GrantVersion = grant.Version
	if _, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", replacement); !errors.Is(err, ErrMessengerDeliveryNeedsReview) || client.sends.Load() != 1 {
		t.Fatalf("another client bypassed review: %v sends=%d", err, client.sends.Load())
	}
	replacement.AcknowledgedRequestID = in.RequestID
	client.sendErr = nil
	sent, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", replacement)
	if err != nil || sent.State != "accepted" || client.sends.Load() != 2 {
		t.Fatalf("explicit review could not start a new attempt: %+v %v", sent, err)
	}
	replacement.AcknowledgedRequestID = ids.Nil
	if _, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", replacement); err != nil || client.sends.Load() != 2 {
		t.Fatalf("review metadata changed a replay's content identity: %v", err)
	}
	replacement.RequestID = ids.NewV7()
	if _, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", replacement); err != nil || client.sends.Load() != 3 {
		t.Fatalf("reviewed unknown blocked later legitimate messages: %v", err)
	}
}

func TestConcurrentMessengerReplayReturnsPendingWithoutAnotherSend(t *testing.T) {
	client := &receiptClient{started: make(chan struct{}), release: make(chan struct{})}
	e, in := setupMessengerSend(t, client)
	done := make(chan error, 1)
	go func() { _, err := e.store.SendMessenger(e.seat(e.rep), RecordDeal, e.deal, "slack", in); done <- err }()
	<-client.started
	pending, err := e.store.SendMessenger(e.seat(e.rep), RecordDeal, e.deal, "slack", in)
	close(client.release)
	firstErr := <-done
	if err != nil || firstErr != nil || pending.State != "pending" || client.sends.Load() != 1 {
		t.Fatalf("concurrent send: %+v %v %v sends=%d", pending, err, firstErr, client.sends.Load())
	}
	receipt, err := e.store.MessengerAttempt(e.seat(e.rep), RecordDeal, e.deal, "slack", in.RequestID)
	if err != nil || receipt.State != "accepted" {
		t.Fatalf("final receipt: %+v %v", receipt, err)
	}
}

func TestProviderRateHoldSurvivesReceiptReplayAndBlocksAnotherSend(t *testing.T) {
	client := &receiptClient{sendErr: &connector.RateLimitedError{RetryAfter: time.Hour}}
	e, in := setupMessengerSend(t, client)
	ctx := e.seat(e.rep)
	first, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in)
	if err != nil || first.State != "failed" || first.FailureCode != "rate_limited" || first.RetryAt == nil {
		t.Fatalf("rate limit feedback was lost: %+v %v", first, err)
	}
	replay, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in)
	if err != nil || replay.ID != first.ID || replay.RetryAt == nil {
		t.Fatalf("receipt replay lost the rate hold: %+v %v", replay, err)
	}
	in.RequestID = ids.NewV7()
	_, err = e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in)
	var limited *connector.RateLimitedError
	if !errors.As(err, &limited) || client.sends.Load() != 1 {
		t.Fatalf("provider was called during its hold: %v sends=%d", err, client.sends.Load())
	}
}

func TestReadSeatCannotSendOpenGroupsOrGrantProviderAccess(t *testing.T) {
	client := &receiptClient{}
	e, in := setupMessengerSend(t, client)
	ctx := e.seat(e.rep)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	actor.SeatType = principal.SeatRead
	ctx = principal.WithActor(ctx, actor)
	if _, err := e.store.SendMessenger(ctx, RecordDeal, e.deal, "slack", in); !errors.Is(err, apperrors.ErrSeatTierInsufficient) {
		t.Fatalf("read seat sent: %v", err)
	}
	if _, err := e.store.MessengerOpenMembers(ctx, RecordDeal, e.deal, "slack", []string{"COLLEAGUE"}); !errors.Is(err, apperrors.ErrSeatTierInsufficient) {
		t.Fatalf("read seat opened a group: %v", err)
	}
	if _, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "REP"}, []byte("replacement")); !errors.Is(err, apperrors.ErrSeatTierInsufficient) {
		t.Fatalf("read seat replaced a grant: %v", err)
	}
	if _, _, err := e.store.OpenMember(ctx, "slack"); err != nil {
		t.Fatalf("read seat could not read its existing authorized account: %v", err)
	}
	if client.sends.Load() != 0 {
		t.Fatal("read seat reached the provider send")
	}
}

func TestAnotherClientCannotStartANewMessageWhileDeliveryIsPending(t *testing.T) {
	client := &receiptClient{started: make(chan struct{}), release: make(chan struct{})}
	e, in := setupMessengerSend(t, client)
	done := make(chan error, 1)
	go func() { _, err := e.store.SendMessenger(e.seat(e.rep), RecordDeal, e.deal, "slack", in); done <- err }()
	<-client.started
	next := in
	next.RequestID = ids.NewV7()
	next.AcknowledgedRequestID = in.RequestID
	_, err := e.store.SendMessenger(e.seat(e.rep), RecordDeal, e.deal, "slack", next)
	close(client.release)
	firstErr := <-done
	if !errors.Is(err, ErrMessengerDeliveryNeedsReview) || firstErr != nil || client.sends.Load() != 1 {
		t.Fatalf("concurrent request bypassed pending delivery: %v %v sends=%d", err, firstErr, client.sends.Load())
	}
}
