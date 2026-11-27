// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	recordIDKey = "record_id"
	requestKey  = "request"
	retryAtKey  = "retry_at"
	revisionKey = "revision"
	tenantKey   = "tenant"
	urlKey      = "url"
)

const (
	appKey                 = "app"
	audienceKey            = "audience"
	channelIDKey           = "channel_id"
	channelKey             = "channel"
	companyIDKey           = "company_id"
	connectionKey          = "connection"
	conversationIDKey      = "conversation_id"
	conversationKey        = "conversation"
	credentialKey          = "credential"
	deletedKey             = "deleted"
	deliveryStateSent      = "sent"
	deliveryStateUnknown   = "unknown"
	failureCodeKey         = "failure_code"
	failureDeliveryUnknown = "delivery_unknown"
	failureRateLimited     = "rate_limited"
	grantKey               = "grant"
	keyKey                 = "key"
	kindKey                = "kind"
	lastErrorKey           = "last_error"
	linkKey                = "link"
	messageKey             = "message"
	previousKey            = "previous"
	providerKey            = "provider"
	reasonKey              = "reason"
	reauthorizeKey         = "reauthorize"
	recordKey              = "record"
	restartCardAction      = "restart_card"
	rootIDKey              = "root_id"
	rootKey                = "root"
	signedAtKey            = "signed_at"
	stateKey               = "state"
	subscriptionKey        = "subscription"
	teamKey                = "team"
	updateKey              = "update"
	userKey                = "user"
	userKind               = "user"
	versionKey             = "version"
)

// BotDeliveryRequest pins a shared post to its reviewed content and audience.
type BotDeliveryRequest struct {
	Link         Link
	Key          string
	ContentHash  string
	AudienceHash string
	Carried      []ids.UUID
}

// BotDelivery retains shared-send identity for replay and uncertain-delivery review.
type BotDelivery struct {
	ID      ids.UUID
	State   string
	Message string
	Carried []ids.UUID
}

