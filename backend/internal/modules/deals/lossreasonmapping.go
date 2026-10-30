// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The reviewed mapping of legacy loss notes to codes. An administrator decides
// one distinct note at a time; a data task then writes the decided code onto
// the lost deals carrying that note. The note itself is never rewritten, and a
// code the closer chose is never replaced.

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	lossMappingTable = "deal_loss_reason_mapping"
	reasonKeyField   = "reason_key"
	decidedByField   = "decided_by"
	// codeLimitOutOfRange is the refusal of a page size the contract bounds.
	codeLimitOutOfRange = "out_of_range"
	// MaxLegacyLossReasons bounds one page of the legacy list.
	MaxLegacyLossReasons = 200
)

var lossReasonKeyShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// legacyNoteHashSQL is a note's mapping key. The list a human decides from and
// the task that applies the decision both call it, so they key notes alike.
func legacyNoteHashSQL(column string) string {
	return `sha256(convert_to(lower(btrim(` + column + `)), 'UTF8'))`
}

// unmappedLossSQL is a lost deal whose note nobody has given a code yet.
const unmappedLossSQL = `d.status = 'lost' AND d.lost_reason IS NOT NULL AND d.lost_reason_code IS NULL`

// pendingMappingFrom is the live lost deals a decided mapping would change.
// That is one with no code, or one an earlier decision on the note set to a
// code the decision no longer names. A code the closer recorded is never one.
// A deal holding its mapped code drops out, so a rerun finds nothing it did.
// $1 limits it to the reviewed mappings, or NULL for all.
var pendingMappingFrom = `FROM deal d
	JOIN deal_loss_reason_mapping m ON m.text_hash = ` + legacyNoteHashSQL("d.lost_reason") + `
	WHERE d.archived_at IS NULL AND d.status = 'lost' AND d.lost_reason IS NOT NULL
	  AND ($1::uuid[] IS NULL OR m.id = ANY($1::uuid[]))
	  AND (d.lost_reason_code IS NULL
	       OR (d.lost_reason_mapping_id = m.id AND d.lost_reason_code <> m.reason_key))`

// ListLegacyLossReasons answers the most frequent notes of the lost deals the
// caller may read that have no code. Each carries its decision, if taken. Only
// a seat that may decide one may list them.
func (s *Store) ListLegacyLossReasons(ctx context.Context, limit int) (crmcontracts.LegacyLossReasonListResponse, error) {
	out := crmcontracts.LegacyLossReasonListResponse{Data: []crmcontracts.LegacyLossReason{}}
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return out, err
	}
	if limit < 1 || limit > MaxLegacyLossReasons {
		return out, &values.ParseError{Field: "limit", Code: codeLimitOutOfRange, Message: "limit must be between 1 and 200"}
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return out, err
	}
	if scope == "" {
		scope = predicateAlways
	}
	limitAt := arg(limit)
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, storekit.SQLf(`WITH notes AS (
			SELECT `+legacyNoteHashSQL("d.lost_reason")+` AS hash, min(d.lost_reason) AS sample, count(*) AS deals
			  FROM deal d
			 WHERE d.archived_at IS NULL AND `+unmappedLossSQL+` AND (`+scope+`)
			 GROUP BY 1)
			SELECT encode(n.hash, 'hex'), n.sample, n.deals::int, (count(*) OVER ())::int,
			       m.id, m.reason_key, m.decided_by, m.updated_at, m.version
			  FROM notes n LEFT JOIN deal_loss_reason_mapping m ON m.text_hash = n.hash
			 ORDER BY n.deals DESC, n.hash LIMIT $%d`, limitAt), args...)
		if err != nil {
			return fmt.Errorf("list legacy loss notes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			row, total, err := scanLegacyLossReason(rows)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, row)
			out.DistinctTotal = total
		}
		return rows.Err()
	})
	return out, err
}

func scanLegacyLossReason(rows pgx.Rows) (crmcontracts.LegacyLossReason, int, error) {
	var row crmcontracts.LegacyLossReason
	var total int
	var mappingID *ids.UUID
	var reasonKey, decidedBy *string
	var decidedAt *time.Time
	var version *int64
	if err := rows.Scan(&row.TextHash, &row.Text, &row.DealCount, &total,
		&mappingID, &reasonKey, &decidedBy, &decidedAt, &version); err != nil {
		return row, 0, fmt.Errorf("scan legacy loss note: %w", err)
	}
	if mappingID != nil && reasonKey != nil && decidedBy != nil && decidedAt != nil && version != nil {
		row.Mapping = &crmcontracts.LossReasonMapping{
			Id: openapi_types.UUID(*mappingID), TextHash: row.TextHash, ReasonKey: *reasonKey,
			DecidedBy: *decidedBy, DecidedAt: *decidedAt, Version: *version,
		}
	}
	return row, total, nil
}

