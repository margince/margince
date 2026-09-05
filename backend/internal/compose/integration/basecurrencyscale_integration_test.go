// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A conversion applies BOTH currencies' minor-unit scales, and the SQL
// spellings have to agree with the Go engine about that.
//
// A stored rate says what ONE MAJOR unit is worth — "1 VND = 0.000035 EUR" is
// how a rates page writes it — while amount_minor on both sides counts MINOR
// units. VND has no minor unit and EUR has two, so multiplying the minor amount
// by the major rate lands a hundredth of the truth. That is not a rounding
// argument: a ₫2,400,000,000 deal read as €840 instead of €84,000 in the
// forecast while the account page, which converts in Go, read €84,000.
//
// The cases below assert LITERAL expected amounts rather than recomputing the
// expression under test, so a scale removed from either side fails here instead
// of cancelling out.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scaleCase is one currency's conversion, with the answer written out.
type scaleCase struct {
	currency string
	// amountMinor is the deal's own money, in its own minor units.
	amountMinor int64
	// rate is one MAJOR unit of currency in MAJOR units of EUR.
	rate string
	// wantBaseMinor is the amount in EUR minor units — cents.
	wantBaseMinor int64
	// why says what the case is for, so a failure names the defect.
	why string
}

func TestAConversionAppliesBothMinorUnitScales(t *testing.T) {
	cases := []scaleCase{
		{
			currency: "VND", amountMinor: 2_400_000_000, rate: "0.000035",
			// ₫2,400,000,000 (VND has no minor unit) × 0.000035 = €84,000.00.
			wantBaseMinor: 8_400_000,
			why:           "a zero-decimal currency against a two-decimal base — the 100x case",
		},
		{
			currency: "JPY", amountMinor: 5_000_000, rate: "0.006",
			// ¥5,000,000 × 0.006 = €30,000.00.
			wantBaseMinor: 3_000_000,
			why:           "the other zero-decimal currency, so the fix is not VND-shaped",
		},
		{
			currency: "KWD", amountMinor: 1_000_000, rate: "3.25",
			// KWD has THREE decimals: 1,000,000 fils = 1,000 dinar × 3.25 = €3,250.00.
			wantBaseMinor: 325_000,
			why:           "a THREE-decimal currency, which fails in the opposite direction",
		},
		{
			currency: "USD", amountMinor: 20_000, rate: "0.9",
			// The case a scale bug still gets right, so it cannot be the only one.
			wantBaseMinor: 18_000,
			why:           "two decimals against two, where both scales cancel",
		},
	}

	for _, c := range cases {
		t.Run(c.currency, func(t *testing.T) {
			e := Setup(t)
			st := seedForecastFXPipeline(t, e)
			at := midQuarter
			seedForecastFXDeal(t, e, st, c.amountMinor, c.currency, midQuarter)
			e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
				VALUES ($1, 'EUR', $2::numeric, DATE '2020-01-01')`, c.currency, c.rate)

			deals := forecastDeals(t, e, at)
			if len(deals) != 1 {
				t.Fatalf("the forecast read %d deals, want the 1 seeded", len(deals))
			}
			got := deals[0]
			if got.BaseMinor == nil {
				t.Fatalf("%s arrived with no base amount, so it is excluded from every "+
					"total rather than converted (%s)", c.currency, c.why)
			}
			if *got.BaseMinor != c.wantBaseMinor {
				t.Errorf("%d %s at %s converted to %d, want %d — %s",
					c.amountMinor, c.currency, c.rate, *got.BaseMinor, c.wantBaseMinor, c.why)
			}
		})
	}
}

// The forecast reads the SHARED expression, and both answer the literal figure.
//
// Not a Go-versus-SQL comparison despite what the forecast's own path suggests:
// ForecastDeals reaches BaseValueSQL, so the two readings below are the same
// expression twice. What it holds is that the forecast has not grown a second,
// private conversion, and that the shared one answers €84,000 for the deal that
// read €840 before. The Go engine's agreement is held where it can be — the
// minor-unit parity test, and the closed-deal case below, which converts
// through deals.ConvertToBase in the writer.
func TestTheForecastReadsTheSharedExpression(t *testing.T) {
	e := Setup(t)
	st := seedForecastFXPipeline(t, e)
	seedForecastFXDeal(t, e, st, 2_400_000_000, "VND", midQuarter)
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('VND', 'EUR', 0.000035, DATE '2020-01-01')`)

	deals := forecastDeals(t, e, midQuarter)
	if len(deals) != 1 || deals[0].BaseMinor == nil {
		t.Fatalf("the forecast did not convert the seeded deal: %+v", deals)
	}
	fromSQL := *deals[0].BaseMinor

	// The same deal through the SQL expression the morning brief and the report
	// engine share, read directly so a change to either spelling shows up here.
	var fromExpr int64
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT `+
			compose.BaseValueSQL("$1", "'EUR'", "d")+
			` FROM deal d WHERE d.currency = 'VND'`,
			time.Now().UTC()).Scan(&fromExpr)
	})
	if err != nil {
		t.Fatalf("reading the shared base-value expression: %v", err)
	}
	if fromSQL != fromExpr {
		t.Errorf("the forecast says %d and the shared expression says %d for one deal",
			fromSQL, fromExpr)
	}
	if fromExpr != 8_400_000 {
		t.Errorf("the shared expression converted to %d, want 8400000 (€84,000)", fromExpr)
	}
}

// A CLOSED deal's figure is frozen by the writer, and it applies both scales
// too.
//
// The open cases above reach only the live-rate arm of the expression. The
// frozen arm reads a column the freeze writer fills, so a writer that stored a
// hundredth of the truth would leave every case above passing while every
// closed foreign-currency deal on the installation carried a wrong number —
// which is the state this whole change exists to leave behind.
//
// Seeded through AdvanceDeal, the real writer, rather than an INSERT of the
// column: a test that supplies its own version of production proves nothing
// about production.
func TestClosingADealFreezesTheScaledAmount(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()

	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Hà Nội rollout", pipeline, open, &e.Rep1))
	amount := int64(2_400_000_000)
	currency := "VND"
	if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{
		AmountMinor: &amount, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal in VND: %v", err)
	}
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('VND', 'EUR', 0.000035, DATE '2020-01-01')`)

	if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	}); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}

	var frozen *int64
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin,
			`SELECT amount_minor_base FROM deal WHERE id = $1`, deal).Scan(&frozen)
	}); err != nil {
		t.Fatalf("reading the frozen base amount: %v", err)
	}
	if frozen == nil {
		t.Fatal("a closed VND deal with a loaded rate froze no base amount, so every " +
			"rollup that sums the column reads it as contributing nothing")
	}
	// ₫2,400,000,000 at 0.000035 is €84,000.00. The unscaled product would be
	// 84,000 — €840.00, a hundredth of it.
	if *frozen != 8_400_000 {
		t.Errorf("the close froze %d, want 8400000 (€84,000)", *frozen)
	}

	// Reopening takes the figure with the rate it was converted at. Left
	// behind, it would price a close that no longer exists.
	if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{ToStageID: open}); err != nil {
		t.Fatalf("reopening the deal: %v", err)
	}
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin,
			`SELECT amount_minor_base FROM deal WHERE id = $1`, deal).Scan(&frozen)
	}); err != nil {
		t.Fatalf("re-reading the base amount after reopen: %v", err)
	}
	if frozen != nil {
		t.Errorf("a reopened deal still carries a frozen base amount of %d", *frozen)
	}
}
