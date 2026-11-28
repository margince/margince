// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func unresolvedMessengerTx(ctx context.Context, tx pgx.Tx, grant MemberGrant, conversation string) (*MessengerAttempt, error) {
	attempt, err := scanAttempt(tx.QueryRow(ctx, `SELECT `+attemptColumns+` FROM chat_send_attempt
 WHERE user_id=@user AND grant_id=@grant AND conversation_id=@conversation
 AND state IN ('pending','unknown') AND failure_code<>'reviewed_unknown' ORDER BY created_at,id LIMIT 1`, pgx.StrictNamedArgs{userKey: grant.UserID, "grant": grant.ID, conversationKey: conversation}))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No unresolved delivery permits a new request.
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func reviewUnresolvedMessengerTx(ctx context.Context, tx pgx.Tx, grant MemberGrant, in MessengerSend) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(@key,0))", pgx.StrictNamedArgs{keyKey: "chat-send-review:" + grant.UserID.String() + ":" + grant.ID.String() + ":" + in.Conversation}); err != nil {
		return err
	}
	earlier, err := unresolvedMessengerTx(ctx, tx, grant, in.Conversation)
	if err != nil || earlier == nil {
		return err
	}
	if needsMessengerReview(earlier, in) {
		return ErrMessengerDeliveryNeedsReview
	}
	if earlier.RequestID == in.RequestID {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE chat_send_attempt SET failure_code='reviewed_unknown' WHERE id=@id AND user_id=@user`, pgx.StrictNamedArgs{"id": earlier.ID, userKey: grant.UserID}); err != nil {
		return err
	}
	_, err = storekit.Audit(ctx, tx, "update", "chat_send_attempt", earlier.ID, map[string]any{failureCodeKey: earlier.FailureCode}, map[string]any{"failure_code": "reviewed_unknown"})
	return err
}

func needsMessengerReview(earlier *MessengerAttempt, in MessengerSend) bool {
	return earlier != nil && earlier.RequestID != in.RequestID && (earlier.State != deliveryStateUnknown || earlier.RequestID != in.AcknowledgedRequestID)
}

func (s *Store) checkUnresolvedMessenger(ctx context.Context, kind string, id ids.UUID, provider string, in MessengerSend) error {
	grant, err := s.MemberGrant(ctx, provider)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		earlier, err := unresolvedMessengerTx(ctx, tx, grant, in.Conversation)
		if err != nil {
			return err
		}
		if needsMessengerReview(earlier, in) {
			return ErrMessengerDeliveryNeedsReview
		}
		return nil
	})
}
