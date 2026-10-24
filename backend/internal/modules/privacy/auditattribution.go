// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AuditAttribution is who an audit row is attributed to: the actor, the human
// whose authority a machine acted under, and the tool it was typed through.
// The record History reads these fields from the same row through
// attributionOf, so a surface that carries a stored audit link cannot read
// them differently.
type AuditAttribution struct {
	ActorType      string
	ActorID        string
	ActorName      *string
	OnBehalfOf     *ids.UUID
	OnBehalfOfName *string
	PassportID     *ids.UUID
	AgentClient    *string
}

// attributionOf is the one decode of a projected audit row into attribution.
func attributionOf(row recordAuditRow) AuditAttribution {
	return AuditAttribution{
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
// query, through the projection the record History uses. An id with no audit
// row is absent from the result; the caller reads that as unknown, not as the
// system.
func AuditAttributionsTx(ctx context.Context, tx pgx.Tx, auditIDs []ids.UUID) (map[ids.UUID]AuditAttribution, error) {
	out := make(map[ids.UUID]AuditAttribution, len(auditIDs))
	if len(auditIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT `+recordAuditColumns+`
		FROM audit_log a`+auditActorNameJoins+agentClientNameJoin+`
		WHERE a.id = ANY($1)`, auditIDs)
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
