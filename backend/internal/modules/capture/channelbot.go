// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LiveBot is a provider's live workspace bot with its unsealed token. It serves
// code that talks to the provider as that bot.
type LiveBot struct {
	ConnectionID ids.UUID
	// TeamID is the provider's id for the team the bot belongs to, held in
	// channel_id.
	TeamID  string
	Address string
	Token   string
}

// LiveBot returns the provider's one live bot. It reads the binding through
// liveChannelBinding, so it counts the same rows a reply send counts, and
// answers ErrNoConnection when no bot is connected.
func (r *Registry) LiveBot(ctx context.Context, provider string) (LiveBot, error) {
	binding, err := r.liveChannelBinding(ctx, provider)
	if err != nil {
		return LiveBot{}, err
	}
	bot := LiveBot{ConnectionID: binding.id}
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT channel_id, coalesce(address, '') FROM channel_connection WHERE id = $1`, binding.id).
			Scan(&bot.TeamID, &bot.Address)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveBot{}, ErrNoConnection
	}
	if err != nil {
		return LiveBot{}, fmt.Errorf("capture: reading the live %s bot: %w", provider, err)
	}
	secret, err := r.resolveCredential(ctx, &binding.credentialRef, nil)
	if err != nil {
		return LiveBot{}, err
	}
	bot.Token = string(secret)
	return bot, nil
}
