// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package auditattribution resolves who an audit row is attributed to: the
// actor, the human a machine acted for, and the tool it was typed through.
// Every surface that names who acted resolves those names through the joins
// here. So no two surfaces can name a different actor for one row.
package auditattribution

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ActorNameColumn answers "who acted" over ActorNameJoins. Only one of the two
// joins can match a row, because actor_type decides which. So the coalesce
// picks the name that exists rather than choosing between two.
const ActorNameColumn = `COALESCE(actor_user.display_name, buyer_participant.full_name)`

// ActorNameJoins resolves the human or Deal Room participant who acted, and
// the human a machine acted for (`obo`). The audit row is aliased `a`.
//
// Each key is built from the joined row ('human:' || id), not by casting
// actor_id. So a machine id resolves to no name instead of a cast error. All
// three are LEFT joins on a primary key, so no join can drop or double a row.
const ActorNameJoins = `
		LEFT JOIN app_user actor_user
		  ON a.actor_type = 'human' AND a.actor_id = 'human:' || actor_user.id::text
		LEFT JOIN deal_room_participant buyer_participant
		  ON a.actor_type = 'buyer' AND a.actor_id = 'buyer:' || buyer_participant.id::text
		LEFT JOIN app_user obo ON obo.id = a.on_behalf_of`

// AgentClientNameJoin resolves the name of the tool a delegated change was
// typed through ("Claude", "Cursor") from the row's passport, as
// `oc.client_name`.
//
// Every hop may be null: a passport minted in Settings has no grant, and a row
// with no passport was not delegated. Each of these reads "via an agent".
const AgentClientNameJoin = `
		LEFT JOIN passport p ON p.id = a.passport_id
		LEFT JOIN oauth_grant g ON g.id = p.oauth_grant_id
		LEFT JOIN oauth_client oc ON oc.client_id = g.client_id`

// Attribution is who an audit row is attributed to.
type Attribution struct {
	ActorType      string
	ActorID        string
	ActorName      *string
	OnBehalfOf     *ids.UUID
	OnBehalfOfName *string
	PassportID     *ids.UUID
	AgentClient    *string
}

// Wire carries the attribution onto the contract, so every surface that names
// who acted spells it the same way.
func (a Attribution) Wire() *crmcontracts.PrincipalAttribution {
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

// FirstCreateSQL is the create audit row a record's creator is read from. It is
// the earliest, with the id breaking a tie, so a replayed create never changes
// who made the record. Both arguments are SQL expressions, never request text.
func FirstCreateSQL(entityType, entityID string) string {
	return `SELECT c.id, c.actor_type, c.actor_id, c.on_behalf_of, c.passport_id, c.occurred_at
		  FROM audit_log c
		 WHERE c.entity_type = ` + entityType + ` AND c.entity_id = ` + entityID + ` AND c.action = 'create'
		 ORDER BY c.occurred_at, c.id LIMIT 1`
}

// CreatorsTx is who created each named record of one type, read from
// FirstCreateSQL. A record with no surviving create row is absent from the
// result, and a caller reads that as unknown, never as the system. The caller
// has already read the records; this admits nothing on its own.
func CreatorsTx(ctx context.Context, tx pgx.Tx, entityType string, entityIDs []ids.UUID) (map[ids.UUID]Attribution, error) {
	out := make(map[ids.UUID]Attribution, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT named.id, a.actor_type, a.actor_id, a.on_behalf_of, a.passport_id,
		       `+ActorNameColumn+`, obo.display_name, oc.client_name
		FROM unnest($2::uuid[]) AS named(id)
		CROSS JOIN LATERAL (`+FirstCreateSQL("$1", "named.id")+`) a`+ActorNameJoins+AgentClientNameJoin,
		entityType, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("auditattribution: reading creators: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entityID ids.UUID
		var who Attribution
		if err := rows.Scan(&entityID, &who.ActorType, &who.ActorID, &who.OnBehalfOf, &who.PassportID,
			&who.ActorName, &who.OnBehalfOfName, &who.AgentClient); err != nil {
			return nil, fmt.Errorf("auditattribution: reading a creator: %w", err)
		}
		out[entityID] = who
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auditattribution: reading creators: %w", err)
	}
	return out, nil
}

// StampCreators sets on each row of a page who created it, in one statement.
// A withheld row gets none: whose agent wrote content the reader may not see,
// and through which tool, is part of that content.
func StampCreators[T any](ctx context.Context, tx pgx.Tx, entityType string, page []T,
	id func(T) ids.UUID, withheld func(T) bool, set func(*T, *crmcontracts.PrincipalAttribution),
) error {
	readable := make([]ids.UUID, 0, len(page))
	for i := range page {
		if !withheld(page[i]) {
			readable = append(readable, id(page[i]))
		}
	}
	creators, err := CreatorsTx(ctx, tx, entityType, readable)
	if err != nil {
		return err
	}
	for i := range page {
		if creator, found := creators[id(page[i])]; found {
			set(&page[i], creator.Wire())
		}
	}
	return nil
}

// NoneWithheld is the withheld answer for a record type a reader sees whole or
// not at all.
func NoneWithheld[T any](T) bool { return false }
