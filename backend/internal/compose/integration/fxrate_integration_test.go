// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The fx_rate editor suite. Writes are append-forward: today or later, a
// same-day correction, and history extended only before the earliest rate.
// It also covers the admin/ops read+write gate and cross-tenant isolation.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func fxRateOf(rows []deals.FxRateRow, from string) (string, bool) {
	for _, r := range rows {
		if r.FromCurrency == from {
			return r.Rate, true
		}
	}
	return "", false
}

// fxTestNow is a fixed instant the fx suite pins the store clock to, so the
// past-date guard and the test's "today" derive from ONE sample — a real clock
// on both sides could straddle UTC midnight and reject a same-day write.
var fxTestNow = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

func TestFxRateAppendForward(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	ctx := e.Admin()
	today := fxTestNow.Truncate(24 * time.Hour)

	r1, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "usd", Rate: "0.9150", EffectiveDate: today})
	if err != nil {
		t.Fatalf("set USD: %v", err)
	}
	if r1.FromCurrency != "USD" || r1.ToCurrency != "EUR" {
		t.Fatalf("got %+v, want USD→EUR", r1)
	}

	// Same installation-zone day → corrects the row in place (one row survives).
	if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.9200", EffectiveDate: today}); err != nil {
		t.Fatalf("correct USD: %v", err)
	}
	hist, err := e.Deals.FxRateHistory(ctx, "USD")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history len = %d, want 1 (same-day correction)", len(hist))
	}
	if hist[0].Rate != "0.9200000000" {
		t.Fatalf("rate = %q, want 0.9200000000", hist[0].Rate)
	}

	// Future date → a new effective-dated row.
	if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.9300", EffectiveDate: today.AddDate(0, 0, 1)}); err != nil {
		t.Fatalf("future USD: %v", err)
	}
	latest, err := e.Deals.ListLatestFxRates(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if rate, ok := fxRateOf(latest, "USD"); !ok || rate != "0.9300000000" {
		t.Fatalf("latest USD = %q (ok=%v), want 0.9300000000", rate, ok)
	}
}

func TestFxRateRejectsPastBaseAndNonPositive(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	ctx := e.Admin()
	today := fxTestNow.Truncate(24 * time.Hour)

	assertInvalid := func(t *testing.T, err error) {
		t.Helper()
		if _, ok := errors.AsType[*deals.FxRateValidationError](err); !ok {
			t.Fatalf("expected FxRateValidationError, got %v", err)
		}
	}
	if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.9", EffectiveDate: today.AddDate(0, 0, -5)}); err != nil {
		t.Fatalf("seed USD: %v", err)
	}
	_, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.9", EffectiveDate: today.AddDate(0, 0, -1)})
	assertInvalid(t, err) // past, and after the earliest rate
	_, err = e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "EUR", Rate: "1", EffectiveDate: today})
	assertInvalid(t, err) // from == base
	_, err = e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0", EffectiveDate: today})
	assertInvalid(t, err) // not > 0
}

// A past date extends the history only backwards. Before the currency's
// earliest rate it prices days that had none. Anywhere later it would change a
// rate some day was already priced at.
func TestFxRateHistoryExtendsOnlyBeforeTheEarliestRate(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	ctx := e.Admin()
	today := fxTestNow.Truncate(24 * time.Hour)
	setOn := func(rate string, day time.Time) error {
		_, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "JPY", Rate: rate, EffectiveDate: day})
		return err
	}
	refused := func(t *testing.T, what string, err error) {
		t.Helper()
		invalid, ok := errors.AsType[*deals.FxRateValidationError](err)
		if !ok || invalid.Code != "fx_rate_past" {
			t.Errorf("%s = %v, want the fx_rate_past refusal", what, err)
		}
	}

	if err := setOn("0.0063", today.AddDate(0, 0, -10)); err != nil {
		t.Fatalf("a first rate for a currency, dated in the past: %v", err)
	}
	if err := setOn("0.0062", today.AddDate(0, -1, 0)); err != nil {
		t.Fatalf("a rate before the earliest one: %v", err)
	}
	refused(t, "a past date between two stored rates", setOn("0.0070", today.AddDate(0, 0, -20)))
	refused(t, "a past date after the latest rate", setOn("0.0070", today.AddDate(0, 0, -1)))
	refused(t, "the earliest rate's own day", setOn("0.0070", today.AddDate(0, -1, 0)))
	if err := setOn("0.0060", today.AddDate(0, -2, 0)); err != nil {
		t.Fatalf("a rate before the new earliest one: %v", err)
	}

	hist, err := e.Deals.FxRateHistory(ctx, "JPY")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var got []string
	for _, row := range hist {
		got = append(got, row.RateDate.Format(time.DateOnly)+"="+row.Rate)
	}
	want := []string{
		today.AddDate(0, 0, -10).Format(time.DateOnly) + "=0.0063000000",
		today.AddDate(0, -1, 0).Format(time.DateOnly) + "=0.0062000000",
		today.AddDate(0, -2, 0).Format(time.DateOnly) + "=0.0060000000",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("history = %v, want %v (the refused writes changed nothing)", got, want)
	}
}

