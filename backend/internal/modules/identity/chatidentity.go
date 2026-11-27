// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A member's own account on a chat network, proved by a one-time code. The
// member asks for a code in their profile and sends it from the account to a
// company account of that network. Only a verified account is stored, so no
// unverified value can ever be used to recognise a member.
//
// WhatsApp is the sender's phone number in digits. Telegram is the user id,
// global across bots. Zalo is the user id under one official account, and the
// official account is stored beside it, so one Zalo field covers one OA.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ChatNetwork is a chat network a member can verify an account on.
type ChatNetwork string

// The networks a member can verify an account on.
const (
	ChatWhatsApp ChatNetwork = "whatsapp"
	ChatTelegram ChatNetwork = "telegram"
	ChatZalo     ChatNetwork = "zalo"
)

// ChatCodeLifetime is how long a verification code can be sent.
const ChatCodeLifetime = 10 * time.Minute

// chatCodeAlphabet leaves out characters that look alike.
const chatCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const chatCodeLength = 8

// ErrChatIdentityTaken is returned when another member already verified the account.
var ErrChatIdentityTaken = fmt.Errorf("another member already verified this chat account: %w", apperrors.ErrConflict)

// chatColumns are the columns of one network, from a closed set so the SQL
// below never takes an identifier from a caller.
type chatColumns struct{ identity, verifiedAt, codeHash, codeExpires string }

func columnsOf(n ChatNetwork) (chatColumns, error) {
	switch n {
	case ChatWhatsApp:
		return chatColumns{"chat_whatsapp_phone", "chat_whatsapp_verified_at", "chat_whatsapp_code_hash", "chat_whatsapp_code_expires_at"}, nil
	case ChatTelegram:
		return chatColumns{"chat_telegram_user_id", "chat_telegram_verified_at", "chat_telegram_code_hash", "chat_telegram_code_expires_at"}, nil
	case ChatZalo:
		return chatColumns{"chat_zalo_user_id", "chat_zalo_verified_at", "chat_zalo_code_hash", "chat_zalo_code_expires_at"}, nil
	}
	return chatColumns{}, fmt.Errorf("a chat network is whatsapp, telegram or zalo: %w", apperrors.ErrInvalidArgument)
}

// ChatIdentity is a network where the member has a verified account or a waiting code.
type ChatIdentity struct {
	Network ChatNetwork
	Account string
	OAID    string
	// VerifiedAt is nil while only a code waits.
	VerifiedAt *time.Time
	// CodeExpiresAt is nil when no unexpired code waits.
	CodeExpiresAt *time.Time
}

func hashChatCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// NormalChatCode folds what a member typed into the form a code is stored in.
func NormalChatCode(text string) string { return strings.ToUpper(strings.TrimSpace(text)) }

func newChatCode() (string, error) {
	raw := make([]byte, chatCodeLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("identity: making a chat code: %w", err)
	}
	for i, b := range raw {
		raw[i] = chatCodeAlphabet[int(b)%len(chatCodeAlphabet)]
	}
	return string(raw), nil
}

// StartChatVerification makes the caller's code for one network and returns it
// once. A second start replaces the first, so one code is active per network.
func (s *Service) StartChatVerification(ctx context.Context, network ChatNetwork) (string, time.Time, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return "", time.Time{}, err
	}
	self, err := selfScoped(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	cols, err := columnsOf(network)
	if err != nil {
		return "", time.Time{}, err
	}
	code, err := newChatCode()
	if err != nil {
		return "", time.Time{}, err
	}
	var expires time.Time
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := storekit.LockRow(ctx, tx, "app_user", self, storekit.LiveOnly); err != nil {
			return err
		}
		// The database clock dates the code here and judges it when it is sent,
		// so the API and the worker cannot disagree about it.
		err := tx.QueryRow(ctx, `UPDATE app_user SET `+cols.codeHash+` = $2, `+cols.codeExpires+` = now() + make_interval(secs => $3)
			WHERE id = $1 AND `+workforce.LiveMemberSQL("")+` RETURNING `+cols.codeExpires,
			self, hashChatCode(code), ChatCodeLifetime.Seconds()).Scan(&expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("identity: storing a chat code: %w", err)
		}
		if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
			return err
		}
		return recordChatChange(ctx, tx, self, map[string]any{"chat_verification_started": string(network)})
	})
	return code, expires, err
}

