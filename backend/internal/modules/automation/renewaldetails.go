// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func (def previewDef) renewalDetails(ctx context.Context, tx pgx.Tx, result *AutomationPreviewResult) error {
	w := def.dateWindow
	if w == nil {
		return nil
	}
	var args []any
	where := def.baseWhere + " AND " + w.column + " IS NULL"
	scope, err := def.scopeClause(ctx, "t", registerArg(&args))
	if err != nil {
		return err
	}
	if scope != "" {
		where += " AND " + scope
	}
	missing, err := def.countRows(ctx, tx, where, args)
	if err != nil {
		return err
	}
	details := &crmcontracts.AutomationRenewalPreview{
		MissingDates: missing, RecursYearly: w.recurring,
		Samples: []crmcontracts.AutomationRenewalPreviewSample{},
	}
	if len(result.Sample) == 0 {
		result.Renewal = details
		return nil
	}
	where, args, err = def.visibleWhere(ctx)
	if err != nil {
		return err
	}
	where += " AND t.id=ANY(" + fmt.Sprintf("$%d", registerArg(&args)(result.Sample)) + ")"
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT t.id, %s FROM %s t WHERE %s ORDER BY t.id`, w.column, def.table, where), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var occurrence time.Time
		if err := rows.Scan(&id, &occurrence); err != nil {
			return err
		}
		if w.recurring {
			occurrence = fieldcatalog.ProjectAnnualOccurrence(occurrence.Month(), occurrence.Day(), w.from, w.to)
		}
		details.Samples = append(details.Samples, crmcontracts.AutomationRenewalPreviewSample{
			EntityId: openapi_types.UUID(id), OccurrenceDate: openapi_types.Date{Time: occurrence},
			ReminderDate: openapi_types.Date{Time: occurrence.AddDate(0, 0, -w.daysBefore)},
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	result.Renewal = details
	return nil
}
