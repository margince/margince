// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The one thing the Brief's own tables cannot answer: whether anybody looked.
//
// What a run HELD is in brief_run and brief_item, and what a rep DID with it is
// in the audit rows the marks write. Neither records a morning the rep opened,
// read and closed — which is the reading the product most wants, because a
// night that ranked a queue nobody opened looks exactly like a night that
// ranked nothing worth opening.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// briefOpenedAction is the system_log action the open is recorded under. The
// ledger row is what makes the event attributable — Validate refuses an
// envelope with no trace link — and system_log rather than audit_log because
// reading a Brief mutates no record, which is the line LogSystem's own contract
// draws.
const briefOpenedAction = "brief.opened"

// recordOpenTimeout bounds the open's own write once the reader has the brief.
const recordOpenTimeout = 5 * time.Second

// RecordShown records that the signed-in rep was shown run, after the caller
// has handed it over. GET /v1/brief and the read_brief tool are the two
// callers, because those are where a rep reads the brief. The reader already
// has the brief, so a failed write is logged and never reaches them.
func (e *BriefEngine) RecordShown(ctx context.Context, run BriefRun) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordOpenTimeout)
	defer cancel()
	if err := e.recordOpened(ctx, run); err != nil {
		slog.ErrorContext(ctx, "the brief was shown but its open could not be recorded",
			"brief_run_id", run.ID, "error", err)
	}
}

// recordOpened writes the open in a transaction of its own, so the read that
// showed the brief writes nothing.
func (e *BriefEngine) recordOpened(ctx context.Context, run BriefRun) error {
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		return emitBriefOpened(ctx, tx, run)
	})
}

// emitBriefOpened records that this rep read this run.
//
// Counts only. The run's contents are the rep's own queue and are recoverable
// from brief_item under the reader's scope; copying any of it into an event
// payload would put deal names on a bus for a question that is answered by
// three integers.
//
// Entity-less, through EmitPipelinePayload: the subject of this event is the
// READING rather than the run, and there is nothing for a consumer to read
// back under its own scope, which is what an entity ref is for. It is also
// what keeps the type internal — every catalogued type outside the entity-less
// class is selectable by a webhook subscription. The run id rides the ledger
// row below instead, attributable without being deliverable.
func emitBriefOpened(ctx context.Context, tx pgx.Tx, run BriefRun) error {
	unread := 0
	for _, item := range run.Items {
		if item.State == briefStateNew {
			unread++
		}
	}
	// The ledger row first: it is the id the envelope's trace points at, so
	// there is no ordering in which an event exists without one.
	ledgerID, err := storekit.LogSystem(ctx, tx, briefOpenedAction, map[string]any{
		"brief_run_id": run.ID.String(),
		"local_day":    run.LocalDay.Format("2006-01-02"),
	})
	if err != nil {
		return fmt.Errorf("briefs: logging the brief open: %w", err)
	}
	payload := crmcontracts.InternalEventBriefOpened{
		LocalDay: openapi_types.Date{Time: run.LocalDay},
		Items:    len(run.Items),
		Unread:   unread,
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, ledgerID, payload); err != nil {
		return fmt.Errorf("briefs: publishing the brief open: %w", err)
	}
	return nil
}
