// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package generatedsummary

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StandingRecord is a record that keeps a status line: the entity type the
// table's CHECK allows and the record's id.
type StandingRecord struct {
	EntityType string
	EntityID   ids.UUID
}

// StandingLine is one record's status sentence and the activities it was
// written from. It is shared by every reader, so whoever serves it must ask
// that the reader may open each of them.
type StandingLine struct {
	Text        string
	Evidence    []ids.UUID
	Language    string
	Fingerprint string
	WrittenAt   time.Time
}

// ReadStandingLineTx is the record's line, and false when none was written.
func ReadStandingLineTx(ctx context.Context, tx pgx.Tx, record StandingRecord) (StandingLine, bool, error) {
	var line StandingLine
	err := tx.QueryRow(ctx, `
		SELECT text, evidence, language, fingerprint, written_at
		  FROM standing_line WHERE entity_type = $1 AND entity_id = $2`,
		record.EntityType, record.EntityID).Scan(
		&line.Text, &line.Evidence, &line.Language, &line.Fingerprint, &line.WrittenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StandingLine{}, false, nil
	}
	if err != nil {
		return StandingLine{}, false, fmt.Errorf("read the status line of a record: %w", err)
	}
	return line, true, nil
}

// SaveStandingLineTx replaces the record's line.
func SaveStandingLineTx(ctx context.Context, tx pgx.Tx, record StandingRecord, line StandingLine) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO standing_line (entity_type, entity_id, text, evidence, language, fingerprint, written_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (entity_type, entity_id) DO UPDATE
		SET text = EXCLUDED.text, evidence = EXCLUDED.evidence, language = EXCLUDED.language,
		    fingerprint = EXCLUDED.fingerprint, written_at = EXCLUDED.written_at`,
		record.EntityType, record.EntityID, line.Text, line.Evidence, line.Language,
		line.Fingerprint, line.WrittenAt); err != nil {
		return fmt.Errorf("store the status line of a record: %w", err)
	}
	return nil
}

// TouchStandingLineTx keeps the line and records that it was read against a
// newer input, so the same input is not asked about twice.
func TouchStandingLineTx(ctx context.Context, tx pgx.Tx, record StandingRecord, fingerprint string) error {
	if _, err := tx.Exec(ctx,
		`UPDATE standing_line SET fingerprint = $3 WHERE entity_type = $1 AND entity_id = $2`,
		record.EntityType, record.EntityID, fingerprint); err != nil {
		return fmt.Errorf("mark the status line of a record as current: %w", err)
	}
	return nil
}

// forgetStandingLinesTx deletes the lines about these contacts. It also
// deletes every line written from an activity filed on one of them, or from
// one already gone, since it may quote them.
func forgetStandingLinesTx(ctx context.Context, tx pgx.Tx, contacts []ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM standing_line
		 WHERE (entity_type = 'contact' AND entity_id = ANY($1))
		    OR evidence && ARRAY(SELECT activity_id FROM activity_link WHERE contact_id = ANY($1))
		    OR EXISTS (SELECT 1 FROM unnest(evidence) cited
		                WHERE NOT EXISTS (SELECT 1 FROM activity WHERE activity.id = cited))`,
		contacts); err != nil {
		return fmt.Errorf("delete the status lines that may quote a contact: %w", err)
	}
	return nil
}
