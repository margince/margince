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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	defaultLostStageName: {"de": "Verloren", "vi": "Thua"},
}

// seededTranslations answers a copy of the seeded translations of name, nil
// for a name the product does not seed.
func seededTranslations(name string) map[string]string {
	return maps.Clone(seededStageNames[name])
}

// carriesSeededTranslations reports a stage whose translations are still the
// ones the product seeded for name. A rename drops them, and the repair
// fills them only where none were ever written.
func carriesSeededTranslations(name string, translations map[string]string) bool {
	seeded, ok := seededStageNames[name]
	return ok && maps.Equal(seeded, translations)
}

// TranslateSeededStagesTx gives a page of live stages after `after` that still
// carry a seeded English name and no translations the seeded translations of
// that name. A stage an admin renamed or translated is not selected, so a
// second run changes nothing. It answers the stages it read and how many it
// changed.
func TranslateSeededStagesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (scanned []ids.UUID, changed int, err error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	if err := auth.Require(ctx, "pipeline", principal.ActionUpdate); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, pipeline_id, name FROM stage
		 WHERE id > $1 AND archived_at IS NULL AND name = ANY($2) AND name_translations = '{}'::jsonb
		 ORDER BY id LIMIT $3 FOR UPDATE`, after, seededNameList(), limit)
	if err != nil {
		return nil, 0, fmt.Errorf("deals: reading stages that lack their seeded translations: %w", err)
	}
	type untranslated struct {
		id       ids.UUID
		pipeline ids.UUID
		name     string
	}
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (untranslated, error) {
		var st untranslated
		err := row.Scan(&st.id, &st.pipeline, &st.name)
		return st, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("deals: reading stages that lack their seeded translations: %w", err)
	}
	for _, st := range page {
		scanned = append(scanned, st.id)
		translations := seededTranslations(st.name)
		if _, err := tx.Exec(ctx, `UPDATE stage SET name_translations = $2 WHERE id = $1`, st.id, translations); err != nil {
			return nil, 0, fmt.Errorf("deals: giving a seeded stage its translations: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "stage", st.id,
			map[string]any{stageNameTranslationsField: map[string]string{}}, map[string]any{stageNameTranslationsField: translations})
		if err != nil {
			return nil, 0, fmt.Errorf("deals: auditing a seeded stage's translations: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, st.id, crmcontracts.PublicEventStageUpdated{
			PipelineId: openapi_types.UUID(st.pipeline),
		}); err != nil {
			return nil, 0, fmt.Errorf("deals: announcing a seeded stage's translations: %w", err)
		}
		changed++
	}
	return scanned, changed, nil
}

// CountUntranslatedSeededStages counts, up to limit+1, the stages
// TranslateSeededStagesTx will change.
func CountUntranslatedSeededStages(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM (
		  SELECT 1 FROM stage
		   WHERE archived_at IS NULL AND name = ANY($1) AND name_translations = '{}'::jsonb
		   LIMIT $2) AS page`, seededNameList(), limit+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("deals: counting stages that lack their seeded translations: %w", err)
	}
	return count, nil
}

func seededNameList() []string {
	names := make([]string, 0, len(seededStageNames))
	for name := range seededStageNames {
		names = append(names, name)
	}
	return names
}
