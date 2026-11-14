// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The events secret of a channel connection: the key a provider that pushes
// notices of new messages signs them with. It is optional, sealed beside the
// bot token, and destroyed with it. Without one the channels are read on a
// timer only.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ChannelEventsCheck is a ChannelBotCheck whose provider pushes signed
// notices. A provider whose check lacks it takes no events secret.
type ChannelEventsCheck interface {
	ValidateEventsSecret(secret string) error
}

// ErrNoEventsSecret reports a live bot with no events secret sealed, so no
// notice can be verified.
var ErrNoEventsSecret = errors.New("capture: the live bot has no events secret")

// connectionRefs are the vault refs one connection names. events is empty
// when no events secret is sealed.
type connectionRefs struct {
	token, events keyvault.Ref
}

// checkEventsSecret refuses an events secret the provider cannot use. An
// empty one is no secret, which every provider takes.
func checkEventsSecret(check ChannelBotCheck, secret string) error {
	if secret == "" {
		return nil
	}
	events, ok := check.(ChannelEventsCheck)
	if !ok {
		return fmt.Errorf("this provider sends no events, so it takes no events secret: %w", apperrors.ErrInvalidArgument)
	}
	if err := events.ValidateEventsSecret(secret); err != nil {
		return fmt.Errorf("the events secret is not one the chat issues, which start with whsec_: %w: %w", err, apperrors.ErrInvalidArgument)
	}
	return nil
}

// sealConnectionSecrets seals the bot token and, when given, the events
// secret. A failure on the second destroys the first, which no row names yet.
func (s *ChannelStore) sealConnectionSecrets(ctx context.Context, ws ids.UUID, token, eventsSecret string) (connectionRefs, error) {
	tokenRef, err := s.sealSecret(ctx, ws, "bot token", token)
	if err != nil {
		return connectionRefs{}, err
	}
	eventsRef, err := s.sealSecret(ctx, ws, "events secret", eventsSecret)
	if err != nil {
		s.destroyDetached(ctx, ws, "channel-events-secret-unsealed", tokenRef)
		return connectionRefs{}, err
	}
	return connectionRefs{token: tokenRef, events: eventsRef}, nil
}

// destroyDetached destroys refs no row names any more. An empty ref is skipped.
func (s *ChannelStore) destroyDetached(ctx context.Context, ws ids.UUID, lifecycle string, refs ...keyvault.Ref) {
	for _, ref := range refs {
		keyvault.DeleteDetached(ctx, s.vault, s.log, ws, ref, lifecycle)
	}
}

// LiveBotEvents returns the provider's live bot without its token, and the
// events secret its notices are signed with. It answers ErrNoConnection when
// no bot is connected and ErrNoEventsSecret when the bot has no secret.
func (r *Registry) LiveBotEvents(ctx context.Context, provider string) (LiveBot, string, error) {
	bot, _, err := r.readLiveBot(ctx, provider)
	if err != nil {
		return LiveBot{}, "", err
	}
	var ref *string
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT events_credential_ref FROM channel_connection WHERE id = $1`, bot.ConnectionID).Scan(&ref)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveBot{}, "", ErrNoConnection
	}
	if err != nil {
		return LiveBot{}, "", fmt.Errorf("capture: reading the live %s bot's events secret: %w", provider, err)
	}
	if ref == nil || *ref == "" {
		return LiveBot{}, "", ErrNoEventsSecret
	}
	secret, err := r.resolveCredential(ctx, ref, nil)
	if err != nil {
		return LiveBot{}, "", err
	}
	return bot, string(secret), nil
}
