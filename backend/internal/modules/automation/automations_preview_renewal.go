// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// renewal_reminder's own slice of the preview surface (automations_preview.go):
// unlike every other catalog key, its table and watched column are
// per-instance (the workspace's own object/date_field params), so it has
// no static previewDefs() entry — resolvePreviewRecipe calls into this
// file instead to build one at request time. Split into its own file
// because it is a distinct concept from the static preview registry, not
// because of size alone.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// Save and preview share the live field check so retired or retyped fields
// cannot leave a newly configured reminder with no usable date source.
func renewalPreviewParams(ctx context.Context, catalog fieldcatalog.Reader, stored Automation, in AutomationPreviewInput) (dateFieldScanParams, error) {
	raw := stored.Params
	if in.Params != nil {
		encoded, err := json.Marshal(in.Params)
		if err != nil {
			return dateFieldScanParams{}, fmt.Errorf("automation: encoding preview params override: %w", err)
		}
		raw = encoded
	}
	p, err := renewalDateFieldScanParams(raw)
	if err != nil {
		if errors.Is(err, errRenewalScanParamsMissing) {
			return dateFieldScanParams{}, &ParamError{
				Field:  paramFieldObject,
				Reason: "object and date_field must both be set to preview a renewal reminder",
			}
		}
		return dateFieldScanParams{}, err
	}
	if !renewalReminderObjectSet[p.Object] {
		return dateFieldScanParams{}, &ParamError{
			Field:  paramFieldObject,
			Reason: "must be one of " + strings.Join(renewalReminderObjects, ", "),
		}
	}
	if err := validateRenewalPreviewDateField(ctx, catalog, p.Object, p.Column); err != nil {
		return dateFieldScanParams{}, err
	}
	return p, nil
}

// validateRenewalPreviewDateField refuses a date_field that is not, right
// now, an active date-typed custom field on object — the preview-time twin
// of customfields.Service.DateFieldCandidates' own ErrUnknownDateColumn
// check, run here instead so the refusal is a 422 ParamError rather than
// the raw database error measure() would otherwise surface (42703, an
// undefined column, if date_field names nothing real). A nil catalog skips
// this check entirely; see renewalPreviewParams' own doc for why.
func validateRenewalPreviewDateField(ctx context.Context, catalog fieldcatalog.Reader, object, column string) error {
	if catalog == nil {
		return nil
	}
	cols, err := catalog.ActiveColumns(ctx, object)
	if err != nil {
		return fmt.Errorf("automation: loading %s's active columns for preview: %w", object, err)
	}
	for _, c := range cols {
		if c.Name == column && c.Type == fieldcatalog.TypeDate {
			return nil
		}
	}
	return &ParamError{
		Field:  "params." + paramKeyDateField,
		Reason: fmt.Sprintf("%q is not an active date-typed custom field on %s", column, object),
	}
}

type previewDateWindow struct {
	daysBefore int
	column     string
	from, to   time.Time
	recurring  bool
}

// Preview and the scheduled scan use the same date-window predicate.
func renewalPreviewDef(now time.Time, p dateFieldScanParams) previewDef {
	column := "t." + pgx.Identifier{p.Column}.Sanitize()
	to := now.AddDate(0, 0, p.DaysBefore)
	return previewDef{
		table:                 p.Object,
		baseWhere:             previewBaseWhereNotArchived,
		dateWindow:            &previewDateWindow{column: column, from: now, to: to, recurring: p.RecursYearly, daysBefore: p.DaysBefore},
		historicalUnavailable: p.RecursYearly,
		firedCount: func(ctx context.Context, tx pgx.Tx, since time.Time) (int, error) {
			var args []any
			where := storekit.DateWindowSQL(column, since, to, false, registerArg(&args))
			var count int
			err := tx.QueryRow(ctx, storekit.SQLf(`SELECT count(*) FROM %s t WHERE %s`, p.Object, where), args...).Scan(&count)
			return count, err
		},
	}
}
