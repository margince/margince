// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Close-date hygiene over real migrated Postgres (formulas §11/§12,
// B-E09.19/.20): the write layer rejects a past close date on an open
// deal at source; the forecast drops flagged deals out of
// Commit/Best-case; and the nightly corrector applies the A6 tiers —
// after a run, no open deal is left claiming a past close date
// (INV-CLOSE-PAST), and a provisional replacement stays excluded until
// a human confirms it through the approvals inbox.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// closeDateEnv wraps integration.Env with a two-open-stage pipeline whose
// probabilities pin the §11 tier judgments (20% early, 60% late).
type closeDateEnv struct {
	*integration.Env
	owner     *pgx.Conn
	pipeline  ids.UUID
	early     ids.UUID // 20%, position 0
	late      ids.UUID // 60%, position 1
	corrector *deals.CloseDateCorrector
	svc       *approvals.Service
	// day is the date the sweep believes it is running on. It starts at the real
	// today, so every test that never touches it runs exactly as before; a test
	// about what TOMORROW's pass does calls nextDay, which is the only way to get
	// there — the proposed date is computed from "today", so two passes in one
	// process otherwise always land on the same day.
	day time.Time
}

// nextDay moves the sweep's clock forward one day. The database keeps real
// time, and that is safe here: the SQL pre-filter is a deliberate superset
// (anything dated within the stalled window, missing, or provisional), so a
// clock one day ahead only widens what the Go assessment then judges.
func (e *closeDateEnv) nextDay() {
	e.day = e.day.AddDate(0, 0, 1)
}

func setupCloseDate(t *testing.T) *closeDateEnv {
	t.Helper()
	e := &closeDateEnv{Env: integration.Setup(t), owner: integration.OwnerConn(t), day: time.Now()}
	e.pipeline = integration.SeedIDRow(t, e.owner,
		`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Hygiene', true, 0)`)
	ctx := context.Background()
	for _, stage := range []struct {
		id          *ids.UUID
		position    int
		probability int
	}{{&e.early, 0, 20}, {&e.late, 1, 60}} {
		*stage.id = ids.NewV7()
		if _, err := e.owner.Exec(ctx,
			`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
			 VALUES ($1, $2, $3, $4, 'open', $5)`,
			*stage.id, e.pipeline, fmt.Sprintf("Stage %d", stage.position), stage.position, stage.probability); err != nil {
			t.Fatal(err)
		}
	}
	quiet := slog.New(slog.NewTextHandler(os.Stderr, nil))
	e.svc = approvals.NewService(e.DB())
	e.svc.WithEffect(deals.CloseDateCorrectionKind, closeDateConfirmEffect(e.svc, deals.NewStore(e.DB(), DealsInstallation())))
	// The policy production builds, not a stub that always says yes: whether a
	// rep has left close dates to the sweep is exactly what these tests are
	// about, and a harness answering it itself would prove nothing.
	owner := dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())}
	e.corrector = deals.NewCloseDateCorrector(e.DB(),
		closeDatePolicy{svc: e.svc, owner: owner},
		quietReviewReader{db: e.DB(), owner: owner}, quiet, installseam.Deals()).
		WithClock(func() time.Time { return e.day })
	return e
}

// sweep runs the corrector over this env's workspace under exactly the scope
// the close_date_workspace worker binds — the pass is per workspace now, and a
// suite that bound its own scope would be proving a path the product does not
// run.
func (e *closeDateEnv) sweep() error {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: closeDateSweepActor})
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return e.corrector.SweepWorkspace(ctx)
}

// seedSweepDeal plants one deal through the owner connection — exactly
// the aged/migrated rows the write layer never saw and the nightly run
// must still clean.
func (e *closeDateEnv) seedSweepDeal(t *testing.T, name string, stage ids.UUID, category *string, closeInDays *int, lastActivityDaysAgo int) ids.UUID {
	t.Helper()
	var expectedClose *time.Time
	if closeInDays != nil {
		v := today().AddDate(0, 0, *closeInDays)
		expectedClose = &v
	}
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		// The deal carries an owner because the gone-quiet review reads its
		// correspondence under that person's authority — an unowned deal is
		// reviewed unnamed, which is a case its own test covers.
		`INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, forecast_category, expected_close_date, last_activity_at, created_at, source, captured_by, owner_id)
		 VALUES ($1, $2, $3, $4, 10000, 'EUR', $5, $6,
		         now() - make_interval(days => $7), now() - interval '120 days', 'manual', 'human:x', $8)`,
		id, name, e.pipeline, stage, category, expectedClose, lastActivityDaysAgo, e.Rep1); err != nil {
		t.Fatalf("seeding deal %q: %v", name, err)
	}
	return id
}

