// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"maps"

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

// seededStageNames holds the further UI languages of every stage name the
// product seeds, keyed by the English name. The default ladder, the closing
// pair and the repair of older workspaces all read it.
var seededStageNames = map[string]map[string]string{
	"Qualified":          {"de": "Qualifiziert", "vi": "Đủ điều kiện"},
	"Discovery":          {"de": "Bedarfsanalyse", "vi": "Khám phá"},
	"Proposal":           {"de": "Angebot", "vi": "Đề xuất"},
	"Negotiation":        {"de": "Verhandlung", "vi": "Đàm phán"},
	defaultWonStageName:  {"de": "Gewonnen", "vi": "Thắng"},
	defaultLostStageName: {"de": "Verloren", "vi": "Thất bại"},
}

// seededTranslations answers a copy of the seeded translations of name, nil
// for a name the product does not seed.
func seededTranslations(name string) map[string]string {
	return maps.Clone(seededStageNames[name])
}

// carriesSeededTranslations reports a stage whose translations are the set
// the product seeds for name. Whether the product wrote them is
// translationsWrittenBySeed's question.
func carriesSeededTranslations(name string, translations map[string]string) bool {
	seeded, ok := seededStageNames[name]
	return ok && maps.Equal(seeded, translations)
}

// seededTranslationsMark is the audit-image key a seeding writer adds when it
// writes a stage's translations. A stage's audit trail then tells the
// product's translations from the same words an admin typed.
const seededTranslationsMark = "name_translations_seeded"

// translationsWrittenBySeed reports that no one but a seeding writer ever
// wrote translations on the stage. A stage born with its pipeline has no
// audit row of its own, so it counts as seeded.
func translationsWrittenBySeed(ctx context.Context, tx pgx.Tx, id ids.StageID) (bool, error) {
	var adminWrote bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM audit_log
		   WHERE entity_type = 'stage' AND entity_id = $1 AND after ? $2
		     AND after->$2 <> '{}'::jsonb AND NOT COALESCE((after->>$3)::boolean, false))`,
		id, stageNameTranslationsField, seededTranslationsMark).Scan(&adminWrote)
	if err != nil {
		return false, fmt.Errorf("deals: reading who translated a stage: %w", err)
	}
	return !adminWrote, nil
}

// seededStageSQL selects the live stages the product seeded that still have
// no translations. A seeded stage was born with a pipeline a system writer
// created. It has no create row of its own and was never renamed. A stage an
// admin added, or renamed back to a seeded name, is not one.
const seededStageSQL = `
	FROM stage s
	WHERE s.archived_at IS NULL AND s.name = ANY(@names) AND s.name_translations = '{}'::jsonb
	  AND EXISTS (SELECT 1 FROM audit_log p
	               WHERE p.entity_type = 'pipeline' AND p.entity_id = s.pipeline_id
	                 AND p.action = 'create' AND p.actor_type = 'system')
	  AND NOT EXISTS (SELECT 1 FROM audit_log a
	                   WHERE a.entity_type = 'stage' AND a.entity_id = s.id
	                     AND (a.action = 'create' OR a.after ? 'name'))`

// TranslateSeededStagesTx gives a page of the stages seededStageSQL selects
// after `after` the seeded translations of their name. A second run changes
// nothing. It answers the stages it read and how many it changed.
func TranslateSeededStagesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (scanned []ids.UUID, changed int, err error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id, s.pipeline_id, s.name, s.version `+seededStageSQL+`
		AND s.id > @after ORDER BY s.id LIMIT @limit FOR UPDATE OF s`, seededStageArgs(after, limit))
	if err != nil {
		return nil, 0, fmt.Errorf("deals: reading stages that lack their seeded translations: %w", err)
	}
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (untranslatedStage, error) {
		var st untranslatedStage
		err := row.Scan(&st.id, &st.pipeline, &st.name, &st.version)
		return st, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("deals: reading stages that lack their seeded translations: %w", err)
	}
	for _, st := range page {
		scanned = append(scanned, st.id)
		if err := translateSeededStage(ctx, tx, st); err != nil {
			return nil, 0, err
		}
		changed++
	}
	return scanned, changed, nil
}

type untranslatedStage struct {
	id, pipeline ids.UUID
	name         string
	version      int64
}

// translateSeededStage writes one stage's seeded translations under the
// version the page read, with its audit row and stage.updated event.
func translateSeededStage(ctx context.Context, tx pgx.Tx, st untranslatedStage) error {
	patch := storekit.NewPatch()
	patch.Set(stageNameTranslationsField, map[string]string{}, seededTranslations(st.name))
	if err := patch.ApplyWithVersion(ctx, tx, "stage", st.id, st.version); err != nil {
		return fmt.Errorf("deals: giving a seeded stage its translations: %w", err)
	}
	after := maps.Clone(patch.After())
	after[seededTranslationsMark] = true
	auditID, err := storekit.Audit(ctx, tx, "update", "stage", st.id, patch.Before(), after)
	if err != nil {
		return fmt.Errorf("deals: auditing a seeded stage's translations: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, st.id, crmcontracts.PublicEventStageUpdated{
		PipelineId: openapi_types.UUID(st.pipeline),
	}); err != nil {
		return fmt.Errorf("deals: announcing a seeded stage's translations: %w", err)
	}
	return nil
}

// CountUntranslatedSeededStages counts, up to limit+1, the stages
// TranslateSeededStagesTx will change.
func CountUntranslatedSeededStages(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+seededStageSQL+` LIMIT @limit) AS page`,
		seededStageArgs(ids.UUID{}, int(limit)+1)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("deals: counting stages that lack their seeded translations: %w", err)
	}
	return count, nil
}

// seededStageArgs binds seededStageSQL and the page after it.
func seededStageArgs(after ids.UUID, limit int) pgx.NamedArgs {
	return pgx.NamedArgs{"names": seededNameList(), "after": after, "limit": limit}
}

func seededNameList() []string {
	names := make([]string, 0, len(seededStageNames))
	for name := range seededStageNames {
		names = append(names, name)
	}
	return names
}

// SeededStageNamesDataTask gives the stages an older workspace was seeded with
// their German and Vietnamese names, so a reader in those languages sees no
// English stage.
func SeededStageNamesDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795583688_seeded_stages_translated", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795583688_seeded_stages_translated"),
		Requires: []string{"core/1794481502"},
		Steps: []datatask.Step{{
			Name: "translate_seeded_stages", Limit: 200, MinLimit: 10,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				count, err := CountUntranslatedSeededStages(ctx, q, maximum)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				scanned, changed, err := TranslateSeededStagesTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, scanned, changed)
			},
		}},
	}
}
