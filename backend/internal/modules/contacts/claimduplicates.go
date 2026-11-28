// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An archived claim is a duplicate the evidence index retired: its kept twin
// carries what a human did, and every reader hides the archived one.
const archivedClaimScope = `archived_at IS NOT NULL`

// DeleteArchivedClaimsTx inspects the claims after the cursor in key order and
// deletes the archived duplicates among them. Each delete is audited on its
// contact and announced as erased, as a claim erasure is.
func DeleteArchivedClaimsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, `+archivedClaimScope+` FROM conversation_claim
		WHERE id > @after ORDER BY id LIMIT @batchLimit`, pgx.NamedArgs{afterCursorArg: after, "batchLimit": limit})
	if err != nil {
		return nil, 0, fmt.Errorf("page the claims to inspect: %w", err)
	}
	type inspectedClaim struct {
		id       ids.UUID
		archived bool
	}
	claims, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (inspectedClaim, error) {
		var claim inspectedClaim
		err := row.Scan(&claim.id, &claim.archived)
		return claim, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("page the claims to inspect: %w", err)
	}
	inspected := make([]ids.UUID, 0, len(claims))
	deleted := 0
	for _, claim := range claims {
		inspected = append(inspected, claim.id)
		if !claim.archived {
			continue
		}
		gone, err := deleteArchivedClaim(ctx, tx, claim.id)
		if err != nil {
			return nil, 0, err
		}
		if gone {
			deleted++
		}
	}
	return inspected, deleted, nil
}

// deleteArchivedClaim reports false when the claim is no longer an archived
// row, so a replay writes no second audit row. The contact comes from the
// delete itself, so a merge that moved the claim is audited on the survivor.
func deleteArchivedClaim(ctx context.Context, tx pgx.Tx, claimID ids.UUID) (bool, error) {
	var contactID ids.UUID
	err := tx.QueryRow(ctx, `DELETE FROM conversation_claim WHERE id = $1 AND `+archivedClaimScope+` RETURNING contact_id`,
		claimID).Scan(&contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("delete the archived claim: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "contact", contactID,
		map[string]any{claimIDKey: claimID.String()}, map[string]any{claimIDKey: claimID.String(), "claim_erased": true})
	if err != nil {
		return false, fmt.Errorf("audit the deleted claim: %w", err)
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, contactID, crmcontracts.PublicEventConversationClaimChanged{
		ClaimId: openapi_types.UUID(claimID), Status: "erased",
	})
}

// CountArchivedClaims counts the archived claims still stored, up to the cap
// plus one.
func CountArchivedClaims(ctx context.Context, q datatask.Querier, limit int64) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM conversation_claim WHERE `+archivedClaimScope+`
		LIMIT @estimateCap) archived`, pgx.NamedArgs{"estimateCap": limit + 1}).Scan(&count)
	return count, err
}