// RemoveChatIdentity forgets the caller's verified account and any pending code on a network.
func (s *Service) RemoveChatIdentity(ctx context.Context, network ChatNetwork) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	self, err := selfScoped(ctx)
	if err != nil {
		return err
	}
	cols, err := columnsOf(network)
	if err != nil {
		return err
	}
	zalo := ""
	if network == ChatZalo {
		zalo = ", chat_zalo_oa_id = NULL"
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE app_user SET `+cols.identity+` = NULL, `+cols.verifiedAt+` = NULL`+zalo+`,
			`+cols.codeHash+` = NULL, `+cols.codeExpires+` = NULL WHERE id = $1 AND `+workforce.LiveMemberSQL(""), self)
		if err != nil {
			return fmt.Errorf("identity: removing a chat account: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrNotFound
		}
		if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
			return err
		}
		return recordChatChange(ctx, tx, self, map[string]any{"chat_identity_removed": string(network)})
	})
}

// MyChatIdentities lists the networks where the caller has a verified account
// or an unexpired code.
func (s *Service) MyChatIdentities(ctx context.Context) ([]ChatIdentity, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	self, err := selfScoped(ctx)
	if err != nil {
		return nil, err
	}
	var out []ChatIdentity
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var wa, tg, zalo, oa *string
		var at, codes [3]*time.Time
		if err := tx.QueryRow(ctx, `SELECT chat_whatsapp_phone, chat_whatsapp_verified_at,
			CASE WHEN chat_whatsapp_code_expires_at > now() THEN chat_whatsapp_code_expires_at END,
			chat_telegram_user_id, chat_telegram_verified_at,
			CASE WHEN chat_telegram_code_expires_at > now() THEN chat_telegram_code_expires_at END,
			chat_zalo_user_id, chat_zalo_oa_id, chat_zalo_verified_at,
			CASE WHEN chat_zalo_code_expires_at > now() THEN chat_zalo_code_expires_at END FROM app_user WHERE id = $1`, self).
			Scan(&wa, &at[0], &codes[0], &tg, &at[1], &codes[1], &zalo, &oa, &at[2], &codes[2]); err != nil {
			return fmt.Errorf("identity: reading chat accounts: %w", err)
		}
		for i, row := range []ChatIdentity{
			{Network: ChatWhatsApp, Account: derefChat(wa)},
			{Network: ChatTelegram, Account: derefChat(tg)},
			{Network: ChatZalo, Account: derefChat(zalo), OAID: derefChat(oa)},
		} {
			row.VerifiedAt = at[i]
			row.CodeExpiresAt = codes[i]
			if row.VerifiedAt != nil || row.CodeExpiresAt != nil {
				out = append(out, row)
			}
		}
		return nil
	})
	return out, err
}

func derefChat(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ChatMemberTx is the active member who verified this account. scope is the
// official account for Zalo and empty elsewhere.
func ChatMemberTx(ctx context.Context, tx pgx.Tx, network ChatNetwork, scope, account string) (ids.UUID, bool, error) {
	cols, err := columnsOf(network)
	if err != nil {
		return ids.Nil, false, err
	}
	query := `SELECT id FROM app_user WHERE ` + cols.identity + ` = $1 AND ` + cols.verifiedAt + ` IS NOT NULL
		AND ` + workforce.LiveMemberSQL("") + ` AND NOT is_agent`
	args := []any{account}
	if network == ChatZalo {
		query += ` AND chat_zalo_oa_id = $2`
		args = append(args, scope)
	}
	var member ids.UUID
	err = tx.QueryRow(ctx, query, args...).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("identity: reading the member behind a chat account: %w", err)
	}
	return member, true, nil
}

// ChatAccount is a member's verified account on one network, with the language
// the member reads Margince in.
type ChatAccount struct {
	// Scope is the official account for Zalo and empty elsewhere.
	Scope  string
	ID     string
	Locale string
}

// ChatAccountTx is the verified account the active member has on a network. It
// answers false when the member has none, has left, or is an agent.
func ChatAccountTx(ctx context.Context, tx pgx.Tx, network ChatNetwork, member ids.UUID) (ChatAccount, bool, error) {
	cols, err := columnsOf(network)
	if err != nil {
		return ChatAccount{}, false, err
	}
	scope := `''`
	if network == ChatZalo {
		scope = `chat_zalo_oa_id`
	}
	var out ChatAccount
	var locale *string
	err = tx.QueryRow(ctx, `SELECT `+cols.identity+`, `+scope+`, locale FROM app_user
		WHERE id = $1 AND `+cols.identity+` IS NOT NULL AND `+cols.verifiedAt+` IS NOT NULL
		AND `+workforce.LiveMemberSQL("")+` AND NOT is_agent`, member).Scan(&out.ID, &out.Scope, &locale)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatAccount{}, false, nil
	}
	if err != nil {
		return ChatAccount{}, false, fmt.Errorf("identity: reading the chat account of a member: %w", err)
	}
	out.Locale = derefChat(locale)
	return out, true, nil
}

// RedeemChatCodeTx verifies the account that sent text as a member's own when
// the text is that member's pending, unexpired code and nothing else. It reports whether
// text was a pending code, so the caller files nothing for one. A code that is
// wrong or expired changes nothing and reports false. An account another member
// already verified is refused.
func RedeemChatCodeTx(ctx context.Context, tx pgx.Tx, network ChatNetwork, scope, account, text string) (ids.UUID, bool, error) {
	cols, err := columnsOf(network)
	if err != nil {
		return ids.Nil, false, err
	}
	code := NormalChatCode(text)
	if len(code) != chatCodeLength {
		return ids.Nil, false, nil
	}
	var member ids.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM app_user WHERE `+cols.codeHash+` = $1 AND `+cols.codeExpires+` > now()
		AND `+workforce.LiveMemberSQL("")+` AND NOT is_agent FOR UPDATE`, hashChatCode(code)).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("identity: finding a chat code: %w", err)
	}
	set, args := cols.identity+` = $2, `+cols.verifiedAt+` = now()`, []any{member, account}
	if network == ChatZalo {
		set += `, chat_zalo_oa_id = $3`
		args = append(args, scope)
	}
	// A savepoint, so an account another member holds leaves the transaction usable.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return ids.Nil, false, fmt.Errorf("identity: verifying a chat account: %w", err)
	}
	if _, err := sp.Exec(ctx, `UPDATE app_user SET `+set+`, `+cols.codeHash+` = NULL, `+cols.codeExpires+` = NULL WHERE id = $1 AND `+workforce.LiveMemberSQL(""), args...); err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return ids.Nil, false, fmt.Errorf("identity: verifying a chat account: %w", rbErr)
		}
		if _, unique := storekit.UniqueViolation(err); unique {
			return member, true, ErrChatIdentityTaken
		}
		return ids.Nil, false, fmt.Errorf("identity: verifying a chat account: %w", err)
	}
	if err := sp.Commit(ctx); err != nil {
		return ids.Nil, false, fmt.Errorf("identity: verifying a chat account: %w", err)
	}
	if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
		return ids.Nil, false, err
	}
	return member, true, recordChatChange(ctx, tx, member, map[string]any{"chat_identity_verified": string(network)})
}

