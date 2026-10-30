// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The settings that decide how the installation REPORTS, as opposed to who it
// is: which months a business year is cut into, and which reading a projected
// landing is built from.
//
// Both are applied on READ and store nothing, which is what separates them from
// the identity settings beside them: changing either re-labels or re-computes
// every report at once and re-means no stored row. Neither ever freezes.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// FiscalYearStartMonth is the month the installation's business year begins,
// 1..12, January by default. It buckets reports on read, so a move relabels
// every period report. A saved period filter keeps the calendar it was saved
// under, which the CalendarChange hook writes down before the move.
var FiscalYearStartMonth = config.DefineStored(config.Spec[int]{
	Key:     "installation.fiscal_year_start_month",
	Env:     "MARGINCE_INSTALLATION_FISCAL_YEAR_START_MONTH",
	Type:    config.Int,
	Default: config.Some(int(time.January)),
	Validate: func(month int) error {
		if month < int(time.January) || month > int(time.December) {
			return fmt.Errorf("a fiscal year starts in month 1..12, not %d", month)
		}
		return nil
	},
	Doc: "The month the business year begins, 1 to 12.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

// ForecastForwardMeasure is which remaining-pipeline reading a projected
// landing is built from.
//
// A setting rather than a fixed choice because it is a question about how the
// installation SELLS, not about the software: a team with a disciplined commit
// stage means something by it, and one that commits everything does not — their
// weighted number is the honest one.
//
// Applied on READ and storing nothing, like FiscalYearStartMonth and unlike
// BaseCurrency: changing it re-computes every landing at once and re-means no
// stored row, so it never freezes.
//
// The set of values lives in the shared kernel because forecasting refuses
// anything outside it and this package may not import that module. Validating
// against a copy here would let the two drift, and the drift only shows up as
// a forecast read failing for an installation that saved cleanly.
var ForecastForwardMeasure = config.DefineStored(config.Spec[string]{
	Key:     "installation.forecast_forward_measure",
	Env:     "MARGINCE_INSTALLATION_FORECAST_FORWARD_MEASURE",
	Type:    config.String,
	Default: config.Some(string(values.MeasureCommitEvidence)),
	Validate: func(measure string) error {
		if _, err := values.ParseForwardMeasure(&measure, "forecast_forward_measure"); err != nil {
			return err
		}
		return nil
	},
	Doc: "Which remaining-pipeline reading a projected landing is built from.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, SurvivesDataReset: true})

// ForecastForwardMeasureOf resolves the stored measure inside a transaction the
// caller already holds.
//
// Here rather than in the composition for the reason FiscalYearStartMonthOf is:
// how this setting is read belongs with the entry that declares it, so a
// wiring site injecting it does not restate the default or the key.
func ForecastForwardMeasureOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return config.StoredValTx(ctx, tx, config.Bound(), ForecastForwardMeasure)
}

// FiscalYearStartMonthOf resolves the month the installation's business year
// begins, inside a transaction the caller already holds.
//
// GetTx for the same reason BaseLanguageOf uses it: an installation
// bootstrapped before this setting existed carries no row, and the default is
// January — the calendar year those installations have always reported by. So
// an absent row is not a broken installation, it is an older one that agrees
// with the default.
func FiscalYearStartMonthOf(ctx context.Context, tx pgx.Tx) (int, error) {
	return config.StoredValTx(ctx, tx, config.Bound(), FiscalYearStartMonth)
}
