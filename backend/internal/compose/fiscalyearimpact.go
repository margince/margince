// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a move of the reporting calendar does to saved work. A saved period
// such as `2026` keeps the dates it named when it was saved. A relative one,
// such as this quarter, follows the setting. The admin is told how many of
// each before the move. The move writes the calendar in force until then on
// every saved question that names a period without one.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// periodFilterOn renders the test that a stored question, at the given JSON
// path, filters on a period with a value to compare. $1 binds the populations
// with period fields and $2 the field names.
func periodFilterOn(question string, unkept bool) string {
	keep := ""
	if unkept {
		keep = " AND filter->'Basis' IS NULL"
	}
	return "(" + question + "->>'Entity' = ANY($1) AND EXISTS (SELECT 1 FROM jsonb_array_elements(" +
		"CASE WHEN jsonb_typeof(" + question + "->'Filters') = 'array' THEN " + question + "->'Filters' ELSE '[]'::jsonb END) filter" +
		" WHERE filter->>'Field' = ANY($2) AND filter->>'Op' NOT IN ('is_null', 'is_not_null')" + keep + "))"
}

// periodVocabulary is the populations that cut a period and the names of the
// period fields, read from the report specs.
func periodVocabulary() (entities, fields []string) {
	for name, spec := range prebuiltReports {
		for field, expr := range spec.dimensions {
			if periodFieldOf(field, expr) != nil {
				entities = append(entities, name)
				break
			}
		}
	}
	slices.Sort(entities)
	return entities, slices.Sorted(maps.Keys(periodGrains))
}

// GetFiscalYearImpact implements GET /installation/settings/fiscal-year-impact.
func (h installationSettingsHandlers) GetFiscalYearImpact(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		httperr.NotImplemented(w, r, "GetFiscalYearImpact")
		return
	}
	ctx := r.Context()
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(ctx, "installation_settings", principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var out crmcontracts.InstallationFiscalYearImpact
	if err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		out, err = fiscalYearImpact(ctx, tx)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// fiscalYearImpact counts the saved work a fiscal-start move concerns.
func fiscalYearImpact(ctx context.Context, tx pgx.Tx) (crmcontracts.InstallationFiscalYearImpact, error) {
	entities, fields := periodVocabulary()
	var out crmcontracts.InstallationFiscalYearImpact
	live := " FROM report_definition d JOIN report_definition_revision r ON r.report_id = d.id AND r.revision = d.revision WHERE d.archived_at IS NULL"
	err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM report_run WHERE `+periodFilterOn("query", false)+`),
		(SELECT count(*)`+live+` AND EXISTS (SELECT 1 FROM jsonb_each(r.question_bindings) b WHERE `+periodFilterOn("(b.value->'query')", false)+`)),
		(SELECT count(*)`+live+` AND (r.selection->>'period' = 'this_quarter' OR r.selection->>'target_basis' = 'fiscal_quarter' OR r.selection->>'close_window' = 'fiscal_quarter'))`,
		entities, fields,
	).Scan(&out.SavedQuestions, &out.ReportsWithKeptPeriods, &out.ReportsWithRelativePeriods)
	if err != nil {
		return out, fmt.Errorf("compose: counting saved work a fiscal-start move concerns: %w", err)
	}
	return out, nil
}

// keepSavedPeriodsOnCalendarChange is the hook a move of the reporting zone or
// the fiscal start runs. A saved question that filters on a period and keeps
// no calendar takes the one in force before the move. So `2026` saved under a
// January start still means January to December 2026 afterwards.
func keepSavedPeriodsOnCalendarChange(ctx context.Context, tx pgx.Tx, before identity.ReportingCalendar) error {
	entities, fields := periodVocabulary()
	rows, err := tx.Query(ctx, `SELECT id, query FROM report_run WHERE `+periodFilterOn("query", true)+` ORDER BY id FOR UPDATE`, entities, fields)
	if err != nil {
		return fmt.Errorf("compose: reading saved questions whose periods keep no calendar: %w", err)
	}
	type savedQuestion struct {
		id    ids.UUID
		query analyticsquery.Query
	}
	saved, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (savedQuestion, error) {
		var out savedQuestion
		var raw []byte
		if err := row.Scan(&out.id, &raw); err != nil {
			return out, err
		}
		return out, json.Unmarshal(raw, &out.query)
	})
	if err != nil {
		return fmt.Errorf("compose: decoding saved questions whose periods keep no calendar: %w", err)
	}
	basis := analyticsquery.CalendarBasis{Timezone: before.Timezone, FiscalStartMonth: before.FiscalYearStartMonth}
	for _, question := range saved {
		kept := question.query
		kept.Filters = analyticsquery.StampBasis(kept.Filters, isPeriodField(kept.Entity), basis)
		raw, err := json.Marshal(kept)
		if err != nil {
			return fmt.Errorf("compose: encoding a saved question's kept periods: %w", err)
		}
		// The rows were locked FOR UPDATE above, in this transaction.
		if _, err := tx.Exec(ctx, `UPDATE report_run SET query = $1 WHERE id = $2`, raw, question.id); err != nil {
			return fmt.Errorf("compose: keeping a saved question's periods: %w", err)
		}
		// The images name the calendar and never a filter value, which can name
		// a record the audit's readers may not open.
		audit, err := storekit.Audit(ctx, tx, "update", tableReportRun, question.id,
			map[string]any{"period_basis": nil}, map[string]any{"period_basis": basis})
		if err != nil {
			return err
		}
		if err := storekit.Emit(ctx, tx, audit, "reporting.changed", tableReportRun, question.id,
			crmcontracts.InternalEventReportingChanged{Object: tableReportRun, Action: "update"}); err != nil {
			return err
		}
	}
	return nil
}
