// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RedactAuditHistory removes retained payloads after a privacy certificate was
// written in this transaction. The database keeps audit metadata immutable.
func RedactAuditHistory(ctx context.Context, tx pgx.Tx, certificate ids.UUID) error {
	return RedactAuditHistoryThrough(ctx, tx, certificate, certificate)
}

// RedactAuditHistoryThrough limits an upgrade correction to the original
// certificate. Later edits and their events remain outside that correction.
func RedactAuditHistoryThrough(ctx context.Context, tx pgx.Tx, certificate, through ids.UUID) error {
	args := pgx.StrictNamedArgs{"certificate": certificate, "through": through}
	var certified bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_log certificate, audit_log boundary
		WHERE `+auditRedactionScope+`
		  AND certificate.before IS NULL AND certificate.after IS NULL
		  AND certificate.xmin::text = pg_current_xact_id()::text)`, args).Scan(&certified); err != nil {
		return fmt.Errorf("store: checking the privacy certificate: %w", err)
	}
	if !certified {
		return errors.New("store: payload redaction needs a privacy certificate written in this transaction")
	}
	if _, err := tx.Exec(ctx, `UPDATE audit_log history
		SET before = NULL, after = NULL, evidence = audit_redacted_evidence(
          history.action IN ('erase','anonymize','expire','release')
          AND history.before IS NULL AND history.after IS NULL, history.evidence)
		FROM audit_log certificate, audit_log boundary
		WHERE `+auditRedactionScope+`
		  AND certificate.xmin::text = pg_current_xact_id()::text
		  AND history.entity_type = boundary.entity_type
		  AND history.entity_id = boundary.entity_id
		  `+auditHistoryThroughBoundary+`
		  AND (history.before IS NOT NULL OR history.after IS NOT NULL
		       OR history.evidence IS DISTINCT FROM audit_redacted_evidence(
               history.action IN ('erase','anonymize','expire','release')
               AND history.before IS NULL AND history.after IS NULL, history.evidence))`, args); err != nil {
		return fmt.Errorf("store: redacting certified audit payloads: %w", err)
	}
	return cancelRedactedEvents(ctx, tx, args)
}

func cancelRedactedEvents(ctx context.Context, tx pgx.Tx, args pgx.StrictNamedArgs) error {
	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox retained
		USING audit_log certificate, audit_log boundary, audit_log history
		WHERE `+auditRedactionScope+`
		  AND history.entity_type = boundary.entity_type
		  AND history.entity_id = boundary.entity_id
		  AND history.id <> certificate.id
		  `+auditHistoryThroughBoundary+`
		  AND retained.envelope->'trace'->>'audit_log_id' = history.id::text`, args); err != nil {
		return fmt.Errorf("store: cancelling events linked to redacted audit records: %w", err)
	}
	_, err := tx.Exec(ctx, `DELETE FROM event_outbox retained
		USING audit_log certificate, audit_log boundary
		WHERE `+auditRedactionScope+`
		  AND certificate.xmin::text = pg_current_xact_id()::text
		  AND retained.envelope->'entity'->>'type' = boundary.entity_type
		  AND retained.envelope->'entity'->>'id' = boundary.entity_id::text
		  AND retained.envelope->'trace'->>'audit_log_id' <> certificate.id::text
		  AND (boundary.id = certificate.id OR retained.created_at <= boundary.occurred_at)`, args)
	if err != nil {
		return fmt.Errorf("store: cancelling certified event payloads: %w", err)
	}
	return nil
}

const auditRedactionScope = `certificate.id = @certificate AND boundary.id = @through
 AND ((certificate.action IN ('erase','anonymize','expire','release')
       AND boundary.entity_type = certificate.entity_type AND boundary.entity_id = certificate.entity_id)
 OR (certificate.action = 'update' AND certificate.entity_type = 'audit_log'
     AND certificate.entity_id = boundary.id AND certificate.evidence = '{"payload_redaction":true}'::jsonb))
 AND boundary.action IN ('erase', 'anonymize', 'expire', 'release')
 AND boundary.before IS NULL AND boundary.after IS NULL`

const auditHistoryThroughBoundary = `AND (boundary.id = certificate.id
 OR (history.occurred_at, history.id) <= (boundary.occurred_at, boundary.id))`
