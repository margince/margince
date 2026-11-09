// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type historyImport struct {
	id            ids.UUID
	active        bool
	additiveLinks bool
	sourceXID     string
	addedContact  ids.UUID
}

func historyImportForAuditTx(ctx context.Context, tx pgx.Tx, audit ids.UUID) (*historyImport, error) {
	if audit == ids.Nil {
		return nil, nil //nolint:nilnil // No history attribution is a live event, not a lookup failure.
	}
	args := []any{audit}
	var run historyImport
	var status string
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT b.id,b.status,COALESCE(a.source_xid::text,''),a.entity_id,`+contacts.CohortAuditSQL("a")+` FROM capture_backfill b JOIN audit_log a ON a.batch_id=b.id
 WHERE a.id=$%d FOR SHARE OF b`, len(args)), args...).Scan(&run.id, &status, &run.sourceXID, &run.addedContact, &run.additiveLinks)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No history attribution is a live event, not a lookup failure.
	}
	if err != nil {
		return nil, err
	}
	run.active = status == voiceRowQueued || status == "running"
	return &run, nil
}

// NewGraphEdgeConsumer shares history attribution with signature scheduling and the existing exact-fold engine.
func NewGraphEdgeConsumer(pool *pgxpool.Pool) *search.GraphEdgeGen {
	return search.NewGraphEdgeGen(search.NewStore(InstallationDB(pool))).WithHistoryResolver(graphHistoryForEventTx)
}

func graphHistoryForEventTx(ctx context.Context, tx pgx.Tx, env events.Envelope) (*search.HistoryImport, error) {
	run, err := historyImportForEventTx(ctx, tx, env)
	if err == nil && run != nil {
		source, sourceErr := derivedAuditSourceTx(ctx, tx, env.Trace.AuditLogID)
		err = sourceErr
		run.sourceXID = source.xid
		run.additiveLinks = source.addedContact != nil
		if source.addedContact != nil {
			run.addedContact = *source.addedContact
		}
	}
	if err != nil || run == nil {
		return nil, err
	}
	deferred, err := historyDeferRunTx(ctx, tx, run)
	if err != nil {
		return nil, err
	}
	runID := run.id
	if deferred != nil {
		runID = *deferred
	}
	return &search.HistoryImport{RunID: runID, Active: deferred != nil, AdditiveLinks: run.additiveLinks, SourceXID: run.sourceXID, AddedContact: run.addedContact}, nil
}

func historyImportForActivityTx(ctx context.Context, tx pgx.Tx, activity ids.UUID) (*historyImport, error) {
	args := []any{activity}
	var audit ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT id FROM audit_log WHERE entity_type='activity' AND entity_id=$%d
 AND action='create' AND batch_id IS NOT NULL ORDER BY occurred_at,id LIMIT 1`, len(args)), args...).Scan(&audit)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No history attribution is a live event, not a lookup failure.
	}
	if err != nil {
		return nil, err
	}
	return historyImportForAuditTx(ctx, tx, audit)
}

type derivedAuditSource struct {
	xid          string
	addedContact *ids.UUID
}

func derivedAuditSourceTx(ctx context.Context, tx pgx.Tx, audit ids.UUID) (derivedAuditSource, error) {
	var source derivedAuditSource
	if audit == ids.Nil {
		return source, nil
	}
	args := []any{audit}
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT COALESCE(a.source_xid::text,''),
 CASE WHEN `+contacts.CohortAuditSQL("a")+` THEN a.entity_id END FROM audit_log a WHERE a.id=$%d`, len(args)), args...).Scan(&source.xid, &source.addedContact)
	if errors.Is(err, pgx.ErrNoRows) {
		return source, nil
	}
	return source, err
}

// Historical work waits for the active import cohort; live work has its own generation.
func historyDeferRunTx(ctx context.Context, tx pgx.Tx, source *historyImport) (*ids.UUID, error) {
	if source != nil && source.active {
		return &source.id, nil
	}
	var run ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM capture_backfill WHERE status IN ('queued','running') ORDER BY created_at,id LIMIT 1 FOR SHARE`).Scan(&run)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No history attribution is a live event, not a lookup failure.
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func historyImportForEventTx(ctx context.Context, tx pgx.Tx, env events.Envelope) (*historyImport, error) {
	if env.Entity.Type == string(recordTypeActivity) {
		if env.Type == eventActivityUpdated && env.Actor.Type != string(principal.PrincipalSystem) {
			return nil, nil //nolint:nilnil // No history attribution is a live event, not a lookup failure.
		}
		return historyImportForActivityTx(ctx, tx, env.Entity.ID)
	}
	return historyImportForAuditTx(ctx, tx, env.Trace.AuditLogID)
}
