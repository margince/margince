// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedUnknownSendsStayUnconfirmedAndRequireAnExplicitNewCard(t *testing.T) {
	e := setupFlush(t)
	e.store.WithMemberAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) { return ctx, nil })
	human := e.member()
	actor, _ := principal.Actor(human)
	actor.SeatType = principal.SeatFull
	human = principal.WithActor(human, actor)
	system := principal.WithActor(human, chatLinksPrincipal)
	e.handle(t, "activity.captured", "activity", e.shared, `{"kind":"note"}`)
	pending, err := e.store.PendingFor(system, e.link)
	if err != nil || len(pending.Items) != 1 {
		t.Fatalf("pending: %#v %v", pending.Items, err)
	}
	session, err := e.flusher.Chats.Open(system)
	if err != nil {
		t.Fatal(err)
	}
	unknown := func(key string, carried []ids.UUID) {
		t.Helper()
		_, err := e.store.DeliverBot(system, chatlinks.BotDeliveryRequest{Link: pending.Link, Key: key, ContentHash: chatFingerprint("An eligible CRM update"), AudienceHash: "verified-fixture", Carried: carried}, func(ctx context.Context) (string, error) {
			if _, err := session.Chat.Post(ctx, pending.Link.ChannelSlug, "An eligible CRM update", key, ""); err != nil {
				return "", err
			}
			return "", messaging.ErrDeliveryUnknown
		})
		if !errors.Is(err, messaging.ErrDeliveryUnknown) {
			t.Fatalf("uncertain receipt: %v", err)
		}
	}
	unknown(pending.Items[0].ID.String(), []ids.UUID{pending.Items[0].ID})
	review, err := e.store.DeliveryReview(human, "deal", e.deal)
	if err != nil || len(review.Deliveries) != 1 || review.Deliveries[0].Updates != 1 || review.Deliveries[0].Card {
		t.Fatalf("review metadata: %#v %v", review, err)
	}
	decision := chatlinks.DeliveryReviewInput{Link: e.link, Action: "skip_unknown", Acknowledged: true, Deliveries: []ids.UUID{review.Deliveries[0].ID}}
	wrong := decision
	wrong.Link = ids.NewV7()
	if _, err := e.store.ReviewDeliveries(human, "deal", e.deal, wrong); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("review crossed link identity: %v", err)
	}
	if _, err := e.store.ReviewDeliveries(human, "deal", ids.NewV7(), decision); err == nil {
		t.Fatal("review crossed record identity")
	}
	denied := actor
	denied.Permissions.Objects = map[string]principal.ObjectGrant{"deal": {Read: true}}
	if _, err := e.store.ReviewDeliveries(principal.WithActor(human, denied), "deal", e.deal, decision); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read-only grant reviewed delivery: %v", err)
	}
	e.store.WithMemberAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) {
		return principal.WithActor(ctx, denied), nil
	})
	if _, err := e.store.ReviewDeliveries(human, "deal", e.deal, decision); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("stale session kept revoked review authority: %v", err)
	}
	e.store.WithMemberAuthority(func(ctx context.Context, _ pgx.Tx) (context.Context, error) { return ctx, nil })
	after, err := e.store.ReviewDeliveries(human, "deal", e.deal, decision)
	if err != nil || len(after.Deliveries) != 0 || after.CardPaused {
		t.Fatalf("review failed: %#v %v", after, err)
	}
	var state string
	var withdrawnUnsent bool
	if err := e.owner.QueryRow(context.Background(), `SELECT state FROM chat_bot_delivery WHERE id=$1`, decision.Deliveries[0]).Scan(&state); err != nil || state != "reviewed_unknown" {
		t.Fatalf("review invented delivery: %q %v", state, err)
	}
	if err := e.owner.QueryRow(context.Background(), `SELECT withdrawn_at IS NOT NULL AND posted_at IS NULL FROM chat_post_item WHERE id=$1`, pending.Items[0].ID).Scan(&withdrawnUnsent); err != nil || !withdrawnUnsent {
		t.Fatalf("uncertain item stamped sent: %v %v", withdrawnUnsent, err)
	}
	if _, err := e.store.ReviewDeliveries(human, "deal", e.deal, decision); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale review accepted: %v", err)
	}
	unknown("card:"+e.link.String()+":uncertain", nil)
	review, err = e.store.DeliveryReview(human, "deal", e.deal)
	if err != nil || len(review.Deliveries) != 1 || !review.Deliveries[0].Card {
		t.Fatalf("overview metadata: %#v %v", review, err)
	}
	decision.Deliveries = []ids.UUID{review.Deliveries[0].ID}
	after, err = e.store.ReviewDeliveries(human, "deal", e.deal, decision)
	if err != nil || !after.CardPaused {
		t.Fatalf("overview not paused: %#v %v", after, err)
	}
	e.flush(t)
	if len(e.chat.Posts()) != 2 {
		t.Fatal("review silently retried an uncertain provider copy")
	}
	restart := chatlinks.DeliveryReviewInput{Link: e.link, Action: "restart_card", Acknowledged: true}
	if _, err := e.store.ReviewDeliveries(human, "deal", e.deal, restart); err != nil {
		t.Fatal(err)
	}
	pending, err = e.store.PendingFor(system, e.link)
	if err != nil || pending.Link.CardPaused || pending.Link.CardGeneration == "" {
		t.Fatalf("new overview did not get its own generation: %#v %v", pending.Link, err)
	}
	e.flush(t)
	posts := e.chat.Posts()
	if len(posts) != 3 || !strings.HasSuffix(posts[2].ClientMsgID, ":"+pending.Link.CardGeneration) {
		t.Fatalf("explicit restart did not create exactly one fresh card: %#v", posts)
	}
	for _, post := range posts[:2] {
		if _, changed := e.chat.Updated(post.TS); changed {
			t.Fatal("local review changed an unconfirmed provider copy")
		}
	}
}
