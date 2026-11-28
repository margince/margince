// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type deliveryResult struct {
	state, code, message string
	retrySeconds         float64
}

func classifyDelivery(receipt messaging.Receipt, conversation string, err error) deliveryResult {
	out := deliveryResult{state: "failed", code: "provider_refused"}
	var limited *connector.RateLimitedError
	switch {
	case errors.Is(err, messaging.ErrDeliveryUnknown):
		out.state, out.code = deliveryStateUnknown, failureDeliveryUnknown
	case errors.As(err, &limited):
		out.code, out.retrySeconds = failureRateLimited, limited.RetryAfter.Seconds()
	case errors.Is(err, messaging.ErrUnauthorized):
		out.code = "reconnect"
	case errors.Is(err, messaging.ErrUnavailable):
		out.code = "unavailable"
	case errors.Is(err, messaging.ErrInvalid):
		out.code = "review_message"
	case errors.Is(err, messaging.ErrUnsupported):
		out.code = "unsupported"
	case err == nil:
		if receipt.ID == "" || receipt.Conversation != conversation {
			out.state, out.code = "unknown", "delivery_unknown"
			return out
		}
		out.state, out.code, out.message = "accepted", "", receipt.ID
	}
	return out
}

func (s *Store) deliverMessenger(ctx context.Context, kind string, id ids.UUID, grant MemberGrant, client messaging.Client, in MessengerSend, attempt MessengerAttempt) (MessengerAttempt, error) {
	attempted := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := requireRecordRead(ctx, kind); err != nil {
			return err
		}
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		if err := holdMemberConsentTx(ctx, tx, grant); err != nil {
			return err
		}
		attempted = true
		receipt, sendErr := client.Send(ctx, messaging.Send{Conversation: in.Conversation, Root: in.Root, Text: in.Text, RequestID: in.RequestID.String(), ExpectedAudience: in.AudienceKey})
		attempt, err = writeDelivery(ctx, tx, attempt, grant.UserID, classifyDelivery(receipt, in.Conversation, sendErr))
		return err
	})
	if err != nil {
		if attempted {
			return MessengerAttempt{}, messaging.ErrDeliveryUnknown
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		cleanupErr := s.db.Tx(cleanupCtx, func(tx pgx.Tx) error {
			_, err := writeDelivery(cleanupCtx, tx, attempt, grant.UserID, deliveryResult{state: "failed", code: "authorization_changed"})
			return err
		})
		return MessengerAttempt{}, errors.Join(err, cleanupErr)
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerAttempt{}, err
	}
	return attempt, nil
}

func writeDelivery(ctx context.Context, tx pgx.Tx, attempt MessengerAttempt, user ids.UUID, result deliveryResult) (MessengerAttempt, error) {
	saved, err := scanAttempt(tx.QueryRow(ctx, `UPDATE chat_send_attempt SET state=@state,provider_message_id=@message,
 failure_code=@code,retry_at=CASE WHEN @retry::double precision>0 THEN now()+@retry*interval '1 second' ELSE NULL END,updated_at=now()
 WHERE id=@id AND user_id=@user AND state='pending' RETURNING `+attemptColumns, pgx.StrictNamedArgs{stateKey: result.state, messageKey: result.message, "code": result.code, "retry": result.retrySeconds, "id": attempt.ID, userKey: user}))
	if err != nil {
		return MessengerAttempt{}, err
	}
	_, err = storekit.Audit(ctx, tx, "update", "chat_send_attempt", attempt.ID, map[string]any{stateKey: "pending"}, map[string]any{stateKey: result.state, failureCodeKey: result.code})
	return saved, err
}