type sweptDeal struct {
	expectedClose *time.Time
	provisional   bool
	forecastCat   *string
}

func (e *closeDateEnv) readSwept(t *testing.T, id ids.UUID) sweptDeal {
	t.Helper()
	var d sweptDeal
	if err := e.owner.QueryRow(context.Background(),
		`SELECT expected_close_date, close_date_provisional, forecast_category FROM deal WHERE id = $1`,
		id).Scan(&d.expectedClose, &d.provisional, &d.forecastCat); err != nil {
		t.Fatal(err)
	}
	return d
}

func (e *closeDateEnv) pendingCorrections(t *testing.T, dealID ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'close_date_correction' AND target_entity_id = $1 AND status = 'pending'`,
		dealID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *closeDateEnv) runForecastReport(t *testing.T, ctx context.Context, body string) reportResultWire {
	t.Helper()
	handlers := reportHandlers{engine: newReportEngine(e.Pool)}
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/forecast", strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handlers.RunReport(rec, req, "forecast")
	var result reportResultWire
	decodeWire(t, rec, http.StatusOK, &result)
	return result
}

func today() time.Time {
	y, m, d := time.Now().UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func intp(v int) *int { return &v }

// --- B-E09.19(a): the write layer rejects the invalid state at source ---

func TestCloseDatePastRejectedOnOpenDealWrites(t *testing.T) {
	e := setupCloseDate(t)
	admin := e.Admin()
	yesterday := today().AddDate(0, 0, -1)
	tomorrow := today().AddDate(0, 0, 1)

	_, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Born invalid", PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.early), Source: "manual",
		ExpectedClose: &yesterday,
	})
	var pastClose *deals.PastCloseDateError
	if !errors.As(err, &pastClose) {
		t.Fatalf("create with a past close date → %v, want PastCloseDateError", err)
	}

	closingToday := today()
	d, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Closing today is fine", PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.early), Source: "manual",
		ExpectedClose: &closingToday,
	})
	if err != nil {
		t.Fatalf("create closing today: %v (overdue is strict <)", err)
	}

	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.UpdateDealInput{ExpectedClose: &yesterday}); !errors.As(err, &pastClose) {
		t.Fatalf("update to a past close date → %v, want PastCloseDateError", err)
	}
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(d.Id)), deals.UpdateDealInput{ExpectedClose: &tomorrow}); err != nil {
		t.Fatalf("update to tomorrow: %v", err)
	}
}

// --- B-E09.19(b): flagged deals drop out of Commit/Best-case (AC-F9) ---

func TestForecastExcludesFlaggedDealsFromCommitAndBestCase(t *testing.T) {
	e := setupCloseDate(t)

	e.seedSweepDeal(t, "Healthy commit", e.late, stringp("commit"), intp(30), 3)
	e.seedSweepDeal(t, "Overdue commit", e.late, stringp("commit"), intp(-10), 3)
	e.seedSweepDeal(t, "Dateless commit", e.late, stringp("commit"), nil, 3)
	provisional := e.seedSweepDeal(t, "Provisional best case", e.late, stringp("best_case"), intp(30), 3)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET close_date_provisional = true WHERE id = $1`, provisional); err != nil {
		t.Fatal(err)
	}

	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	result := e.runForecastReport(t, ctx, `{"group_by":["forecast_category","currency"]}`)

	counts := map[string]int64{}
	for _, row := range result.Rows {
		key, _ := row["forecast_category"].(string)
		counts[key] = wireInt(t, row, "deals")
	}
	if counts["commit"] != 1 {
		t.Errorf("commit deals = %d, want only the healthy one", counts["commit"])
	}
	if counts["best_case"] != 0 {
		t.Errorf("best_case deals = %d, want 0 — the provisional date must stay excluded", counts["best_case"])
	}
	if counts["slipped"] != 3 {
		t.Errorf("slipped deals = %d, want the overdue + dateless + provisional trio", counts["slipped"])
	}

	// The filter rides the same expression: asking for commit returns the
	// healthy deal alone, so a drill-through can never resurrect a
	// flagged deal into the number it was excluded from.
	filtered := e.runForecastReport(t, ctx, `{"filters":{"forecast_category":"commit"},"group_by":["forecast_category","currency"]}`)
	if len(filtered.Rows) != 1 || wireInt(t, filtered.Rows[0], "deals") != 1 {
		t.Fatalf("filter commit → %+v, want exactly the one healthy deal", filtered.Rows)
	}
}

