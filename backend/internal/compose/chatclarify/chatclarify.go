// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatclarify runs one workspace's pass over the messages colleagues
// forwarded to a company chat account without naming the author. It forms the
// batches, asks each colleague who wrote them, and sends the follow-ups. The
// accounts send nothing else. What is sent, and to whom, is decided in the
// capture module; this package supplies the networks' senders and the clock.
package chatclarify

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatmembers"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// actorID names the sweep in whatever it touches.
const actorID = "system:chat-clarification"

// Sweep is the pass's dependencies. Senders yields the one resolver of chat
// accounts' senders, bound to the workspace's database.
type Sweep struct {
	Pool    *pgxpool.Pool
	Senders func(db *database.DB) capture.ChatSenders
	Now     func() time.Time
	Log     *slog.Logger
}

// SendersOf is the registry's resolver, the one the comms send path uses, bound
// to the workspace a pass runs for. A role with no registry resolves no bot.
func SendersOf(registry *capture.Registry, vault keyvault.Vault) func(db *database.DB) capture.ChatSenders {
	return func(db *database.DB) capture.ChatSenders {
		if registry == nil {
			return capture.NewChatSenders(db, vault, nil)
		}
		senders := registry.ChatSenders()
		senders.DB = db
		return senders
	}
}

// Workspace runs the pass for one tenant. No member decides it, so it runs as
// the system and is bound to the workspace it was queued for.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(ctx, actorID)
	return s.asker(workspace).Sweep(ctx)
}

func (s Sweep) asker(workspace ids.UUID) *capture.ChatAsker {
	db := database.ForInstallation(s.Pool)
	return &capture.ChatAsker{DB: db, Members: chatmembers.New(s.Pool), Senders: s.Senders(db), Now: s.Now, Log: s.Log}
}
