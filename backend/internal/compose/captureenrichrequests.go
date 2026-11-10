// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (g *CaptureEnrichTrigger) requestContacts(ctx context.Context, env events.Envelope) (bool, error) {
	var target *ids.UUID
	if env.Type == eventActivityUpdated {
		var payload crmcontracts.PublicEventActivityUpdated
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return false, err
		}
		audience := payload.ChangedFields.Audience
		if audience == nil || *audience != crmcontracts.Workspace {
			target = signatureRelinkedContact(payload.ChangedFields)
			if target == nil {
				return false, nil
			}
		}
	}
	requested := false
	err := runPerWorkspace(ctx, g.pool, func(ctx context.Context, workspace ids.UUID) error {
		ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "agent:enrich")
		return database.WithWorkspaceTx(ctx, g.pool, func(tx pgx.Tx) error {
			changed, err := g.requestContactsTx(ctx, tx, env, target)
			requested = requested || changed
			return err
		})
	})
	return requested, err
}

func (g *CaptureEnrichTrigger) requestContactsTx(ctx context.Context, tx pgx.Tx, env events.Envelope, target *ids.UUID) (bool, error) {
	history, err := historyImportForEventTx(ctx, tx, env)
	if err != nil {
		return false, err
	}
	if history == nil && (events.IsImported(env) || g.now().Sub(env.OccurredAt) > captureEnrichFreshWindow) {
		return false, nil
	}
	var deferred *ids.UUID
	if history != nil {
		deferred, err = historyDeferRunTx(ctx, tx, history)
		if err != nil {
			return false, err
		}
	}
	contactIDs := []ids.ContactID{}
	if env.Entity.Type == string(recordTypeContact) {
		contactIDs = append(contactIDs, ids.From[ids.ContactKind](env.Entity.ID))
	} else {
		enabled, err := config.StoredValTx(ctx, tx, StoredConfig(g.pool), capture.SignatureEnrich)
		if err != nil {
			return false, err
		}
		contactIDs, err = contacts.SignatureContactsForActivityTx(ctx, tx, env.Entity.ID, enabled, target)
		if err != nil {
			return false, err
		}
	}
	if len(contactIDs) == 0 {
		return false, nil
	}
	source, err := derivedAuditSourceTx(ctx, tx, env.Trace.AuditLogID)
	if err != nil {
		return false, err
	}
	changed, err := contacts.QueueSignatureEventTx(ctx, tx, contactIDs, contacts.SignatureRequest{DeferredRun: deferred, SourceXID: source.xid, Reopen: env.Type == eventActivityUpdated || (history != nil && env.Type == eventActivityCaptured), Live: history == nil})
	if err != nil {
		return false, err
	}
	return changed, nil
}

const (
	historyReleaseRunBatch = 8
	historyReleaseRowBatch = 256
)

func releaseTerminalImportWork(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	ctx = principal.SystemActing(ctx, "agent:enrich")
	more := false
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		replacement, err := historyDeferRunTx(ctx, tx, nil)
		if err != nil {
			return err
		}
		args := []any{historyReleaseRunBatch}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT b.id FROM capture_backfill b
   WHERE b.status IN ('done','error','cancelled') AND (EXISTS (
    SELECT 1 FROM contact_signature_enrich_state st WHERE st.deferred_run_id=b.id)
    OR EXISTS(SELECT 1 FROM graph_contact_edge e WHERE e.deferred_run_id=b.id)
    OR EXISTS(SELECT 1 FROM graph_interaction_edge e WHERE e.deferred_run_id=b.id))
   ORDER BY b.id LIMIT $%d FOR SHARE OF b SKIP LOCKED`, len(args)), args...)
		if err != nil {
			return err
		}
		runs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		contactsCount, err := contacts.ReleaseSignatureWorkTx(ctx, tx, runs, replacement, historyReleaseRowBatch)
		if err != nil {
			return err
		}
		graphCount, err := search.ReleaseGraphHistoryTx(ctx, tx, runs, replacement, historyReleaseRowBatch)
		if err != nil {
			return err
		}
		released := contactsCount + graphCount
		more = contactsCount >= historyReleaseRowBatch || graphCount >= historyReleaseRowBatch || (len(runs) == historyReleaseRunBatch && released > 0)
		return nil
	})
	return more, err
}
