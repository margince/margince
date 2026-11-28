// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MessengerSend pins text to the conversation and audience the sender reviewed.
type MessengerSend struct {
	AcknowledgedRequestID ids.UUID `json:"-"`
	RequestID             ids.UUID
	GrantVersion          int64
	Conversation          string
	Root                  string
	Text                  string
	AudienceKey           string
}

// MessengerAttempt retains the sender's durable delivery outcome without filing the private text.
type MessengerAttempt struct {
	ID           ids.UUID
	RequestID    ids.UUID
	Conversation string
	Message      string
	State        string
	UpdatedAt    time.Time
	FailureCode  string
	RetryAt      *time.Time
	contentHash  string
}

const attemptColumns = `id,request_id,conversation_id,provider_message_id,
 CASE WHEN state='pending' AND created_at<now()-interval '60 seconds' THEN 'unknown' ELSE state END,
 updated_at,content_hash,failure_code,retry_at`

func scanAttempt(row pgx.Row) (MessengerAttempt, error) {
	var attempt MessengerAttempt
	err := row.Scan(&attempt.ID, &attempt.RequestID, &attempt.Conversation, &attempt.Message, &attempt.State, &attempt.UpdatedAt, &attempt.contentHash, &attempt.FailureCode, &attempt.RetryAt)
	return attempt, err
}

// MessengerAttempt retains the sender's durable delivery outcome without filing the private text.
func (s *Store) MessengerAttempt(ctx context.Context, kind string, id ids.UUID, provider string, request ids.UUID) (MessengerAttempt, error) {
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerAttempt{}, err
	}
	user, err := memberActor(ctx)
	if err != nil {
		return MessengerAttempt{}, err
	}
	var attempt MessengerAttempt
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.readMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		attempt, err = scanAttempt(tx.QueryRow(ctx, "SELECT "+attemptColumns+" FROM chat_send_attempt WHERE user_id=@user AND request_id=@request AND record_type=@kind AND record_id=@record AND grant_id IN (SELECT id FROM chat_member_grant WHERE provider=@provider)",
			pgx.StrictNamedArgs{userKey: user, requestKey: request, kindKey: kind, recordKey: id, providerKey: provider}))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MessengerAttempt{}, apperrors.ErrNotFound
	}
	return attempt, err
}

func sendHash(kind string, id ids.UUID, provider string, in MessengerSend) (string, error) {
	raw, err := json.Marshal(struct {
		Kind     string
		Record   ids.UUID
		Provider string
		Input    MessengerSend
	}{Kind: kind, Record: id, Provider: provider, Input: in})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// SendMessenger records delivery state around the provider call to prevent duplicate sends.
func (s *Store) SendMessenger(ctx context.Context, kind string, id ids.UUID, provider string, in MessengerSend) (MessengerAttempt, error) {
	if _, err := memberMutationActor(ctx); err != nil {
		return MessengerAttempt{}, err
	}
	if err := s.admitMessengerRecord(ctx, kind, id); err != nil {
		return MessengerAttempt{}, err
	}
	if !validMessengerSend(provider, in) {
		return MessengerAttempt{}, apperrors.ErrInvalidArgument
	}
	hash, err := sendHash(kind, id, provider, in)
	if err != nil {
		return MessengerAttempt{}, err
	}
	previous, err := s.MessengerAttempt(ctx, kind, id, provider, in.RequestID)
	if err == nil {
		if previous.contentHash != hash {
			return MessengerAttempt{}, apperrors.ErrConflict
		}
		return previous, nil
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		return MessengerAttempt{}, err
	}
	if err := s.checkUnresolvedMessenger(ctx, kind, id, provider, in); err != nil {
		return MessengerAttempt{}, err
	}
	if err := s.messengerSendHold(ctx, provider); err != nil {
		return MessengerAttempt{}, err
	}
	grant, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return MessengerAttempt{}, err
	}
	if grant.Version != in.GrantVersion {
		return MessengerAttempt{}, apperrors.ErrVersionSkew
	}
	if err := validateSendAudience(ctx, client, in); err != nil {
		return MessengerAttempt{}, err
	}
	attempt, created, err := s.prepareAttempt(ctx, kind, id, grant, in, hash)
	if err != nil || !created {
		return attempt, err
	}
	return s.deliverMessenger(ctx, kind, id, grant, client, in, attempt)
}

