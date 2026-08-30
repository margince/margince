// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// When a human limits a message's audience AFTER the derived read models were
// built, the models have to follow the limit — the interaction-edge projection
// refolds on the same activity.updated through its own consumer, and this one
// handles what that fold cannot: the derived SIGNALS whose evidence cites the
// limited message (a workspace-visible summary of a limited email is that
// email's content, read by everyone), the vector and the attention label
// derived from the message's own text, and the thread-scan watermark, so the
// next extraction pass re-reads the conversation under its new audience.
//
// Narrowing is the direction that needs the consumer. Widening re-derives
// itself: the embedding generator listens to the same activity.updated and
// re-indexes a row that is workspace again, and the classify backlog picks the
// row up on its next pass. A narrowed row produces no such work, so what was
// derived while it was open would simply stay.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AudienceRescopeGen is the audience-change consumer (cg:audience-rescope).
type AudienceRescopeGen struct {
	db *database.DB
}

// NewAudienceRescopeGen builds the consumer over the shared pool.
func NewAudienceRescopeGen(pool *pgxpool.Pool) *AudienceRescopeGen {
	return &AudienceRescopeGen{db: InstallationDB(pool)}
}

// HandleEvent reacts to an activity.updated whose changed_fields carries the
// audience — the shape SetAudience emits, and nothing else. Every other event
// answers nil so the group keeps flowing.
func (g *AudienceRescopeGen) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != "activity.updated" || env.Entity.ID == ids.Nil {
		return nil
	}
	var payload struct {
		ChangedFields struct {
			Audience *string `json:"audience"`
		} `json:"changed_fields"`
	}
	// A payload this consumer cannot read is an update it was not built for
	// (the plain field patch carries no audience), never an error to wedge
	// the group on — the same skip an absent audience takes.
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return nil //nolint:nilerr // an unreadable payload is a skip, not a failure to retry
	}
	if payload.ChangedFields.Audience == nil {
		return nil
	}
	ctx, err := g.rescopeContext(ctx, env)
	if err != nil {
		return err
	}
	return g.db.Tx(ctx, func(tx pgx.Tx) error {
		return g.rescope(ctx, tx, env.Entity.ID)
	})
}

// rescopeContext binds the installation's workspace and a system principal:
// the models being corrected are maintenance state, and the correction must
// reach every row whatever the limiting human could themselves read.
func (g *AudienceRescopeGen) rescopeContext(ctx context.Context, env events.Envelope) (context.Context, error) {
	ws, err := g.db.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:audience_rescope",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	}), nil
}

func (g *AudienceRescopeGen) rescope(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	var threadKey *string
	var owner *ids.UUID
	// The capture owner is the one reader a limited message always admits —
	// the same spelling the extraction's due-threads scan resolves it by. The
	// join proves the parsed id names a HUMAN seat: an agent-captured
	// activity stamps its passport id there, and a passport is not a reader a
	// signal can answer to — it resolves to no owner, and the signals narrow
	// to the archive path instead of failing the app_user FK.
	var current string
	err := tx.QueryRow(ctx, `
		SELECT a.thread_key, a.audience, u.id
		  FROM activity a
		  LEFT JOIN app_user u
		    ON u.id = substring(a.captured_by from '([0-9a-f-]{36})$')::uuid
		 WHERE a.id = $1`, activityID).Scan(&threadKey, &current, &owner)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Deleted or erased between the event and this pass: the models
			// follow the archive through their own consumers.
			return nil
		}
		return fmt.Errorf("audience-rescope: reading activity %s: %w", activityID, err)
	}
	// The ROW's audience, not the event's. The bus is at-least-once and events
	// arrive out of order, so a message narrowed and widened again before this
	// consumer ran would otherwise be corrected towards a narrowing that no
	// longer stands: the vector deleted and the label cleared on a row that is
	// workspace, with nothing to rebuild them, because the widening's own event
	// was handled first and found nothing to do.
	//
	// The event still decides that a correction is DUE — that is what the
	// payload's changed_fields is for. What it must not decide is which
	// direction, because by now the answer is on the row.
	if current != "workspace" {
		if _, err := signals.NarrowDerivedForActivity(ctx, tx, activityID, owner); err != nil {
			return err
		}
		if err := activities.RetractDerivedForActivityTx(ctx, tx, activityID); err != nil {
			return err
		}
	}
	// Widened or narrowed alike, the conversation's summary state is stale:
	// dropping the scan watermark makes the thread due, and the next pass
	// re-derives under the audience as it now stands (a still-live narrowed
	// signal keeps its fingerprint, so the re-derivation cannot double it).
	if threadKey != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM signal_thread_scan WHERE thread_key = $1`, *threadKey); err != nil {
			return fmt.Errorf("audience-rescope: marking thread due again: %w", err)
		}
	}
	return nil
}
