// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Applying reviewed mappings: the page of lost deals a decision would change,
// and the write that carries the decided code onto each one.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReviewedLossMapping is one decision an administrator saw when they applied
// the mappings, at the version they saw.
type ReviewedLossMapping struct {
	ID      ids.UUID `json:"id"`
	Version int64    `json:"version"`
}

// LossMappingVersionsTx answers the current version of each named mapping
// that still exists, so a review can be checked against what is stored now.
func LossMappingVersionsTx(ctx context.Context, tx pgx.Tx, mappings []ids.UUID) (map[ids.UUID]int64, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, version FROM deal_loss_reason_mapping WHERE id = ANY($1::uuid[])`, mappings)
	if err != nil {
		return nil, fmt.Errorf("read loss reason mappings: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ReviewedLossMapping])
	out := make(map[ids.UUID]int64, len(found))
	for _, m := range found {
		out[m.ID] = m.Version
	}
	return out, err
}

// lossMappingCandidate is one lost deal the apply step inspects, with the
// mapping its note matches, if any.
type lossMappingCandidate struct {
	id             ids.UUID
	version        int64
	code, basis    *string
	mapping        ids.UUID
	mappedKey      string
	mappingVersion int64
}

// ApplyLossReasonMappingsTx writes the reviewed mappings onto one page of the
// lost deals they would change, after the cursor. A mapping whose version
// moved since the review is left for the next one.
func ApplyLossReasonMappingsTx(
	ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, reviewed map[ids.UUID]int64,
) (scanned []ids.UUID, changed int, err error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, 0, err
	}
	if len(reviewed) == 0 {
		return nil, 0, nil
	}
	page, err := lossMappingCandidates(ctx, tx, after, limit, slices.Collect(maps.Keys(reviewed)))
	if err != nil {
		return nil, 0, err
	}
	for _, row := range page {
		scanned = append(scanned, row.id)
		if !row.mapsAnew(reviewed) {
			continue
		}
		wrote, err := writeMappedLossReason(ctx, tx, row)
		if err != nil {
			return nil, 0, err
		}
		if wrote {
			changed++
		}
	}
	return scanned, changed, nil
}

func (row lossMappingCandidate) mapsAnew(reviewed map[ids.UUID]int64) bool {
	version, ok := reviewed[row.mapping]
	return ok && version == row.mappingVersion
}

// pendingMappingFrom is the live lost deals a decided mapping would change.
// That is one with no code, or one an earlier mapping coded with a key the
// decision no longer names. A deal holding its mapped code drops out, so a rerun finds
// nothing it did. $1 limits it to the reviewed mappings, or NULL for every one.
var pendingMappingFrom = `FROM deal d
	JOIN deal_loss_reason_mapping m ON m.note_key = ` + legacyNoteKeySQL("d.lost_reason") + `
	WHERE ` + mappableNoteSQL + ` AND ($1::uuid[] IS NULL OR m.id = ANY($1::uuid[]))
	  AND (d.lost_reason_code IS NULL OR d.lost_reason_code <> m.reason_key)`

// pendingMappingsOf is pendingMappingFrom narrowed to the deals the caller
// may see, with its arguments: neither a count nor the apply reaches beyond
// them. It asks the deal read gate first, since a scope clause is no gate.
func pendingMappingsOf(ctx context.Context, mappings []ids.UUID) (string, []any, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return "", nil, err
	}
	args := []any{mappings}
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return "", nil, err
	}
	if scope == "" {
		scope = predicateAlways
	}
	return pendingMappingFrom + ` AND (` + scope + `)`, args, nil
}

func lossMappingCandidates(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, mappings []ids.UUID) ([]lossMappingCandidate, error) {
	from, args, err := pendingMappingsOf(ctx, mappings)
	if err != nil {
		return nil, err
	}
	args = append(args, after, limit)
	// The mapping row is held FOR SHARE beside the deal. A re-decision waits
	// for this batch, or committed before it and reads here at its new
	// version, which mapsAnew refuses as unreviewed.
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT d.id, d.version, d.lost_reason_code, d.lost_reason_basis,
			m.id, m.reason_key, m.version
		  `+from+` AND d.id > $%d
		 ORDER BY d.id LIMIT $%d FOR UPDATE OF d FOR SHARE OF m`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("read lost deals to map: %w", err)
	}
	defer rows.Close()
	var page []lossMappingCandidate
	for rows.Next() {
		var row lossMappingCandidate
		if err := rows.Scan(&row.id, &row.version, &row.code, &row.basis,
			&row.mapping, &row.mappedKey, &row.mappingVersion); err != nil {
			return nil, err
		}
		page = append(page, row)
	}
	return page, rows.Err()
}

// writeMappedLossReason is the mapped half of the deal write shape: the row,
// its audit entry and its deal.updated event, in the caller's transaction. A
// deal the operator may not write is left as it is.
func writeMappedLossReason(ctx context.Context, tx pgx.Tx, row lossMappingCandidate) (bool, error) {
	err := auth.EnsureWritable(ctx, tx, dealTable, row.id)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE deal SET lost_reason_code = $2, lost_reason_basis = 'mapped'
		WHERE id = $1 AND version = $3 AND archived_at IS NULL`, row.id, row.mappedKey, row.version)
	if err != nil {
		return false, fmt.Errorf("map a lost deal's reason: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	before := map[string]any{lossReasonCodeField: row.code, lossBasisField: row.basis}
	after := map[string]any{lossReasonCodeField: row.mappedKey, lossBasisField: lossBasisMapped}
	// The decision the code came from, so the deal's history names it.
	evidence := map[string]any{"loss_reason_mapping_id": row.mapping.String(), "loss_reason_mapping_version": row.mappingVersion}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "deal", row.id, before, after, evidence)
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, row.id, crmcontracts.PublicEventDealUpdated{ChangedFields: after})
}

// CountLossMappingCandidates counts, up to limit+1, the lost deals the caller
// may see that the given mappings would change; nil counts every decision.
func CountLossMappingCandidates(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64, mappings []ids.UUID,
) (int64, error) {
	from, args, err := pendingMappingsOf(ctx, mappings)
	if err != nil {
		return 0, err
	}
	args = append(args, limit+1)
	var count int64
	err = q.QueryRow(ctx, storekit.SQLf(`SELECT count(*) FROM (SELECT 1 `+from+` LIMIT $%d) pending`, len(args)),
		args...).Scan(&count)
	return count, err
}
