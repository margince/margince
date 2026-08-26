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

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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

func queryRestoreOf(ctx context.Context, tx pgx.Tx, entityType string, entityID, undidID ids.UUID) (recordAuditRow, error) {
	var row recordAuditRow
	err := scanRecordAuditRow(tx.QueryRow(ctx, `
		SELECT a.id, a.actor_type, a.actor_id, a.on_behalf_of, a.action, a.occurred_at,
		       a.authorization_rule, a.before, a.after, a.passport_id,
		       actor_user.display_name, obo.display_name, oc.client_name
		FROM audit_log a`+auditActorNameJoins+agentClientNameJoin+`
		WHERE a.entity_type = $1 AND a.entity_id = $2
		  AND a.evidence ->> $4 = $3::text
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT 1`, entityType, entityID, undidID, UndidAuditLogID), &row)
	if errors.Is(err, pgx.ErrNoRows) {
		return recordAuditRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return recordAuditRow{}, fmt.Errorf("read the reversal's history line: %w", err)
	}
	return row, nil
}

// auditRowScanner is what both readers of this projection have in common: a
// pgx.Row and a pgx.Rows both scan. One spelling of the column list and its
// two jsonb decodes, because two would drift the moment a column is added to
// one query and not the other.
type auditRowScanner interface {
	Scan(dest ...any) error
}

func scanRecordAuditRow(src auditRowScanner, r *recordAuditRow) error {
	var beforeJSON, afterJSON []byte
	if err := src.Scan(&r.id, &r.actorType, &r.actorID, &r.onBehalfOf, &r.action, &r.occurredAt,
		&r.authorizationRule, &beforeJSON, &afterJSON, &r.passportID,
		&r.actorDisplayName, &r.onBehalfOfName, &r.agentClientName); err != nil {
		return err
	}
	if err := unmarshalJSONBMap(beforeJSON, &r.before); err != nil {
		return fmt.Errorf("audit row %s before: %w", r.id, err)
	}
	if err := unmarshalJSONBMap(afterJSON, &r.after); err != nil {
		return fmt.Errorf("audit row %s after: %w", r.id, err)
	}
	return nil
}