func (s *Store) prepareAttempt(ctx context.Context, kind string, id ids.UUID, grant MemberGrant, in MessengerSend, hash string) (MessengerAttempt, bool, error) {
	var attempt MessengerAttempt
	created := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		if err := holdMemberConsentTx(ctx, tx, grant); err != nil {
			return err
		}
		if err := reviewUnresolvedMessengerTx(ctx, tx, grant, in); err != nil {
			return err
		}
		var err error
		attempt, err = scanAttempt(tx.QueryRow(ctx, `INSERT INTO chat_send_attempt(user_id,grant_id,grant_version,record_type,record_id,
   conversation_id,root_id,request_id,content_hash,audience_hash,state)
   VALUES(@user,@grant,@version,@kind,@record,@conversation,@root,@request,@hash,@audience,'pending')
   ON CONFLICT(user_id,request_id) DO NOTHING RETURNING `+attemptColumns, pgx.StrictNamedArgs{
			userKey: grant.UserID, "grant": grant.ID, versionKey: grant.Version, kindKey: kind, "record": id, conversationKey: in.Conversation,
			rootKey: in.Root, "request": in.RequestID, "hash": hash, audienceKey: in.AudienceKey,
		}))
		if errors.Is(err, pgx.ErrNoRows) {
			attempt, err = scanAttempt(tx.QueryRow(ctx, "SELECT "+attemptColumns+" FROM chat_send_attempt WHERE user_id=@user AND request_id=@request",
				pgx.StrictNamedArgs{userKey: grant.UserID, "request": in.RequestID}))
			if err == nil && attempt.contentHash != hash {
				return apperrors.ErrConflict
			}
			return err
		}
		if err != nil {
			return err
		}
		created = true
		_, err = storekit.Audit(ctx, tx, "create", "chat_send_attempt", attempt.ID, nil, map[string]any{stateKey: "pending", "user_id": grant.UserID})
		return err
	})
	return attempt, created, err
}

func (s *Store) messengerSendHold(ctx context.Context, provider string) error {
	user, err := memberActor(ctx)
	if err != nil {
		return err
	}
	var seconds *float64
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT max(extract(epoch from a.retry_at-now()))::double precision
 FROM chat_send_attempt a JOIN chat_member_grant g ON g.id=a.grant_id
 WHERE a.user_id=@user AND g.provider=@provider AND a.grant_version=g.consent_version AND a.retry_at>now()`, pgx.StrictNamedArgs{userKey: user, providerKey: provider}).Scan(&seconds)
	})
	if err != nil {
		return err
	}
	if seconds != nil && *seconds > 0 {
		return &connector.RateLimitedError{RetryAfter: time.Duration(math.Ceil(*seconds)) * time.Second}
	}
	return nil
}

func validMessengerSend(provider string, in MessengerSend) bool {
	return validProvider(provider) && !in.RequestID.IsZero() && in.GrantVersion >= 1 && in.Conversation != "" && in.AudienceKey != "" && strings.TrimSpace(in.Text) != "" && len(in.Text) <= 90000
}

func validateSendAudience(ctx context.Context, client messaging.Client, in MessengerSend) error {
	room, err := client.Conversation(ctx, in.Conversation)
	if err != nil {
		return err
	}
	audience, err := messaging.AudienceKey(room)
	if err != nil || room.External || audience != in.AudienceKey {
		return messaging.ErrInvalid
	}
	return nil
}
