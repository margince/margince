// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func (w *Flusher) postNative(ctx context.Context, chat chatlinks.Chat, pending chatlinks.Pending, record chatRecord) error {
	if len(pending.Items) == 0 {
		return nil
	}
	for _, item := range pending.Items {
		prior, err := w.Store.BotDeliveryFor(ctx, pending.Link.ID, item.ID.String())
		if err == nil && prior.State == "sent" {
			return w.Store.MarkPosted(ctx, pending.Link.ID, item.ID, prior.Message, prior.Carried, pending.Link.MessageDetail)
		}
		if err == nil && prior.State != "refused" {
			return messaging.ErrDeliveryUnknown
		}
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
	}
	writer, ok := chat.(chatwire.ReviewedWriter)
	if !ok {
		return messaging.ErrUnsupported
	}
	prepared, err := w.nativeLines(ctx, chat, pending.Link, record, pending.Items, 25000)
	if err != nil {
		return err
	}
	if len(prepared.carried) == 0 {
		return nil
	}
	postID := prepared.carried[0]
	if len(prepared.text) == 0 {
		return w.Store.MarkPosted(ctx, pending.Link.ID, postID, "", prepared.carried, pending.Link.MessageDetail)
	}
	text := chatPostText(record, prepared.text, w.Origin)
	delivery, err := w.Store.DeliverBot(ctx, chatlinks.BotDeliveryRequest{Link: pending.Link, Key: postID.String(), ContentHash: chatFingerprint(text), AudienceHash: prepared.audience.key, Carried: prepared.carried}, func(sendCtx context.Context) (string, error) {
		posted, err := writer.PostReviewed(sendCtx, pending.Link.ChannelSlug, text, postID.String(), pending.Link.RootID, prepared.audience.key)
		return posted.ID, err
	})
	if err != nil {
		return err
	}
	return w.Store.MarkPosted(ctx, pending.Link.ID, postID, delivery.Message, delivery.Carried, pending.Link.MessageDetail)
}

func (w *Flusher) rewriteNative(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, record chatRecord, message string, items []chatlinks.Item) error {
	writer, ok := chat.(chatwire.ReviewedWriter)
	if !ok {
		return messaging.ErrUnsupported
	}
	prepared, err := w.nativeLines(ctx, chat, link, record, items, 0)
	if err != nil {
		return err
	}
	if err := writer.UpdateReviewed(ctx, link.ChannelSlug, message, chatPostText(record, prepared.text, w.Origin), prepared.audience.key); err != nil {
		return err
	}
	return w.Store.MarkRewritten(ctx, link.ID, message)
}

func (w *Flusher) postReviewed(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, text, key, root string) (chatwire.Posted, error) {
	if _, native := chat.(chatwire.RootReader); !native {
		return chat.Post(ctx, link.ChannelSlug, text, key, root)
	}
	prior, err := w.Store.BotDeliveryFor(ctx, link.ID, key)
	if err == nil && prior.State == "sent" {
		return chatwire.Posted{ID: prior.Message}, nil
	}
	if err == nil && prior.State != "refused" {
		return chatwire.Posted{}, messaging.ErrDeliveryUnknown
	}
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return chatwire.Posted{}, err
	}
	writer, ok := chat.(chatwire.ReviewedWriter)
	if !ok {
		return chatwire.Posted{}, messaging.ErrUnsupported
	}
	audience, err := w.outboundAudience(ctx, chat, link)
	if err != nil {
		return chatwire.Posted{}, err
	}
	delivery, err := w.Store.DeliverBot(ctx, chatlinks.BotDeliveryRequest{Link: link, Key: key, ContentHash: chatFingerprint(text), AudienceHash: audience.key}, func(sendCtx context.Context) (string, error) {
		posted, err := writer.PostReviewed(sendCtx, link.ChannelSlug, text, key, root, audience.key)
		return posted.ID, err
	})
	return chatwire.Posted{ID: delivery.Message}, err
}

func (w *Flusher) updateReviewed(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, message, text string) error {
	if _, native := chat.(chatwire.RootReader); !native {
		return chat.Update(ctx, link.ChannelSlug, message, text)
	}
	writer, ok := chat.(chatwire.ReviewedWriter)
	if !ok {
		return messaging.ErrUnsupported
	}
	audience, err := w.outboundAudience(ctx, chat, link)
	if err != nil {
		return err
	}
	return writer.UpdateReviewed(ctx, link.ChannelSlug, message, text, audience.key)
}

func postFailureReason(err error) string {
	switch {
	case errors.Is(err, chatlinks.ErrAudienceNeedsReview):
		return "audience_changed"
	case errors.Is(err, messaging.ErrDeliveryUnknown):
		return "delivery_unknown"
	default:
		reason, _ := nativeRootFailure(err)
		return reason
	}
}
