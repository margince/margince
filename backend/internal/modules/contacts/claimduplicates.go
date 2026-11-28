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
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// An archived claim is a duplicate the evidence index retired: its kept twin
// carries what a human did, and every reader hides the archived one.
const archivedClaimScope = `archived_at IS NOT NULL`

// ArchivedClaimsDataTask deletes the duplicate claims migration 1791101875
// archived, so a later release can drop the mark.
func ArchivedClaimsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795851263_archived_claims_deleted", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795851263_archived_claims_deleted"),
		Requires: []string{"core/1791101875"},
		Steps: []datatask.Step{{
			Name: "conversation_claim", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: countArchivedClaims, Batch: deleteArchivedClaimsBatch, EmptyWhenDone: true,
		}},
	}
}

// deleteArchivedClaimsBatch inspects the claims after the cursor in key order
// and deletes the archived duplicates among them.
func deleteArchivedClaimsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id, `+archivedClaimScope+` FROM conversation_claim
		WHERE id > @after ORDER BY id LIMIT @batchLimit`, pgx.NamedArgs{afterCursorArg: after, "batchLimit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("page the claims to inspect: %w", err)
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
		return datatask.BatchOut{}, fmt.Errorf("page the claims to inspect: %w", err)
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
			return datatask.BatchOut{}, err
		}
		if gone {
			deleted++
		}
	}
	return datatask.AdvanceUUID(in, inspected, deleted)
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
	// The image and event match privacy's eraseCommitmentClaims. A module may
	// not import privacy, and eventtypeownership_test.go names privacy as the second emitter.
	auditID, err := storekit.Audit(ctx, tx, "update", "contact", contactID, map[string]any{claimIDKey: claimID},
		map[string]any{claimIDKey: claimID, "claim_erased": true})
	if err != nil {
		return false, fmt.Errorf("audit the deleted claim: %w", err)
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, contactID, crmcontracts.PublicEventConversationClaimChanged{
		ClaimId: openapi_types.UUID(claimID), Status: "erased",
	})
}

// countArchivedClaims counts the archived claims still stored, up to the cap
// plus one.
func countArchivedClaims(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM conversation_claim WHERE `+archivedClaimScope+`
		LIMIT @estimateCap) archived`, pgx.NamedArgs{"estimateCap": maximum + 1}).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("contacts: counting the archived claims: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}