// --- B-E09.20: the A6 tiers ---

func TestCloseDateSweepRollsClearOverdueActiveDealProvisionally(t *testing.T) {
	e := setupCloseDate(t)
	// Early stage (20%), plainly overdue, touched 3 days ago, no forecast
	// override → the §11 worked example's 🟢 case. Two open stages remain
	// from position 0, velocity falls back to 14 → today + 28.
	id := e.seedSweepDeal(t, "Slipped but alive", e.early, nil, intp(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	want := today().AddDate(0, 0, 2*deals.CloseDateStageDays)
	if swept.expectedClose == nil || !swept.expectedClose.Equal(want) {
		t.Errorf("auto-rolled date = %v, want %s (2 stages × 14-day fallback)", swept.expectedClose, want.Format(time.DateOnly))
	}
	// The rolled date is a stage-velocity estimate, so it lands PROVISIONAL
	// and asks. The 🟢 tier buys promptness — the deal stops claiming a date
	// that has passed, tonight, without waiting for a human — not the claim
	// that a buyer agreed to the replacement. Nothing on this path read a
	// buyer's message, and a date nobody confirmed must not enter supported
	// forecast claims looking confirmed.
	if !swept.provisional {
		t.Error("the auto-rolled date is a velocity estimate — it must be provisional")
	}
	if got := e.pendingCorrections(t, id); got != 0 {
		t.Errorf("the tier staged %d cards; an estimate is APPLIED and announced on the "+
			"morning receipt, where the rep can put it back", got)
	}

	// Reversibility: the audit row carries the exact before/after images.
	var before, after string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT before::text, after::text FROM audit_log
		 WHERE entity_type = 'deal' AND entity_id = $1 AND action = 'update'
		 ORDER BY occurred_at DESC LIMIT 1`, id).Scan(&before, &after); err != nil {
		t.Fatalf("no audit row for the overnight change: %v", err)
	}
	if !strings.Contains(before, "expected_close_date") || !strings.Contains(after, want.Format(time.DateOnly)) {
		t.Errorf("audit diff (before %s, after %s) does not carry the rollback images", before, after)
	}
}

// A slipped close date is, by deal_forecast_history's own reckoning, the single
// most common reason a real forecast moves — and the nightly corrector is the
// writer that slips the most of them. It moves the date without touching the
// stage, which is exactly the move deal_stage_history cannot see, so a
// reconstruction blind to the sweep reconciles over human edits and silently
// omits every machine re-date while presenting itself as the whole answer.
//
// The row is stamped with the corrector's own principal rather than a human's:
// changed_by exists to say who moved it, and "nobody" is not one of the answers.
func TestCloseDateSweepRecordsTheForecastItMoved(t *testing.T) {
	e := setupCloseDate(t)
	// The 🟢 worked example again: overdue, active, no override — the tier that
	// re-dates the deal outright, so the move is the sweep's and nothing else's.
	id := e.seedSweepDeal(t, "Slipped but alive", e.early, nil, intp(-12), 3)

	forecastRows := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM deal_forecast_history WHERE deal_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := forecastRows(t); got != 0 {
		t.Fatalf("the seed wrote %d forecast rows, want 0 — it plants the row directly", got)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	want := today().AddDate(0, 0, 2*deals.CloseDateStageDays)
	if swept := e.readSwept(t, id); swept.expectedClose == nil || !swept.expectedClose.Equal(want) {
		t.Fatalf("the sweep did not re-date the deal (%v), so the rest of this test proves nothing", swept.expectedClose)
	}
	if got := forecastRows(t); got != 1 {
		t.Fatalf("the sweep wrote %d forecast rows, want 1 — it moved the close date and a "+
			"reconstruction of the forecast as of any date after the run would read the overdue one", got)
	}

	var recordedDate *time.Time
	var changedBy string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT close_date_at_change, changed_by FROM deal_forecast_history WHERE deal_id = $1`,
		id).Scan(&recordedDate, &changedBy); err != nil {
		t.Fatal(err)
	}
	if recordedDate == nil || !recordedDate.Equal(want) {
		t.Errorf("recorded close date = %v, want %s — the row carries the date the deal now has",
			recordedDate, want.Format(time.DateOnly))
	}
	// closeDateSweepActor is what the WORKER binds (jobs_deals.go), so this
	// asserts the attribution production actually writes rather than a spelling
	// the fixture chose for itself.
	if changedBy != closeDateSweepActor {
		t.Errorf("changed_by = %q, want %q — the recorder takes the running principal, so a "+
			"machine re-date is attributable rather than anonymous history", changedBy, closeDateSweepActor)
	}
}

// A commit-bearing deal is re-dated provisionally and drops out of Commit.
//
// It used to raise a card as well, and the card is what this change removes:
// the sweep had already written the date, so asking a rep to confirm it was a
// question whose answer changed nothing. The forecast exclusion is the part
// that always mattered — a provisional date is a machine's estimate, and the
// number must not count it.
func TestCloseDateSweepRedatesAndExcludesAForecastBearingDeal(t *testing.T) {
	e := setupCloseDate(t)
	// Explicit commit + late stage: overdue, active — never auto-final.
	id := e.seedSweepDeal(t, "Commit slipped", e.late, stringp("commit"), intp(-10), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if swept.expectedClose == nil || swept.expectedClose.Before(today()) {
		t.Fatalf("provisional date = %v — INV-CLOSE-PAST must hold immediately", swept.expectedClose)
	}
	if !swept.provisional {
		t.Error("🟡 replacement must be provisional until a human confirms")
	}
	if swept.forecastCat == nil || *swept.forecastCat != "commit" {
		t.Errorf("forecast_category = %v, want the untouched commit override (the number moves by exclusion, not by edit)", swept.forecastCat)
	}
	if got := e.pendingCorrections(t, id); got != 0 {
		t.Errorf("the sweep raised %d cards; the correction is applied and reported "+
			"on the morning receipt, so there is nothing to confirm", got)
	}

	// Excluded from Commit while provisional (AC-F9): the commit filter
	// matches nothing, so the aggregate has no group row at all.
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	result := e.runForecastReport(t, ctx, `{"filters":{"forecast_category":"commit"}}`)
	if len(result.Rows) != 0 {
		t.Errorf("commit rows while provisional = %+v, want none", result.Rows)
	}

	// A second nightly run leaves a corrected deal alone: its date is no longer
	// flagged, so there is nothing to correct twice.
	before := e.readSwept(t, id)
	e.nextDay()
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if after := e.readSwept(t, id); !sameDay(after.expectedClose, before.expectedClose) {
		t.Errorf("a second sweep moved the date again, from %v to %v",
			dayOf(before.expectedClose), dayOf(after.expectedClose))
	}
}

// A card left over from before this change can still be confirmed.
//
// The sweep no longer stages one, but an installation upgrading mid-week has
// pending cards its last nightly run raised, and each names a deal standing on
// a provisional date. The confirm effect stays registered for exactly them:
// dropping it would leave those deals provisional forever, with the only verb
// that could settle them gone.
//
// Staged directly here, because the pass that used to make one does not.
func TestALeftoverCardStillConfirmsTheDateAndClearsProvisional(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Confirm me", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	swept := e.readSwept(t, id)
	if !swept.provisional || swept.expectedClose == nil {
		t.Fatalf("the sweep left no provisional date to confirm: %+v", swept)
	}
	approvalID := e.stageLegacyCard(t, id, *swept.expectedClose)

	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := e.svc.Decide(human, approvalID, true, nil); err != nil {
		t.Fatalf("approve + effect: %v", err)
	}

	swept = e.readSwept(t, id)
	if swept.provisional {
		t.Error("confirmation must clear close_date_provisional")
	}
	if swept.expectedClose == nil || swept.expectedClose.Before(today()) {
		t.Errorf("confirmed date = %v, want the proposed future date", swept.expectedClose)
	}

	// Confirmed: the deal counts in Commit again.
	result := e.runForecastReport(t, human, `{"filters":{"forecast_category":"commit"}}`)
	if len(result.Rows) != 1 || wireInt(t, result.Rows[0], "deals") != 1 {
		t.Errorf("commit total after confirm = %+v, want the deal back", result.Rows)
	}
}

// A deal nobody has touched is notched down AND re-dated.
//
// It used to keep its date: only the invariant forced one onto a quiet deal,
// on the reading that an optimistic re-date on top of a downgrade said too
// much. That left the forecast corrected and the calendar lying, which is the
// half a rep actually reads. Both move now, both are the sweep's estimate, and
// both are on one Undo.
func TestCloseDateSweepDowngradesAndRedatesAQuietDeal(t *testing.T) {
	e := setupCloseDate(t)
	// Quiet 90 days, commit override, date still future but inside the
	// stalled window (unrealistic_stale) → 🔻: one forecast notch down,
	// the date untouched — the zombie guard.
	id := e.seedSweepDeal(t, "Gone quiet", e.late, stringp("commit"), intp(30), 90)
	originalDate := today().AddDate(0, 0, 30)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if swept.forecastCat == nil || *swept.forecastCat != "best_case" {
		t.Errorf("forecast_category = %v, want best_case (one notch down from commit)", swept.forecastCat)
	}
	if swept.expectedClose == nil || swept.expectedClose.Equal(originalDate) {
		t.Errorf("date = %v, still the original %s — a deal nobody has touched carries "+
			"a date nobody believes, and notching the forecast while leaving the "+
			"calendar alone corrects the number and leaves the date lying",
			swept.expectedClose, originalDate.Format(time.DateOnly))
	}
	if !swept.provisional {
		t.Error("the replacement is the sweep's own estimate and must say so")
	}
	if got := e.pendingCorrections(t, id); got != 0 {
		t.Errorf("the gone-quiet tier staged %d cards; both the notch and the date "+
			"are applied, and one receipt carries them with one way back", got)
	}
}

func TestCloseDateSweepDowngradesQuietOverdueDealWithProvisionalDate(t *testing.T) {
	e := setupCloseDate(t)
	// Quiet AND overdue: the invariant forces a replacement date, but it
	// lands provisional and the category still notches down.
	id := e.seedSweepDeal(t, "Quiet and overdue", e.late, stringp("best_case"), intp(-20), 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if swept.forecastCat == nil || *swept.forecastCat != "pipeline" {
		t.Errorf("forecast_category = %v, want pipeline (one notch down from best_case)", swept.forecastCat)
	}
	if swept.expectedClose == nil || swept.expectedClose.Before(today()) {
		t.Errorf("date = %v — the invariant still demands a non-past date", swept.expectedClose)
	}
	if !swept.provisional {
		t.Error("the forced replacement on a quiet deal must be provisional, never an optimistic re-date")
	}
}

// AC-F9 / §12 rule 5, the hard invariant: whatever mix of tiers the
// night starts with, no open deal survives the run with a past close
// date — while closed deals keep their historical dates untouched.
func TestCloseDateSweepLeavesNoOpenDealWithPastCloseDate(t *testing.T) {
	e := setupCloseDate(t)
	e.seedSweepDeal(t, "Auto tier", e.early, nil, intp(-12), 3)
	e.seedSweepDeal(t, "Provisional tier", e.late, stringp("commit"), intp(-10), 3)
	e.seedSweepDeal(t, "Downgrade tier", e.late, stringp("commit"), intp(-20), 90)
	e.seedSweepDeal(t, "Dateless", e.late, stringp("commit"), nil, 3)
	won := e.seedSweepDeal(t, "Won long ago", e.late, nil, intp(-100), 3)
	// Amountless close: the deal_closed_fx CHECK only demands a frozen
	// rate when a closed deal carries money, which is beside this point.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET status = 'won', closed_at = now() - interval '100 days',
		   amount_minor = NULL, currency = NULL WHERE id = $1`, won); err != nil {
		t.Fatal(err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	var openPast int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM deal WHERE status = 'open' AND archived_at IS NULL
		   AND expected_close_date < current_date`).Scan(&openPast); err != nil {
		t.Fatal(err)
	}
	if openPast != 0 {
		t.Errorf("%d open deal(s) survived the nightly run with a past close date — INV-CLOSE-PAST broken", openPast)
	}
	var openMissing int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM deal WHERE status = 'open' AND archived_at IS NULL
		   AND expected_close_date IS NULL`).Scan(&openMissing); err != nil {
		t.Fatal(err)
	}
	if openMissing != 0 {
		t.Errorf("%d open deal(s) still dateless after the run", openMissing)
	}
	if got := e.readSwept(t, won); got.expectedClose == nil || !got.expectedClose.Before(today()) {
		t.Errorf("the won deal's historical date changed to %v — closed deals are never flagged", got.expectedClose)
	}
}

// A deal explicitly asked to wait is not "gone dark" (§11 edge case):
// the wait suppresses the 🔻 quiet branch, but its past date still takes
// the 🟡 provisional path — a paused deal must not claim a past date.
func TestCloseDateSweepWaitUntilSuppressesDowngradeButNotOverdue(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Paused politely", e.early, nil, intp(-5), 90)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET wait_until = current_date + 60 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if swept.forecastCat != nil {
		t.Errorf("forecast_category = %v, want untouched NULL — the wait suppresses the downgrade", swept.forecastCat)
	}
	if swept.expectedClose == nil || swept.expectedClose.Before(today()) || !swept.provisional {
		t.Errorf("(date, provisional) = (%v, %v) — the past date must be replaced provisionally", swept.expectedClose, swept.provisional)
	}
}

// The starvation the frozen membership exists to end.
//
// The old candidate query was `ORDER BY created_at, id LIMIT 200` over the live
// deal table, with no cursor and no record of a pass. A corrected deal stays
// eligible — the sweep leaves it provisional, and provisional is one of the
// three conditions that admit a deal — so the same oldest 200 rows re-qualified
// every night, the LIMIT cut at the same place, and deal 201 was never reached.
// Not assessed late: not assessed at all, on any night, forever.
//
// So the fixture is deliberately shaped like the real failure: 200 healthy deals
// created FIRST, which the old query would have filled its whole page with, and
// the overdue one created last so it sorts past the cut. A pass that reaches it
// is a pass that walked its whole frozen set.
func TestCloseDateSweepReachesDealsPastTheOldPageLimit(t *testing.T) {
	e := setupCloseDate(t)

	// Healthy near-term dates: eligible for the pre-filter (inside the stalled
	// window), flagged by nothing, so each settles as a plain check.
	healthy := 200
	for i := range healthy {
		e.seedSweepDeal(t, fmt.Sprintf("Healthy %03d", i), e.early, nil, intp(30), 3)
	}
	// Created last, so it sorts behind every one of them.
	overdue := e.seedSweepDeal(t, "Overdue behind the old cut", e.early, nil, intp(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	// The deal the old shape could never see now carries its correction.
	swept := e.readSwept(t, overdue)
	if swept.expectedClose == nil || !swept.expectedClose.After(today()) {
		t.Errorf("the deal past the old page limit still claims %v — it was never assessed",
			swept.expectedClose)
	}
	if !swept.provisional {
		t.Error("its replacement date is an estimate and must be provisional")
	}

	// And the pass can say so: one run, covering everything it froze.
	var (
		eligible, checked int
		status            string
	)
	if err := e.owner.QueryRow(context.Background(),
		`SELECT eligible, checked, status FROM close_date_run ORDER BY started_at DESC LIMIT 1`).
		Scan(&eligible, &checked, &status); err != nil {
		t.Fatalf("the pass recorded no run: %v", err)
	}
	if eligible != healthy+1 {
		t.Errorf("froze %d deals, want the %d seeded", eligible, healthy+1)
	}
	if checked != eligible {
		t.Errorf("checked %d of %d — a complete pass settles its whole set", checked, eligible)
	}
	if status != deals.CloseDateRunComplete {
		t.Errorf("run status = %q, want %q", status, deals.CloseDateRunComplete)
	}
}

// A pass interrupted part-way resumes where it stopped rather than starting
// again at the first row — which is what made the worker's 5-minute deadline a
// permanent cap rather than a pause.
func TestCloseDateSweepResumesAnUnfinishedPass(t *testing.T) {
	e := setupCloseDate(t)
	for i := range 3 {
		e.seedSweepDeal(t, fmt.Sprintf("Overdue %d", i), e.early, nil, intp(-12), 3)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	var runID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM close_date_run ORDER BY started_at DESC LIMIT 1`).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	// Reopen the finished run with one member unsettled and the cursor behind
	// it: exactly the state a killed worker leaves.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE close_date_run SET status = 'running', finished_at = NULL, checked = checked - 1
		  WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	var reopened ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`UPDATE close_date_run_member SET outcome = 'pending', settled_at = NULL
		  WHERE ctid IN (SELECT ctid FROM close_date_run_member
		                  WHERE run_id = $1 ORDER BY deal_created_at DESC, deal_id DESC LIMIT 1)
		 RETURNING deal_id`, runID).Scan(&reopened); err != nil {
		t.Fatal(err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	// The SAME run finished — a second one would mean it restarted rather than
	// resumed, and would have re-corrected every deal it had already handled.
	var runs int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM close_date_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Errorf("close_date_run rows = %d, want 1 — the retry opened a second pass", runs)
	}
	var status string
	var eligible, checked int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status, eligible, checked FROM close_date_run WHERE id = $1`, runID).
		Scan(&status, &eligible, &checked); err != nil {
		t.Fatal(err)
	}
	if status != deals.CloseDateRunComplete || checked != eligible {
		t.Errorf("resumed run: status %q, checked %d of %d — want a completed pass",
			status, checked, eligible)
	}
}

// A deal created after the freeze belongs to the NEXT pass, and one that closes
// mid-pass keeps its place in the ledger with a reason. Both are what make
// "checked N of M" reconcile instead of drifting with the live table.
func TestCloseDateSweepFreezesItsMembershipAtTheStart(t *testing.T) {
	e := setupCloseDate(t)
	settled := e.seedSweepDeal(t, "Archived before the sweep runs", e.early, nil, intp(-12), 3)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET archived_at = now() WHERE id = $1`, settled); err != nil {
		t.Fatal(err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	// The archived deal never entered the frozen set: the freeze reads live
	// open deals, so a deal already gone is not owed an outcome.
	var members int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM close_date_run_member WHERE deal_id = $1`, settled).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 0 {
		t.Errorf("an archived deal joined the frozen set (%d members)", members)
	}

	// A deal created now is not retrofitted into the finished pass.
	fresh := e.seedSweepDeal(t, "Created after the freeze", e.early, nil, intp(-12), 3)
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM close_date_run_member WHERE deal_id = $1`, fresh).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 0 {
		t.Errorf("a deal created after the freeze joined that pass (%d members)", members)
	}
}

// A member that goes away between the freeze and its turn is SKIPPED, not
// dropped. Losing it from the ledger would quietly shrink the denominator, and
// "checked 9 of 9" would be true of a set that used to have ten deals in it.
func TestCloseDateSweepSkipsAMemberArchivedMidPass(t *testing.T) {
	e := setupCloseDate(t)
	doomed := e.seedSweepDeal(t, "Archived after the freeze", e.early, nil, intp(-12), 3)

	// Run once so the deal is frozen into a pass, then reopen that pass with
	// this member unsettled and archive the deal underneath it — the state a
	// concurrent human archive leaves for the resuming walk.
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	var runID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM close_date_run ORDER BY started_at DESC LIMIT 1`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	// Zero the tallies with the members, so what the resumed walk records is
	// the only thing these counters hold.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE close_date_run
		    SET status = 'running', finished_at = NULL, checked = 0, corrected = 0, staged = 0
		  WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE close_date_run_member SET outcome = 'pending', settled_at = NULL
		  WHERE run_id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET archived_at = now() WHERE id = $1`, doomed); err != nil {
		t.Fatal(err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	var outcome string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT outcome FROM close_date_run_member WHERE run_id = $1 AND deal_id = $2`,
		runID, doomed).Scan(&outcome); err != nil {
		t.Fatalf("the archived member left the ledger: %v", err)
	}
	if outcome != "skipped" {
		t.Errorf("outcome = %q, want skipped — it was frozen in, so it is owed an answer", outcome)
	}
	// And the pass still accounts for it: settled, so the run can finish, while
	// staying outside the corrected tally.
	var eligible, checked, corrected int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT eligible, checked, corrected FROM close_date_run WHERE id = $1`, runID).
		Scan(&eligible, &checked, &corrected); err != nil {
		t.Fatal(err)
	}
	if checked != eligible {
		t.Errorf("checked %d of %d — a skipped member is still settled", checked, eligible)
	}
	if corrected != 0 {
		t.Errorf("corrected = %d, want 0 — an archived deal is not corrected", corrected)
	}
}

// The ledger records what the pass DID, not which tier it picked.
//
// The outcome used to be read off the chosen action, so a tier that decided to
// correct and then wrote nothing still counted as a correction. Switching
// maintenance off is the clearest case: every branch stops writing, and a
// receipt built from intentions would report a night's worth of corrections
// that never touched a deal — the same "machine work that changed nothing"
// this ledger exists to expose.
func TestCloseDateRunCountsOnlyWritesThatHappened(t *testing.T) {
	e := setupCloseDate(t)
	e.seedSweepDeal(t, "Overdue but maintenance is off", e.early, nil, intp(-12), 3)

	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO setting (key, value, updated_at) VALUES ($1, 'false'::jsonb, now())
		 ON CONFLICT (key) DO UPDATE SET value = 'false'::jsonb, updated_at = now()`,
		deals.MaintenanceWritesEnabled.Key()); err != nil {
		t.Fatal(err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	var eligible, checked, corrected int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT eligible, checked, corrected FROM close_date_run ORDER BY started_at DESC LIMIT 1`).
		Scan(&eligible, &checked, &corrected); err != nil {
		t.Fatal(err)
	}
	if eligible != 1 || checked != 1 {
		t.Errorf("eligible %d checked %d — the deal is still assessed with writes off", eligible, checked)
	}
	if corrected != 0 {
		t.Errorf("corrected = %d, want 0 — nothing was written, so nothing was corrected", corrected)
	}
	// And the deal really is untouched: still claiming its past date.
	swept := e.readSwept(t, e.firstDealID(t))
	if swept.expectedClose == nil || !swept.expectedClose.Before(today()) {
		t.Errorf("the deal was re-dated to %v with maintenance switched off", swept.expectedClose)
	}
}

// firstDealID is the only deal these single-deal cases seed.
func (e *closeDateEnv) firstDealID(t *testing.T) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM deal ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// stageLegacyCard raises the confirm card the sweep used to raise, for the one
// test that still needs one: an installation upgrading with cards already
// pending.
func (e *closeDateEnv) stageLegacyCard(t *testing.T, dealID ids.UUID, proposed time.Time) ids.ApprovalID {
	t.Helper()
	proposal := deals.CloseDateCorrection{
		DealID:            ids.From[ids.DealKind](dealID),
		ExpectedCloseDate: proposed.Format(time.DateOnly),
		PreviousCloseDate: stringp(proposed.Format(time.DateOnly)),
		Asking:            deals.AskingIsThisDateRight,
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	targetType := approvalTargetDeal
	id, err := e.svc.Stage(e.Admin(), approvals.StageInput{
		Kind:           deals.CloseDateCorrectionKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		TargetType:     targetType,
		TargetID:       dealID,
		Summary:        "Confirm the real close date",
	})
	if err != nil {
		t.Fatalf("staging the leftover card: %v", err)
	}
	return id
}
