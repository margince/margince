// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attributionOf is the one decode of a projected audit row into attribution.
func attributionOf(row recordAuditRow) auditattribution.Attribution {
	return auditattribution.Attribution{
		ActorType:      row.actorType,
		ActorID:        row.actorID,
		ActorName:      row.actorDisplayName,
		OnBehalfOf:     row.onBehalfOf,
		OnBehalfOfName: row.onBehalfOfName,
		PassportID:     row.passportID,
		AgentClient:    row.agentClientName,
	}
}

// AuditAttributionsTx resolves the attribution of the given audit rows in one
// query, through the projection the record History uses. Only rows written for
// the named entity count. The stored link has no foreign key, so a link to
// another record's audit row must read as unknown. An id with no matching
// audit row is absent from the result. The caller reads that as unknown, not
// as the system.
func AuditAttributionsTx(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, auditIDs []ids.UUID) (map[ids.UUID]auditattribution.Attribution, error) {
	out := make(map[ids.UUID]auditattribution.Attribution, len(auditIDs))
	if len(auditIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT `+recordAuditColumns+`
		FROM audit_log a`+auditattribution.ActorNameJoins+auditattribution.AgentClientNameJoin+`
		WHERE a.id = ANY($1) AND a.entity_type = $2 AND a.entity_id = $3`, auditIDs, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("privacy: reading audit attributions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row recordAuditRow
		if err := scanRecordAuditRow(rows, &row); err != nil {
			return nil, fmt.Errorf("privacy: reading an audit attribution: %w", err)
		}
		out[row.id] = attributionOf(row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("privacy: reading audit attributions: %w", err)
	}
	return out, nil
}