// DecideLegacyLossReason records which code one legacy note counts under, and
// who decided it. No deal changes here: ApplyLossReasonMappingsTx carries the
// decision to the deals once an administrator applies the reviewed mappings.
func (s *Store) DecideLegacyLossReason(ctx context.Context, textHash, reasonKey string) (crmcontracts.LossReasonMapping, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.LossReasonMapping{}, err
	}
	hash, err := hex.DecodeString(textHash)
	if err != nil || len(hash) != 32 || textHash != strings.ToLower(textHash) {
		return crmcontracts.LossReasonMapping{}, &values.ParseError{Field: "text_hash", Code: "invalid_hash", Message: "text_hash must be 64 lowercase hex digits"}
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.LossReasonMapping{}, err
	}
	key := strings.TrimSpace(reasonKey)
	var out crmcontracts.LossReasonMapping
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureChoosableLossReason(ctx, tx, reasonKeyField, key); err != nil {
			return err
		}
		before, found, err := readLossMappingByHash(ctx, tx, hash)
		if err != nil {
			return err
		}
		if found && before.ReasonKey == key {
			out = before
			return nil
		}
		if err := writeLossMapping(ctx, tx, hash, key, by, before, found); err != nil {
			return err
		}
		out, _, err = readLossMappingByHash(ctx, tx, hash)
		return err
	})
	return out, err
}

// writeLossMapping stores a decision with its audit row and event. A second
// decision on one note replaces the first; the audit row keeps the first.
func writeLossMapping(
	ctx context.Context, tx pgx.Tx, hash []byte, key, by string,
	before crmcontracts.LossReasonMapping, found bool,
) error {
	after := map[string]any{reasonKeyField: key, decidedByField: by}
	if !found {
		id := ids.NewV7()
		_, err := tx.Exec(ctx, `INSERT INTO deal_loss_reason_mapping (id, text_hash, reason_key, decided_by)
			VALUES ($1, $2, $3, $4)`, id, hash, key, by)
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert loss reason mapping: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "create", "deal_loss_reason_mapping", id, nil, after)
		if err != nil {
			return err
		}
		return announceLossReasonChange(ctx, tx, auditID, lossMappingTable, "create", id)
	}
	id := ids.UUID(before.Id)
	tag, err := tx.Exec(ctx, `UPDATE deal_loss_reason_mapping SET reason_key = $2, decided_by = $3
		WHERE id = $1 AND version = $4`, id, key, by, before.Version)
	if err != nil {
		return fmt.Errorf("update loss reason mapping: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrConflict
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "deal_loss_reason_mapping", id,
		map[string]any{reasonKeyField: before.ReasonKey, decidedByField: before.DecidedBy}, after)
	if err != nil {
		return err
	}
	return announceLossReasonChange(ctx, tx, auditID, lossMappingTable, "update", id)
}

func readLossMappingByHash(ctx context.Context, tx pgx.Tx, hash []byte) (crmcontracts.LossReasonMapping, bool, error) {
	var out crmcontracts.LossReasonMapping
	var id ids.UUID
	err := tx.QueryRow(ctx, `SELECT id, encode(text_hash, 'hex'), reason_key, decided_by, updated_at, version
		FROM deal_loss_reason_mapping WHERE text_hash = $1 FOR UPDATE`, hash).
		Scan(&id, &out.TextHash, &out.ReasonKey, &out.DecidedBy, &out.DecidedAt, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, fmt.Errorf("read loss reason mapping: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	return out, true, nil
}

// ReviewedLossMapping is one decision an administrator saw when they applied
// the mappings, at the version they saw.
type ReviewedLossMapping struct {
	ID      ids.UUID `json:"id"`
	Version int64    `json:"version"`
}

// ReviewedLossMappingsTx answers every decided mapping at its current version,
// in id order, up to limit.
func ReviewedLossMappingsTx(ctx context.Context, tx pgx.Tx, limit int) ([]ReviewedLossMapping, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, version FROM deal_loss_reason_mapping ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("read loss reason mappings: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ReviewedLossMapping])
}

// lossMappingCandidate is one lost deal the apply step inspects, with the
// mapping its note matches, if any.
type lossMappingCandidate struct {
	id             ids.UUID
	version        int64
	code, basis    *string
	currentMapping *ids.UUID
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

func lossMappingCandidates(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, mappings []ids.UUID) ([]lossMappingCandidate, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id, d.version, d.lost_reason_code, d.lost_reason_basis, d.lost_reason_mapping_id,
			m.id, m.reason_key, m.version
		  `+pendingMappingFrom+` AND d.id > $2
		 ORDER BY d.id LIMIT $3 FOR UPDATE OF d`, mappings, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read lost deals to map: %w", err)
	}
	defer rows.Close()
	var page []lossMappingCandidate
	for rows.Next() {
		var row lossMappingCandidate
		if err := rows.Scan(&row.id, &row.version, &row.code, &row.basis, &row.currentMapping,
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
	tag, err := tx.Exec(ctx, `UPDATE deal SET lost_reason_code = $2, lost_reason_basis = 'mapped',
		lost_reason_mapping_id = $3 WHERE id = $1 AND version = $4 AND archived_at IS NULL`,
		row.id, row.mappedKey, row.mapping, row.version)
	if err != nil {
		return false, fmt.Errorf("map a lost deal's reason: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	before := map[string]any{lossReasonCodeField: row.code, lossBasisField: row.basis, lossMappingField: row.currentMapping}
	after := map[string]any{lossReasonCodeField: row.mappedKey, lossBasisField: lossBasisMapped, lossMappingField: row.mapping.String()}
	auditID, err := storekit.Audit(ctx, tx, "update", "deal", row.id, before, after)
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, row.id, crmcontracts.PublicEventDealUpdated{ChangedFields: after})
}

// CountLossMappingCandidates counts, up to limit+1, the lost deals the given
// mappings would change; nil mappings counts for every decision.
func CountLossMappingCandidates(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64, mappings []ids.UUID,
) (int64, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+pendingMappingFrom+` LIMIT $2) pending`,
		mappings, limit+1).Scan(&count)
	return count, err
}
