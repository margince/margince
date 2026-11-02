// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ThreadRepairGen persists a repair wake before acknowledging an ordinary mail
// edit; the worker resumes the same bounded engine capture already uses.
type ThreadRepairGen struct{ pool *pgxpool.Pool }

// NewThreadRepairGen binds durable edit recovery to the installation database.
func NewThreadRepairGen(pool *pgxpool.Pool) *ThreadRepairGen { return &ThreadRepairGen{pool: pool} }

// HandleEvent acknowledges an edit only after its repair wake commits.
func (g *ThreadRepairGen) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != eventActivityUpdated || env.Entity.ID.IsZero() {
		return nil
	}
	if env.Actor.ID == threadRepairActor || env.Actor.ID == "connector:thread_repair" {
		return nil
	}
	var updated crmcontracts.PublicEventActivityUpdated
	if err := json.Unmarshal(env.Payload, &updated); err != nil {
		return fmt.Errorf("%w: decode %s: %v", events.ErrUnactionable, env.Type, err)
	}
	if updated.ChangedFields.ThreadKey != nil || updated.ChangedFields.Subject == nil {
		return nil
	}
	db := InstallationDB(g.pool)
	ws, err := db.Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(principal.SystemActing(ctx, "system:thread_repair_wake"), env.Trace.CorrelationID)
	return db.Tx(ctx, func(tx pgx.Tx) error {
		var key string
		err := tx.QueryRow(ctx, `SELECT thread_key FROM activity WHERE id=@id AND kind='email' AND thread_key IS NOT NULL
 AND archived_at IS NULL AND restricted_at IS NULL`, pgx.NamedArgs{"id": env.Entity.ID}).Scan(&key)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return capture.QueueThreadRepairTx(ctx, tx, key)
	})
}
