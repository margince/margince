// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The reviewed mapping of legacy loss notes to codes. An administrator decides
// one note at a time; a data task then writes the decided code onto the lost
// deals carrying that note. The note itself is never rewritten, and a code the
// closer chose is never replaced.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

var (
	lossReasonKeyShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	noteKeyShape       = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// legacyNoteKeySQL is a note's mapping key, the expression
// idx_deal_lost_reason_note_key indexes. The list a human decides from, the
// task that applies the decision and the cleanup that drops it all call it.
func legacyNoteKeySQL(column string) string {
	return `md5(lower(btrim(` + column + `)))`
}

// mappableNoteSQL is a live lost deal a mapping may speak for: one with no
// code, or one an earlier mapping coded. A code the closer recorded is not one.
const mappableNoteSQL = `d.archived_at IS NULL AND d.status = 'lost' AND d.lost_reason IS NOT NULL
	AND (d.lost_reason_code IS NULL OR d.lost_reason_basis = 'mapped')`

// ListLegacyLossReasons answers the notes of the lost deals the caller may
// read that a mapping speaks for, undecided ones first. Each says how many
// deals its decision would still change. Only a seat that
// may decide a mapping may list them.
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
		rows, err := tx.Query(ctx, storekit.SQLf(`SELECT `+legacyNoteKeySQL("d.lost_reason")+` AS note_key,
			       min(d.lost_reason), count(*)::int, (count(*) FILTER (WHERE m.id IS NOT NULL
			         AND (d.lost_reason_code IS NULL OR d.lost_reason_code <> m.reason_key)))::int,
			       (count(*) OVER ())::int, m.id, m.reason_key, m.decided_by, m.updated_at, m.version
			  FROM deal d
			  LEFT JOIN deal_loss_reason_mapping m ON m.note_key = `+legacyNoteKeySQL("d.lost_reason")+`
			 WHERE `+mappableNoteSQL+` AND (`+scope+`)
			 GROUP BY 1, m.id
			 ORDER BY m.id IS NULL DESC, count(*) DESC, 1 LIMIT $%d`, limitAt), args...)
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
	if err := rows.Scan(&row.TextHash, &row.Text, &row.DealCount, &row.PendingCount, &total,
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
// decision to the deals once an administrator applies the mappings they saw.
func (s *Store) DecideLegacyLossReason(ctx context.Context, noteKey, reasonKey string) (crmcontracts.LossReasonMapping, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.LossReasonMapping{}, err
	}
	if !noteKeyShape.MatchString(noteKey) {
		return crmcontracts.LossReasonMapping{}, &values.ParseError{Field: "text_hash", Code: "invalid_hash", Message: "text_hash must be 32 lowercase hex digits"}
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
		before, found, err := readLossMappingByKey(ctx, tx, noteKey)
		if err != nil {
			return err
		}
		if found && before.ReasonKey == key {
			out = before
			return nil
		}
		if err := writeLossMapping(ctx, tx, noteKey, key, by, before, found); err != nil {
			return err
		}
		out, _, err = readLossMappingByKey(ctx, tx, noteKey)
		return err
	})
	return out, err
}

// writeLossMapping stores a decision with its audit row and event. A second
// decision on one note replaces the first; the audit row keeps the first.
func writeLossMapping(
	ctx context.Context, tx pgx.Tx, noteKey, key, by string,
	before crmcontracts.LossReasonMapping, found bool,
) error {
	after := map[string]any{reasonKeyField: key, decidedByField: by}
	if !found {
		id := ids.NewV7()
		_, err := tx.Exec(ctx, `INSERT INTO deal_loss_reason_mapping (id, note_key, reason_key, decided_by)
			VALUES ($1, $2, $3, $4)`, id, noteKey, key, by)
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

func readLossMappingByKey(ctx context.Context, tx pgx.Tx, noteKey string) (crmcontracts.LossReasonMapping, bool, error) {
	var out crmcontracts.LossReasonMapping
	var id ids.UUID
	err := tx.QueryRow(ctx, `SELECT id, note_key, reason_key, decided_by, updated_at, version
		FROM deal_loss_reason_mapping WHERE note_key = $1 FOR UPDATE`, noteKey).
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

// dropOrphanedLossMapping deletes the decision on a note once no deal carries
// that note any more, with its audit row and event. A mapping is derived from
// a free-text note, so it must not outlive every deal that wrote it.
func dropOrphanedLossMapping(ctx context.Context, tx pgx.Tx, note *string) error {
	if note == nil {
		return nil
	}
	rows, err := tx.Query(ctx, `DELETE FROM deal_loss_reason_mapping m
		WHERE m.note_key = `+legacyNoteKeySQL("$1::text")+`
		  AND NOT EXISTS (SELECT 1 FROM deal d WHERE d.lost_reason IS NOT NULL
		                  AND `+legacyNoteKeySQL("d.lost_reason")+` = m.note_key)
		RETURNING m.id, m.reason_key, m.decided_by`, *note)
	if err != nil {
		return fmt.Errorf("drop an orphaned loss mapping: %w", err)
	}
	type dropped struct {
		id             ids.UUID
		key, decidedBy string
	}
	gone, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dropped, error) {
		var d dropped
		return d, row.Scan(&d.id, &d.key, &d.decidedBy)
	})
	if err != nil {
		return err
	}
	for _, d := range gone {
		auditID, err := storekit.Audit(ctx, tx, "delete", "deal_loss_reason_mapping", d.id,
			map[string]any{reasonKeyField: d.key, decidedByField: d.decidedBy}, nil)
		if err != nil {
			return err
		}
		if err := announceLossReasonChange(ctx, tx, auditID, lossMappingTable, "delete", d.id); err != nil {
			return err
		}
	}
	return nil
}