// recordChatChange audits a change to a member's chat accounts and announces
// the member as updated. The image names the network and never the account.
func recordChatChange(ctx context.Context, tx pgx.Tx, user ids.UUID, image map[string]any) error {
	auditID, err := storekit.Audit(ctx, tx, "update", "user", user, map[string]any{"chat_identity_change": false}, image)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, user, crmcontracts.PublicEventUserUpdated{
		UserId: openapi_types.UUID(user), By: openapi_types.UUID(user),
	})
}

// clearedChatAccountsSQL sets every chat account and pending code of a member to
// nothing. A member who leaves frees their accounts, so a successor can confirm
// the same company account, and no code of theirs stays usable.
const clearedChatAccountsSQL = `chat_whatsapp_phone = NULL, chat_whatsapp_verified_at = NULL,
	chat_whatsapp_code_hash = NULL, chat_whatsapp_code_expires_at = NULL,
	chat_telegram_user_id = NULL, chat_telegram_verified_at = NULL,
	chat_telegram_code_hash = NULL, chat_telegram_code_expires_at = NULL,
	chat_zalo_user_id = NULL, chat_zalo_oa_id = NULL, chat_zalo_verified_at = NULL,
	chat_zalo_code_hash = NULL, chat_zalo_code_expires_at = NULL`
