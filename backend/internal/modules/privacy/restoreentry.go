// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The reversal's own history line, read back rather than assembled a second
// time. What a restore answers must be the line the history screen will show;
// rendering it here from the write's own knowledge would be a second spelling
// that can disagree with the first.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UndidAuditLogID is the evidence key naming the row a restore reverses. It is
// the only link between the two, so it is spelled here — where the read that
// follows it lives — and imported by the writer rather than typed twice.
const UndidAuditLogID = "undid_audit_log_id"

// ReadRestoreOf returns the history line for the restore that reversed
// undidID, rendered exactly as ListRecordHistory renders it.
//
// It is looked up by the evidence link and not by "the newest restore on the
// record": a record with several reversals has several, and taking the newest
// would hand the caller somebody else's line whenever two people press Undo in
// the same moment.
func ReadRestoreOf(ctx context.Context, db *database.DB, entityType string, entityID, undidID ids.UUID) (RecordHistoryEntry, error) {
	var entry RecordHistoryEntry
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		// The same row-scope gate the history read takes. Anything returning a
		// record is a read and carries it, replay and reversal paths included.
		var visErr error
		if entityType == entityTypeActivity {
			visErr = auth.EnsureActivityContentVisible(ctx, tx, entityID)
		} else {
			visErr = auth.EnsureVisible(ctx, tx, entityType, entityID)
		}
		if visErr != nil {
			return visErr
		}
		row, err := queryRestoreOf(ctx, tx, entityType, entityID, undidID)
		if err != nil {
			return err
		}
		entry = recordHistoryEntry(row, defaultFieldMasks[entityType])
		return nil
	})
	if err != nil {
		return RecordHistoryEntry{}, err
	}
	return entry, nil
}

// queryRestoreOf reads the reversal row through the record-history window, so a
// reversal recorded on a LINK is found from the record whose history was being
// read. An edge reversal's audit row sits on ('relationship', edge_id): bound to
// the record's own identity this read misses it, the write is reported as a
// no-op, and a link that really was removed reads as "the record already holds
// these values".
//
// The window's admission is what keeps that widening honest — the reversal is
// found only through an edge the record is an end of, whose other end the caller
// may see.
func queryRestoreOf(ctx context.Context, tx pgx.Tx, entityType string, entityID, undidID ids.UUID) (recordAuditRow, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos, idPos := arg(entityType), arg(entityID)
	conds := []string{fmt.Sprintf("%s = $%d::text", reversalLinkColumn, arg(undidID))}

	edgeCTE, err := edgeSubjectCTE(ctx, entityType, idPos, arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		edgeCTE = ""
	} else if err != nil {
		return recordAuditRow{}, err
	}

	var row recordAuditRow
	// Newest first with a window of one: a record with several reversals of one
	// entry has the newest as its current answer, which is the order the window
	// already imposes.
	err = scanEdgeAuditRow(tx.QueryRow(ctx,
		recordHistoryWindowSQL(typePos, idPos, arg(1), conds, edgeCTE), args...), &row)
	if errors.Is(err, pgx.ErrNoRows) {
		return recordAuditRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return recordAuditRow{}, fmt.Errorf("read the reversal's history line: %w", err)
	}
	return row, nil
}
