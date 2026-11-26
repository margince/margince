// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatmembers answers the capture sink's two questions about a member's
// chat account from the identity module. Who owns this account? Is this message
// the code that confirms it?
package chatmembers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Members answers both questions from the members' verified accounts.
type Members struct {
	Identity *identity.Service
}

// New is Members over the installation's identity service.
func New(pool *pgxpool.Pool) Members { return Members{Identity: identity.NewService(pool)} }

// network names the provider's chat network. A provider with no verified
// accounts, and a WhatsApp sender with no phone number, answer false.
func network(provider, scope string) (identity.ChatNetwork, bool) {
	switch provider {
	case "telegram", "telegram_business":
		return identity.ChatTelegram, true
	case "whatsapp":
		return identity.ChatWhatsApp, scope == ""
	case "zalo_oa":
		return identity.ChatZalo, true
	}
	return "", false
}

// MemberOf is the member who verified the account.
func (m Members) MemberOf(ctx context.Context, tx pgx.Tx, provider, scope, account string) (ids.UUID, bool, error) {
	n, ok := network(provider, scope)
	if !ok {
		return ids.Nil, false, nil
	}
	return identity.ChatMemberTx(ctx, tx, n, scope, account)
}

// RedeemCodeTx verifies the account when text is a pending code. An account
// another member already verified still consumes the message, so the code is
// not filed as correspondence, and verifies nothing.
func (m Members) RedeemCodeTx(ctx context.Context, tx pgx.Tx, provider, scope, account, text string) (bool, error) {
	n, ok := network(provider, scope)
	if !ok {
		return false, nil
	}
	_, redeemed, err := identity.RedeemChatCodeTx(ctx, tx, n, scope, account, text)
	if errors.Is(err, identity.ErrChatIdentityTaken) {
		return true, nil
	}
	return redeemed, err
}
