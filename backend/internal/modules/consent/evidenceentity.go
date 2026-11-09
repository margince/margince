// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func qualifyingWriterEntity(ctx context.Context, tx pgx.Tx, reviewed *ids.UUID) (ids.UUID, error) {
	if reviewed == nil {
		return sendingEntityOf(ctx, tx)
	}
	if !reviewed.IsZero() {
		return *reviewed, nil
	}
	var entity ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM legal_entity WHERE is_default
 AND (SELECT count(*) FROM legal_entity)=1`).Scan(&entity)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, &ContextNotAnswerableError{Why: "This review does not name the sending company. Check the message again."}
	}
	return entity, err
}

func evidenceEntity(ctx context.Context, tx pgx.Tx, entity ids.UUID) (ids.UUID, error) {
	if !entity.IsZero() {
		return entity, nil
	}
	return sendingEntityOf(ctx, tx)
}

// Old evidence states no entity. Several entities require its scope to be confirmed.
func legacyEvidenceEntitySQL(entity string) string {
	return `((SELECT count(*) FROM legal_entity) = 1
 AND EXISTS (SELECT 1 FROM legal_entity WHERE id = ` + entity + ` AND is_default))`
}

func recordedEntitySQL(column, entity string) string {
	return `(` + column + ` = ` + entity + ` OR (` + column + ` IS NULL AND ` + legacyEvidenceEntitySQL(entity) + `))`
}

func recordedEntitiesSQL(column, entity string) string {
	return `(` + entity + ` = ANY(` + column + `) OR (` + column + ` IS NULL AND ` + legacyEvidenceEntitySQL(entity) + `))`
}

func activityEntitySQL(alias, entity string) string {
	return `(` + recordedEntitySQL(alias+`.legal_entity_id`, entity) + ` OR EXISTS (
 SELECT 1 FROM capture_import entity_import WHERE entity_import.activity_id = ` + alias + `.id
 AND ` + recordedEntitySQL(`entity_import.legal_entity_id`, entity) + `))`
}

func qualifyingSourceEntities(ctx context.Context, tx pgx.Tx, event QualifyingEvent) ([]ids.UUID, error) {
	if event.SourceEntityType != "activity" {
		return nil, errors.New("consent: the exchange does not name a recorded activity")
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT scope.entity FROM activity a CROSS JOIN LATERAL (
 SELECT a.legal_entity_id AS entity
 UNION SELECT ci.legal_entity_id FROM capture_import ci WHERE ci.activity_id = a.id
 UNION SELECT le.id FROM legal_entity le WHERE le.is_default AND (SELECT count(*) FROM legal_entity) = 1
 ) scope WHERE a.id = @activity AND a.archived_at IS NULL AND a.restricted_at IS NULL AND scope.entity IS NOT NULL`,
		pgx.StrictNamedArgs{"activity": event.SourceEntityID})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