func TestFxRateWriteDeniedForNonAdmin(t *testing.T) {
	e := Setup(t)
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	_, err := e.Deals.SetFxRate(repCtx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.9", EffectiveDate: time.Now().UTC()})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep write err = %v, want ErrPermissionDenied", err)
	}
}

func TestFxRateReadDeniedForNonAdmin(t *testing.T) {
	e := Setup(t)
	roCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, ReadOnlyPerms)
	if _, err := e.Deals.ListLatestFxRates(roCtx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read_only list err = %v, want ErrPermissionDenied", err)
	}
}

func TestFxRateWritesAuditRow(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	if _, err := e.Deals.SetFxRate(e.Admin(), deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.9", EffectiveDate: fxTestNow}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='fx_rate' AND action='create'`); n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
}

// fxRatePerms is a config-editing principal narrowed to ONE fx_rate grant row.
// The matrix distinguishes inserting a new (currency, day) from overwriting an
// existing one; only a principal missing one of the two grants can prove that
// the endpoint actually asks for the right one.
func fxRatePerms(g principal.ObjectGrant) principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"ops"},
		Objects: map[string]principal.ObjectGrant{
			"fx_rate": g,
			// The matrix narrows fx_rate and ONLY fx_rate. Writing a rate
			// resolves the base currency it converts into, which is read
			// behind installation_settings — an object every seeded role holds
			// (0191), so withholding it here would refuse for a reason the
			// matrix is not about.
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}
}

// One endpoint, two grants: it inserts under `create` and overwrites under
// `update`, and neither grant substitutes for the other. Every refusal is the
// 403 sentinel, and a refused write leaves the sheet untouched.
func TestFxRateCreateAndUpdateGrantsGateSeparately(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	today := fxTestNow.Truncate(24 * time.Hour)
	setOn := func(ctx context.Context, rate string, day time.Time) error {
		_, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{
			FromCurrency: "USD", Rate: rate, EffectiveDate: day,
		})
		return err
	}
	creator := e.As(e.Rep1, nil, fxRatePerms(principal.ObjectGrant{Create: true, Read: true}))
	updater := e.As(e.Rep1, nil, fxRatePerms(principal.ObjectGrant{Update: true, Read: true}))

	// Nothing on the sheet for (USD, today): there is no rate to update, so
	// the update-only principal is refused the insert.
	if err := setOn(updater, "0.9000", today); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("update-only insert = %v, want ErrPermissionDenied", err)
	}
	if err := setOn(creator, "0.9100", today); err != nil {
		t.Fatalf("create-only insert: %v", err)
	}
	// The row exists now, so the SAME call is an overwrite — which holding
	// create alone must not buy.
	if err := setOn(creator, "0.9200", today); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("create-only overwrite = %v, want ErrPermissionDenied", err)
	}
	if err := setOn(updater, "0.9300", today); err != nil {
		t.Fatalf("update-only overwrite: %v", err)
	}

	hist, err := e.Deals.FxRateHistory(e.Admin(), "USD")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 || hist[0].Rate != "0.9300000000" {
		t.Fatalf("sheet = %+v, want one row at 0.9300000000 (the refused writes wrote nothing)", hist)
	}

	// Holding both does both halves: a new day inserts, the same day overwrites.
	both := e.As(e.Rep1, nil, fxRatePerms(principal.ObjectGrant{Create: true, Read: true, Update: true}))
	if err := setOn(both, "0.9400", today.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("both-grants insert on a new day: %v", err)
	}
	if err := setOn(both, "0.9500", today); err != nil {
		t.Fatalf("both-grants overwrite: %v", err)
	}
}

// An overwrite is audited as the verb that admitted it, so
// audit_log.authorization_rule attributes the update grant rather than the
// create grant the insert used.
func TestFxRateOverwriteAuditsAsUpdate(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	ctx := e.Admin()
	for _, rate := range []string{"0.9", "0.95"} {
		if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{
			FromCurrency: "USD", Rate: rate, EffectiveDate: fxTestNow,
		}); err != nil {
			t.Fatalf("set %s: %v", rate, err)
		}
	}
	for action, want := range map[string]int{"create": 1, "update": 1} {
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log
			WHERE entity_type='fx_rate' AND action='`+action+`'`); n != want {
			t.Fatalf("audit rows for %s = %d, want %d", action, n, want)
		}
	}
	// The update audit carries what it displaced — an overwrite whose before
	// image is null is an unusable ledger entry.
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type='fx_rate' AND action='update' AND before->>'rate' = '0.9000000000'`); n != 1 {
		t.Fatalf("update audit rows carrying the displaced rate = %d, want 1", n)
	}
}
