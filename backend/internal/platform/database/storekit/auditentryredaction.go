// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RedactHistoricalAuditEntry applies a current correction proof to one old row.
// It creates no erasure boundary on the domain record's later history.
func RedactHistoricalAuditEntry(ctx context.Context, tx pgx.Tx, certificate ids.UUID) error {
	var original ids.UUID
	err := tx.QueryRow(ctx, `UPDATE audit_log history
 SET `+redactedAuditAssignment+`
 FROM audit_log certificate
 WHERE certificate.id=@certificate AND certificate.entity_type='audit_log'
 AND certificate.action='update' AND certificate.before IS NULL AND certificate.after IS NULL
 AND certificate.source_xid=pg_current_xact_id()
 AND certificate.evidence='{"payload_redaction":true,"legacy_erased_record":true}'::jsonb
 AND history.id=certificate.entity_id RETURNING history.id`,
		pgx.StrictNamedArgs{"certificate": certificate}).Scan(&original)
	if err != nil {
		return fmt.Errorf("store: redacting a certified historical audit entry: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox WHERE envelope->'trace'->>'audit_log_id'=@original::uuid::text`, pgx.StrictNamedArgs{"original": original}); err != nil {
		return fmt.Errorf("store: cancelling events of a redacted historical entry: %w", err)
	}
	return nil
}
