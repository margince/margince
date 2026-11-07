// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// HistoricalAuditPage counts changes and the inspected keys for cursor progress.
type HistoricalAuditPage struct {
	Scanned []ids.UUID
	Changed int
}

// RedactHistoricalAuditPageTx advances over the audit primary key, including
// ordinary writes. The run ID bounds the walk before newly written certificates.
func RedactHistoricalAuditPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, country CountryReader) (HistoricalAuditPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return HistoricalAuditPage{}, err
	}
	floor, err := ResolveFloorPolicy(ctx, tx, country)
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id, entity_type, entity_id,
	 action IN ('erase','anonymize','expire','release') AND before IS NULL AND after IS NULL
	 AND (evidence ? 'cause' OR evidence ? 'retention_action' OR evidence ? 'emails_suppressed')
	 FROM audit_log WHERE id > @after AND id < @until ORDER BY id LIMIT @limit`,
		pgx.StrictNamedArgs{"after": after, "until": until, "limit": limit})
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	type entry struct {
		id, entity ids.UUID
		kind       string
		privacy    *bool
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (entry, error) {
		var e entry
		err := row.Scan(&e.id, &e.kind, &e.entity, &e.privacy)
		return e, err
	})
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	page := HistoricalAuditPage{}
	for _, e := range entries {
		page.Scanned = append(page.Scanned, e.id)
		if e.privacy == nil || !*e.privacy {
			continue
		}
		changed, err := redactHistoricalRecord(ctx, tx, e.id, e.kind, e.entity, floor)
		if err != nil {
			return HistoricalAuditPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

func redactHistoricalRecord(ctx context.Context, tx pgx.Tx, original ids.UUID, kind string, entity ids.UUID, floor FloorPolicy) (bool, error) {
	kept, err := historicalAuditUnderHold(ctx, tx, original, kind, entity, floor)
	if err != nil || kept {
		return false, err
	}
	var needed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_log history, audit_log certificate
	 WHERE certificate.id = @original AND history.entity_type = certificate.entity_type
	 AND history.entity_id = certificate.entity_id
	 AND (history.occurred_at, history.id) <= (certificate.occurred_at, certificate.id)
	 AND (history.before IS NOT NULL OR history.after IS NOT NULL
	      OR history.evidence IS DISTINCT FROM audit_redacted_evidence(
          history.action IN ('erase','anonymize','expire','release','pin','restrict')
          AND history.before IS NULL AND history.after IS NULL, history.evidence, history.action)))
	 OR EXISTS (SELECT 1 FROM event_outbox retained, audit_log certificate
	 WHERE certificate.id = @original AND retained.created_at <= certificate.occurred_at
	 AND retained.envelope->'entity'->>'type' = certificate.entity_type
	 AND retained.envelope->'entity'->>'id' = certificate.entity_id::text
	 AND retained.envelope->'trace'->>'audit_log_id' <> certificate.id::text)`,
		pgx.StrictNamedArgs{"original": original}).Scan(&needed); err != nil {
		return false, err
	}
	if !needed {
		return false, nil
	}
	certificate, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "audit_log", original, nil,
		map[string]any{"payload_redaction": true})
	if err != nil {
		return false, err
	}
	return true, storekit.RedactAuditHistoryThrough(ctx, tx, certificate, original)
}

func historicalAuditUnderHold(ctx context.Context, tx pgx.Tx, original ids.UUID, kind string, entity ids.UUID, floor FloorPolicy) (bool, error) {
	var held bool
	var err error
	entityArgs := pgx.StrictNamedArgs{"entity": entity}
	switch kind {
	case contactObject:
		err = tx.QueryRow(ctx, `SELECT legal_hold FROM contact WHERE id=@entity FOR UPDATE`, entityArgs).Scan(&held)
	case "lead":
		err = tx.QueryRow(ctx, `SELECT legal_hold FROM lead WHERE id=@entity FOR UPDATE`, entityArgs).Scan(&held)
	case "data_subject_request":
		err = tx.QueryRow(ctx, `SELECT dispute_open FROM data_subject_request WHERE id=@entity FOR UPDATE`, entityArgs).Scan(&held)
	case "privacy_request_proof":
		err = tx.QueryRow(ctx, `SELECT d.dispute_open FROM data_subject_request d
   JOIN audit_log certificate ON certificate.evidence->>'request_id'=d.id::text
   WHERE certificate.id=@original FOR UPDATE OF d`, pgx.StrictNamedArgs{"original": original}).Scan(&held)
	case "activity":
		current, carried := floor.sqlArgs()
		args := []any{entity, current, carried}
		err = tx.QueryRow(ctx, `SELECT a.restricted_at IS NOT NULL OR `+
			retentionscope.HeldThroughAnyLink("a.id")+` OR (`+handelsbriefShielded(len(args)-1, len(args))+`)
			 FROM activity a WHERE a.id=`+storekit.SQLf("$%d", len(args)-2)+` FOR UPDATE`, args...).Scan(&held)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return held, err
}

// CountHistoricalAuditRows bounds planning work at one row past the estimate cap.
func CountHistoricalAuditRows(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM audit_log LIMIT @maximum) bounded`, pgx.StrictNamedArgs{"maximum": maximum + 1}).Scan(&count)
	return count, err
}