// BotDeliveryFor reads the durable outcome before a shared post is retried.
func (s *Store) BotDeliveryFor(ctx context.Context, link ids.UUID, key string) (BotDelivery, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return BotDelivery{}, err
	}
	var out BotDelivery
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id,state,coalesce(message_id,''),carried_item_ids FROM chat_bot_delivery WHERE link_id=@link AND request_key=@key`, pgx.StrictNamedArgs{linkKey: link, keyKey: key}).Scan(&out.ID, &out.State, &out.Message, &out.Carried)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

// DeliverBot holds durable send state around the external provider call.
func (s *Store) DeliverBot(ctx context.Context, in BotDeliveryRequest, send func(context.Context) (string, error)) (BotDelivery, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return BotDelivery{}, err
	}
	if send == nil || in.Key == "" || len(in.Key) > 200 || in.ContentHash == "" || in.AudienceHash == "" {
		return BotDelivery{}, apperrors.ErrInvalidArgument
	}
	if in.Carried == nil {
		in.Carried = []ids.UUID{}
	}
	delivery, err := s.beginBotDelivery(ctx, in)
	if err != nil {
		return BotDelivery{}, err
	}
	if delivery.State == deliveryStateSent {
		return delivery, nil
	}
	message, sendErr := send(ctx)
	state := "sent"
	if sendErr != nil {
		state = deliveryStateUnknown
		if knownBotRefusal(sendErr) {
			state = "refused"
		}
	} else if message == "" {
		state = deliveryStateUnknown
		sendErr = messaging.ErrDeliveryUnknown
	}
	delivery.Message, delivery.State = message, state
	if err := s.finishBotDelivery(ctx, delivery); err != nil {
		return BotDelivery{}, errors.Join(messaging.ErrDeliveryUnknown, err)
	}
	return delivery, sendErr
}

func (s *Store) beginBotDelivery(ctx context.Context, in BotDeliveryRequest) (BotDelivery, error) {
	var out BotDelivery
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := holdPostingLinkTx(ctx, tx, in.Link); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: "chat-bot-delivery:" + in.Link.ID.String()}); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id,state,coalesce(message_id,''),carried_item_ids FROM chat_bot_delivery WHERE link_id=@link AND request_key=@key FOR UPDATE`, pgx.StrictNamedArgs{linkKey: in.Link.ID, "key": in.Key}).Scan(&out.ID, &out.State, &out.Message, &out.Carried)
		if err == nil && out.State == "sent" {
			return nil
		}
		if err == nil && out.State != "refused" {
			return messaging.ErrDeliveryUnknown
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_bot_delivery WHERE link_id=@link AND state IN ('sending','unknown'))`, pgx.StrictNamedArgs{linkKey: in.Link.ID}).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return messaging.ErrDeliveryUnknown
		}
		before := map[string]any(nil)
		operation := ModeCreate
		if errors.Is(err, pgx.ErrNoRows) {
			out.ID = ids.NewV7()
			if _, err := tx.Exec(ctx, `INSERT INTO chat_bot_delivery(id,link_id,request_key,content_hash,audience_hash,carried_item_ids,state) VALUES(@id,@link,@key,@content,@audience,@carried,'sending')`, pgx.StrictNamedArgs{"id": out.ID, linkKey: in.Link.ID, "key": in.Key, "content": in.ContentHash, audienceKey: in.AudienceHash, "carried": in.Carried}); err != nil {
				return err
			}
		} else {
			before = botDeliveryImage(out)
			operation = updateKey
			if changed, err := tx.Exec(ctx, `UPDATE chat_bot_delivery SET content_hash=@content,audience_hash=@audience,carried_item_ids=@carried,state='sending',message_id=NULL,updated_at=now() WHERE id=@id AND state='refused'`, pgx.StrictNamedArgs{"id": out.ID, "content": in.ContentHash, "audience": in.AudienceHash, "carried": in.Carried}); err != nil {
				return err
			} else if changed.RowsAffected() != 1 {
				return apperrors.ErrConflict
			}
		}
		out.State, out.Message, out.Carried = "sending", "", in.Carried
		_, err = storekit.Audit(ctx, tx, operation, "chat_bot_delivery", out.ID, before, botDeliveryImage(out))
		return err
	})
	return out, err
}

func (s *Store) finishBotDelivery(ctx context.Context, delivery BotDelivery) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		before := delivery
		before.State, before.Message = "sending", ""
		tag, err := tx.Exec(ctx, `UPDATE chat_bot_delivery SET state=@state,message_id=nullif(@message,''),updated_at=now() WHERE id=@id AND state='sending'`, pgx.StrictNamedArgs{"id": delivery.ID, stateKey: delivery.State, messageKey: delivery.Message})
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrConflict
		}
		_, err = storekit.Audit(ctx, tx, "update", "chat_bot_delivery", delivery.ID, botDeliveryImage(before), botDeliveryImage(delivery))
		return err
	})
}

func botDeliveryImage(delivery BotDelivery) map[string]any {
	return map[string]any{stateKey: delivery.State, "message_id": delivery.Message, "carried_item_ids": delivery.Carried}
}

func knownBotRefusal(err error) bool {
	var limited *connector.RateLimitedError
	return errors.As(err, &limited) || errors.Is(err, chatwire.ErrTokenRejected) || errors.Is(err, chatwire.ErrNotMember) || errors.Is(err, chatwire.ErrRequestRejected) || errors.Is(err, messaging.ErrInvalid) || errors.Is(err, messaging.ErrUnauthorized) || errors.Is(err, messaging.ErrUnsupported)
}

func holdPostingLinkTx(ctx context.Context, tx pgx.Tx, link Link) error {
	var posting bool
	if err := tx.QueryRow(ctx, `SELECT NOT post_disabled AND post_supported AND status='active' FROM chat_link WHERE id=@id AND archived_at IS NULL AND connection_id=@connection AND version=@version AND deal_id IS NOT DISTINCT FROM @deal AND project_id IS NOT DISTINCT FROM @project AND company_id IS NOT DISTINCT FROM @company FOR SHARE`, pgx.StrictNamedArgs{"id": link.ID, connectionKey: link.ConnectionID, versionKey: link.Version, "deal": link.DealID, "project": link.ProjectID, "company": link.CompanyID}).Scan(&posting); err != nil {
		return err
	}
	if !posting {
		return ErrLinkNotFound
	}
	return nil
}
