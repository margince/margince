// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ChannelBot is what a provider confirms about a bot token. ID is the
// provider's own id, which the binding is keyed on; Label is shown.
type ChannelBot struct {
	ID    string
	Label string
}

// ChannelBotCheck confirms a bot token with one provider before anything is
// sealed. Connect and ReplaceToken run the same check, so a provider's rules for
// a usable bot live in one place. Address is the provider's web address for a
// self-hosted provider and empty for one with a fixed API.
type ChannelBotCheck interface {
	ValidateToken(token string) error
	Confirm(ctx context.Context, address, token string) (ChannelBot, error)
	NeedsAddress() bool
	// FixedIdentity reports whether a replacement token must belong to the
	// same bot, because records outside this table are bound to it.
	FixedIdentity() bool
}

// The provider-neutral refusals a ChannelBotCheck wraps, so the transport maps
// every provider's failure to the same wire codes.
var (
	ErrChannelTokenRejected       = errors.New("capture: the provider rejected the bot token")
	ErrChannelProviderUnreachable = errors.New("capture: the provider could not be reached")
)

// telegramBotCheck confirms a Telegram bot and clears its webhook, because
// Telegram refuses the long poll while a webhook is registered. Pending updates
// are kept: they are the customer's messages, and the first poll collects them.
type telegramBotCheck struct{ api telegram.API }

func (telegramBotCheck) ValidateToken(token string) error { return telegram.ValidateToken(token) }

func (telegramBotCheck) NeedsAddress() bool { return false }

func (telegramBotCheck) FixedIdentity() bool { return false }

func (c telegramBotCheck) Confirm(ctx context.Context, _, token string) (ChannelBot, error) {
	bot, err := c.api.GetMe(ctx, token)
	if err != nil {
		return ChannelBot{}, err
	}
	if err := c.api.DeleteWebhook(ctx, token); err != nil {
		return ChannelBot{}, fmt.Errorf("capture: clearing the bot's webhook so its updates can be polled: %w", err)
	}
	return ChannelBot{ID: fmt.Sprintf("%d", bot.ID), Label: bot.Username}, nil
}

// botCheck returns the provider's check, or refuses a provider this deployment
// composed no check for.
func (s *ChannelStore) botCheck(provider string) (ChannelBotCheck, error) {
	check, ok := s.bots[provider]
	if !ok {
		return nil, fmt.Errorf("channel provider %q is not implemented: %w", provider, apperrors.ErrConflict)
	}
	return check, nil
}

// WithBotCheck returns a copy of the store that also connects bots of
// provider, so compose adds a provider without this module importing it.
func (s *ChannelStore) WithBotCheck(provider string, check ChannelBotCheck) *ChannelStore {
	next := *s
	next.bots = make(map[string]ChannelBotCheck, len(s.bots)+1)
	maps.Copy(next.bots, s.bots)
	next.bots[provider] = check
	return &next
}
