// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AuditAttribution is who an audit row is attributed to. It holds the actor,
// the human whose authority a machine acted under, and the tool it was typed
// through.
// The record History reads these fields from the same row through
// attributionOf. A surface that carries a stored audit link therefore cannot
// read them differently.
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

// Wire carries the attribution onto the contract, so every surface that names
// who acted spells it the same way.
func (a AuditAttribution) Wire() *crmcontracts.PrincipalAttribution {
	out := &crmcontracts.PrincipalAttribution{
		ActorType:      crmcontracts.PrincipalAttributionActorType(a.ActorType),
		ActorId:        a.ActorID,
		ActorName:      a.ActorName,
		OnBehalfOfName: a.OnBehalfOfName,
		AgentClient:    a.AgentClient,
	}
	if a.OnBehalfOf != nil {
		id := openapi_types.UUID(*a.OnBehalfOf)
		out.OnBehalfOf = &id
	}
	if a.PassportID != nil {
		id := openapi_types.UUID(*a.PassportID)
		out.PassportId = &id
	}
	return out
}

// CreationAttributionTx is who created the record: its earliest create audit
// row, read through the same projection as AuditAttributionsTx. Nil when no
// create row survives, which the caller reads as unknown, not as the system.
// The caller has already read the record, the gate the record History holds.
func CreationAttributionTx(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) (*AuditAttribution, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+recordAuditColumns+`
		FROM audit_log a`+auditActorNameJoins+agentClientNameJoin+`
		WHERE a.entity_type = $1 AND a.entity_id = $2 AND a.action = 'create'
		ORDER BY a.occurred_at, a.id
		LIMIT 1`, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("privacy: reading a creation attribution: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("privacy: reading a creation attribution: %w", err)
		}
		return nil, nil
	}
	var row recordAuditRow
	if err := scanRecordAuditRow(rows, &row); err != nil {
		return nil, fmt.Errorf("privacy: reading a creation attribution: %w", err)
	}
	attribution := attributionOf(row)
	return &attribution, nil
}

// AuditAttributionsTx resolves the attribution of the given audit rows in one
// query, through the projection the record History uses. Only rows written for
// the named entity count. The stored link has no foreign key, so a link to
// another record's audit row must read as unknown. An id with no matching
// audit row is absent from the result. The caller reads that as unknown, not
// as the system.
func AuditAttributionsTx(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, auditIDs []ids.UUID) (map[ids.UUID]AuditAttribution, error) {
	out := make(map[ids.UUID]AuditAttribution, len(auditIDs))
	if len(auditIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT `+recordAuditColumns+`
		FROM audit_log a`+auditActorNameJoins+agentClientNameJoin+`
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
