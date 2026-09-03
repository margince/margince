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

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// briefOpenedAction is the system_log action the open is recorded under. The
// ledger row is what makes the event attributable — Validate refuses an
// envelope with no trace link — and system_log rather than audit_log because
// reading a Brief mutates no record, which is the line LogSystem's own contract
// draws.
const briefOpenedAction = "brief.opened"

// emitBriefOpened records that this rep read this run.
//
// Inside the caller's transaction, which is the read's OWN transaction rather
// than a second one opened to write telemetry: LatestRun already writes there
// (expired snoozes resurface on read), so the counts below are the ones the rep
// is about to be shown and the event cannot describe a page that failed to
// render. A telemetry write that could commit while the read it reports rolled
// back would report opens that never happened.
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
