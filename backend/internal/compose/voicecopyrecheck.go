// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A voice sample read from sent mail stays only while the mail could still be
// learned from. Every write that archives, restricts, narrows or retitles a
// mail emits an event. So one consumer rechecks the mail behind each of them,
// whichever module wrote it. Learning holds its mails share-locked until it
// commits, so a narrowing either waits for the sample or finds it here.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// voiceCopyRecheckEvents are the events after which a mail may no longer feed
// a voice. Each names the activity it changed.
var voiceCopyRecheckEvents = map[string]bool{
	"activity.updated":     true,
	"activity.archived":    true,
	"retention.restricted": true,
}

// VoiceCopyRecheckGen is the consumer (cg:voice-copy-recheck).
type VoiceCopyRecheckGen struct {
	db  *database.DB
	now func() time.Time
}

// NewVoiceCopyRecheckGen builds the consumer over the shared pool.
func NewVoiceCopyRecheckGen(pool *pgxpool.Pool) *VoiceCopyRecheckGen {
	return &VoiceCopyRecheckGen{db: InstallationDB(pool), now: time.Now}
}

// HandleEvent rechecks the mail an event changed. The recheck reads the row as
// it stands, so a redelivered or out-of-order event answers the same.
func (g *VoiceCopyRecheckGen) HandleEvent(ctx context.Context, env events.Envelope) error {
	if !voiceCopyRecheckEvents[env.Type] || env.Entity.Type != "activity" || env.Entity.ID == ids.Nil {
		return nil
	}
	ws, err := g.db.Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:voice_copy_recheck",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	return g.db.Tx(ctx, func(tx pgx.Tx) error {
		return recheckVoiceCopiesTx(ctx, tx, env.Entity.ID, g.now().UTC())
	})
}
