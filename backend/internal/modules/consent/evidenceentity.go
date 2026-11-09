// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	err := tx.QueryRow(ctx, `SELECT le.id FROM legal_entity le WHERE `+legacyEvidenceEntitySQL("le.id")).Scan(&entity)
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

func activityEvidenceSQL(ctx context.Context, alias, entity string, args *[]any) (string, error) {
	visible, err := auth.ActivityContentClause(ctx, alias, func(value any) int {
		*args = append(*args, value)
		return len(*args)
	})
	if err != nil {
		return "", err
	}
	return "(" + visible + ") AND " + activityEntitySQL(alias, entity), nil
}

const availableQualifyingActivitySQL = `a.archived_at IS NULL AND a.restricted_at IS NULL`

func recordedQualificationSQL(ctx context.Context, alias string, args *[]any) (string, error) {
	visible, err := auth.ActivityContentClause(ctx, "a", func(value any) int {
		*args = append(*args, value)
		return len(*args)
	})
	if err != nil {
		return "", err
	}
	return qualifyingRecordsSQL(alias, visible, args), nil
}

func readableEvidenceSQL(ctx context.Context, need string, args *[]any) (string, error) {
	readable, err := auth.ContentReadableActivitiesSQL(ctx, need, func(value any) int {
		*args = append(*args, value)
		return len(*args)
	})
	if err != nil {
		return "", err
	}
	return "WITH readable_evidence AS MATERIALIZED (" + readable + ") ", nil
}

func qualifyingRecordsSQL(alias, visible string, args *[]any) string {
	bind := func(value any) string {
		*args = append(*args, value)
		return storekit.SQLf("$%d", len(*args))
	}
	var manualKinds []string
	for kind, manual := range handRecordedKinds {
		if manual {
			manualKinds = append(manualKinds, kind)
		}
	}
	sort.Strings(manualKinds)
	kinds := bind(manualKinds)
	people := bind([]string{sourceKindHuman, sourceKindAgent})
	captured, inquiry := bind(sourceKindCaptured), bind(KindInquiry)
	return `((` + alias + `.kind=ANY(` + kinds + `) AND ` + alias + `.source=ANY(` + people + `))
 OR (` + alias + `.source=` + captured + ` AND ` + alias + `.kind=` + inquiry + `
 AND ` + alias + `.source_entity_type='activity' AND EXISTS (
 SELECT 1 FROM activity a WHERE a.id=` + alias + `.source_entity_id
 AND ` + availableQualifyingActivitySQL + ` AND (` + visible + `))))`
}

func qualifyingSourceEntities(ctx context.Context, tx pgx.Tx, event QualifyingEvent) ([]ids.UUID, bool, error) {
	if event.SourceEntityType != "activity" {
		return nil, false, errors.New("consent: the exchange does not name a recorded activity")
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT scope.entity FROM activity a CROSS JOIN LATERAL (
 SELECT a.legal_entity_id AS entity
 UNION SELECT ci.legal_entity_id FROM capture_import ci WHERE ci.activity_id = a.id
 UNION SELECT le.id FROM legal_entity le WHERE `+legacyEvidenceEntitySQL("le.id")+`
 ) scope WHERE a.id = @activity AND `+availableQualifyingActivitySQL+` ORDER BY scope.entity`,
		pgx.StrictNamedArgs{"activity": event.SourceEntityID})
	if err != nil {
		return nil, false, err
	}
	scopes, err := pgx.CollectRows(rows, pgx.RowTo[*ids.UUID])
	if err != nil || len(scopes) == 0 {
		return nil, false, err
	}
	entities := make([]ids.UUID, 0, len(scopes))
	for _, entity := range scopes {
		if entity != nil {
			entities = append(entities, *entity)
		}
	}
	return entities, true, nil
}
