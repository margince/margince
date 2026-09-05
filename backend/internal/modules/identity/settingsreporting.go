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

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// FiscalYearStartMonth is the month the installation's business year begins,
// 1..12. January is the default, which is what every installation reported by
// before this existed — so an installation that never touches it sees no
// change, and no saved report view moves under it.
//
// It buckets reports on READ and stores nothing, so changing it re-labels every
// period report immediately and re-means no stored row. That is the opposite of
// BaseCurrency, which freezes: a fiscal year is a way of cutting time, not a
// value anything has already converted against.
//
// It does re-point a SAVED report view, which is a real gap rather than a
// property of this setting: a period bucket's text travels out in a derivation
// handle and binds back as an equality filter (reportperiod.go), so a view
// saved under one fiscal start names a different span after it moves. Tracked
// as its own decision — re-point, invalidate, or warn — rather than settled
// here, because none of the three is obviously right and the label cannot fix
// it either way: margince/margince#2569. Named rather than described, so a
// reader can see whether it is still open instead of trusting a comment's
// account of a gap.
//
// What the label DOES fix is the reader's half: spelling both years means the
// answer they get is unambiguous about which twelve months it covers, even when
// the filter that produced it is stale.
var FiscalYearStartMonth = settings.Define[int](
	"installation.fiscal_year_start_month",
	installationSettingsObject,
	"update",
	int(time.January),
	func(month int) error {
		if month < int(time.January) || month > int(time.December) {
			return fmt.Errorf("a fiscal year starts in month 1..12, not %d", month)
		}
		return nil
	},
).AsInstallationIdentity()

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
var ForecastForwardMeasure = settings.Define[string](
	"installation.forecast_forward_measure",
	installationSettingsObject,
	"update",
	string(values.MeasureCommitEvidence),
	func(measure string) error {
		if _, err := values.ParseForwardMeasure(&measure, "forecast_forward_measure"); err != nil {
			return err
		}
		return nil
	},
).AsInstallationIdentity()

// ForecastForwardMeasureOf resolves the stored measure inside a transaction the
// caller already holds.
//
// Here rather than in the composition for the reason FiscalYearStartMonthOf is:
// how this setting is read belongs with the entry that declares it, so a
// wiring site injecting it does not restate the default or the key.
func ForecastForwardMeasureOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return settings.GetTx(ctx, tx, ForecastForwardMeasure)
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
	return settings.GetTx(ctx, tx, FiscalYearStartMonth)
}
