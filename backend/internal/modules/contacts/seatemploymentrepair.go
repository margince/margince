// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatEmploymentPage is one page of live employment edges, in id order.
type SeatEmploymentPage struct {
	Scanned []ids.UUID
	Retired int
}

// RetireSeatEmploymentEdgesTx archives the inferred employment edges on this
// page whose contact holds a live seat's login address. A login changes only
// with the seat's own account, unlike an alias or a mailbox label.
//
// Inferred means the create audit names capture or a provider as its origin.
// Every audit row on the edge must also be by a system or connector principal.
// An archived edge keeps its row and audit trail; a human records it again.
func RetireSeatEmploymentEdgesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID string) (SeatEmploymentPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return SeatEmploymentPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return SeatEmploymentPage{}, err
	}
	page, retirable, err := inferredEmploymentPage(ctx, tx, after, limit)
	if err != nil {
		return SeatEmploymentPage{}, err
	}
	owners := make([]ids.ContactID, len(retirable))
	for i, edge := range retirable {
		owners[i] = edge.contact
	}
	seats, err := seatContactsMatchingTx(ctx, tx, owners, func(seats storekit.SeatAddresses, address string) bool {
		return seats.IsLogin(address)
	})
	if err != nil {
		return SeatEmploymentPage{}, err
	}
	for _, edge := range retirable {
		if !seats[edge.contact] {
			continue
		}
		retired, err := retireSeatEmploymentEdge(ctx, tx, edge, runID)
		if err != nil {
			return SeatEmploymentPage{}, err
		}
		if retired {
			page.Retired++
		}
	}
	return page, nil
}

type inferredEdge struct {
	id      ids.UUID
	contact ids.ContactID
	version int64
}

// inferredEmploymentPage reads one page and the edges on it the repair may
// retire. Both audit tests read idx_audit_entity_narrow.
func inferredEmploymentPage(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (SeatEmploymentPage, []inferredEdge, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.contact_id, r.version,
		       EXISTS (SELECT 1 FROM audit_log a
		                WHERE a.entity_type = $3 AND a.entity_id = r.id AND a.action = 'create'
		                  AND a.after ->> $4 IN ($5, $6))
		       AND NOT EXISTS (SELECT 1 FROM audit_log a
		                WHERE a.entity_type = $3 AND a.entity_id = r.id
		                  AND a.actor_type NOT IN ('system', 'connector'))
		  FROM relationship r
		 WHERE r.kind = 'employment' AND r.archived_at IS NULL AND r.id > $1
		 ORDER BY r.id
		 LIMIT $2`,
		after, limit, tableRelationship, auditKeyOrigin, relationshipOriginCapture, relationshipOriginProvider)
	if err != nil {
		return SeatEmploymentPage{}, nil, fmt.Errorf("contacts: paging employment edges for the seat repair: %w", err)
	}
	defer rows.Close()
	var page SeatEmploymentPage
	var retirable []inferredEdge
	for rows.Next() {
		var edge inferredEdge
		var inferred bool
		if err := rows.Scan(&edge.id, &edge.contact, &edge.version, &inferred); err != nil {
			return SeatEmploymentPage{}, nil, fmt.Errorf("contacts: reading employment edges for the seat repair: %w", err)
		}
		page.Scanned = append(page.Scanned, edge.id)
		if inferred {
			retirable = append(retirable, edge)
		}
	}
	if err := rows.Err(); err != nil {
		return SeatEmploymentPage{}, nil, fmt.Errorf("contacts: paging employment edges for the seat repair: %w", err)
	}
	return page, retirable, nil
}

// retireSeatEmploymentEdge archives one edge under its contact's employment
// lock, which the seat check came before (lockorder.go). An edge edited since
// the page was read has another version and stays.
func retireSeatEmploymentEdge(ctx context.Context, tx pgx.Tx, edge inferredEdge, runID string) (bool, error) {
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, edge.contact.String()); err != nil {
		return false, err
	}
	archived, err := scanRelationship(tx.QueryRow(ctx, `
		UPDATE relationship SET archived_at = now(), is_current_primary = false
		 WHERE id = $1 AND version = $2 AND archived_at IS NULL
		 RETURNING `+relationshipColumns, edge.id, edge.version))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: retiring a seat's inferred employment: %w", err)
	}
	evidence := map[string]any{"data_task_run": runID, auditKeyReason: "workspace_seat_address"}
	if err := emitRelationshipChangeWithEvidence(ctx, tx, "archive", nil, archived, evidence); err != nil {
		return false, fmt.Errorf("contacts: recording a retired seat employment: %w", err)
	}
	return true, nil
}

// CountLiveEmploymentEdges counts, up to limit+1, the edges the repair pages.
func CountLiveEmploymentEdges(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM relationship WHERE kind = 'employment' AND archived_at IS NULL LIMIT $1) edges`,
		limit+1).Scan(&count)
	return count, err
}
