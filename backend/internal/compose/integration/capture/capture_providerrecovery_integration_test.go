// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Outage recovery over real Postgres: rows are parked by the writers an outage
// drives (ClaimDue, Defer, RetireExhausted, MarkQueued, ExpireExhausted), never
// by inserting the parked shape, so the recovery is judged against what those
// writers actually leave behind.

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// outageWindow brackets "now" widely enough that clock skew between the test
// process and the database cannot move a freshly parked row out of it.
func outageWindow() capturemod.ReopenWindow {
	now := time.Now()
	return capturemod.ReopenWindow{From: now.Add(-time.Hour), To: now.Add(time.Hour)}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// parkThroughAnOutage spends every pending row's attempts on failed answers and
// retires them, exactly as a provider that never answered does.
func parkThroughAnOutage(t *testing.T, e *integration.SearchEnv, store *capturemod.PendingStore) {
	t.Helper()
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	for range capturemod.PendingMaxAttempts {
		claimed, err := store.ClaimDue(wsCtx, 100)
		if err != nil {
			t.Fatalf("claiming: %v", err)
		}
		for _, p := range claimed {
			if err := store.Defer(wsCtx, p, 0, "provider unavailable", false); err != nil {
				t.Fatalf("deferring: %v", err)
			}
		}
	}
	if _, err := store.RetireExhausted(wsCtx, capturemod.ExhaustedReason); err != nil {
		t.Fatalf("retiring exhausted: %v", err)
	}
}

type ledgerRow struct {
	status   string
	attempts int
}

func ledgerOf(t *testing.T, e *integration.SearchEnv, address string) ledgerRow {
	t.Helper()
	var r ledgerRow
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT status, attempts FROM capture_pending_counterparty
			 WHERE email = $1 ORDER BY created_at DESC LIMIT 1`, address).Scan(&r.status, &r.attempts)
	})
	if err != nil {
		t.Fatalf("reading the ledger row for %s: %v", address, err)
	}
	return r
}

func backdateResolution(t *testing.T, e *integration.SearchEnv, address string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET resolved_at = resolved_at - interval '10 days' WHERE email = $1`, address)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func reopenAudits(t *testing.T, e *integration.SearchEnv, entity string) int {
	t.Helper()
	return countRows(t, e, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = '`+entity+`' AND evidence ? 'reopened_window_from'`)
}

// seedParkedSenders captures five senders and parks them all, then makes four
// of them differ: one parked long before the window, one a human has since
// rejected, one a model judged and could not hold, one with a review offer
// standing. The fifth is the plain case.
func seedParkedSenders(t *testing.T, env captureEnv) {
	t.Helper()
	e := env.e
	store := capturemod.NewPendingStore(e.DB())
	for _, sender := range []string{"plain", "old", "decided", "judged", "offered"} {
		env.sync(t, email(sender+"@stranger.example", sender, captureOwner, "rec"+sender+"@stranger.example", ""))
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)

	// The model-judged row leaves the queue through Retire, not exhaustion.
	claimed, err := store.ClaimDue(wsCtx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range claimed {
		if p.Email == "judged@stranger.example" {
			if err := store.Retire(wsCtx, p, "below the confidence floor on a re-ask", capturemod.VerdictMeasurement{}); err != nil {
				t.Fatal(err)
			}
		} else if err := store.Defer(wsCtx, p, 0, "provider unavailable", false); err != nil {
			t.Fatal(err)
		}
	}
	parkThroughAnOutage(t, e, store)
	backdateResolution(t, e, "old@stranger.example")

	// A stranger's mail is held from colleagues, and the review queue offers
	// only what colleagues may see, so the one sender meant to carry an offer
	// has its message shared first.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity SET audience = 'workspace', audience_reason = NULL
			 WHERE id = (SELECT activity_id FROM capture_pending_counterparty
			              WHERE email = 'offered@stranger.example')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The offer is staged by the verdict engine's own sweep, as in production.
	engine := compose.NewCounterpartyVerdictEngine(e.Pool, nil, compose.CaptureConfig{}, quietLog())
	if err := engine.StageReviewsWorkspace(wsCtx, 0); err != nil {
		t.Fatalf("staging the review offers: %v", err)
	}

	// A human rejects one: the ledger row leaves `unsure` through the real writer.
	var decided ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM capture_pending_counterparty WHERE email = 'decided@stranger.example'`).Scan(&decided)
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return store.ResolveReviewed(context.Background(), tx, decided, capturemod.PendingStatusRejected, "declined in the review queue")
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReopenParkedReopensOnlyWhatTheWindowParkedAndNobodyDecided(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seedParkedSenders(t, env)
	var offer ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT proposal_id FROM capture_pending_counterparty
			 WHERE email = 'offered@stranger.example'`).Scan(&offer)
	}); err != nil {
		t.Fatalf("seed: the offered sender has no review offer: %v", err)
	}

	recovery := compose.NewProviderRecovery(e.Pool)
	got, err := recovery.Reopen(context.Background(), outageWindow(), 0)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	// plain and offered: both exhausted inside the window and still unsure.
	if got.Counterparties != 2 {
		t.Fatalf("reopened %d sender questions, want 2 (plain, offered)", got.Counterparties)
	}
	for _, sender := range []string{"plain", "offered"} {
		if r := ledgerOf(t, e, sender+"@stranger.example"); r.status != "pending" || r.attempts != 0 {
			t.Errorf("%s = %+v, want pending with a fresh allowance", sender, r)
		}
	}
	for sender, want := range map[string]string{"old": "unsure", "decided": "rejected", "judged": "unsure"} {
		if r := ledgerOf(t, e, sender+"@stranger.example"); r.status != want {
			t.Errorf("%s = %q, want %q untouched", sender, r.status, want)
		}
	}
	if n := countRows(t, e, `SELECT count(*) FROM approval WHERE id = '`+offer.String()+`' AND status = 'pending'`); n != 0 {
		t.Error("a reopened question left its review offer pending")
	}
	if n := reopenAudits(t, e, "capture_pending_counterparty"); n != 2 {
		t.Errorf("%d audit rows for the reopenings, want 2", n)
	}

	again, err := recovery.Reopen(context.Background(), outageWindow(), 0)
	if err != nil {
		t.Fatalf("second Reopen: %v", err)
	}
	if again != (compose.ParkedWork{}) {
		t.Fatalf("a second run reopened %+v, want nothing", again)
	}
}

func TestReopenParkedDryRunCountsAndWritesNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seedParkedSenders(t, env)

	found, err := compose.NewProviderRecovery(e.Pool).Count(context.Background(), outageWindow())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if found.Counterparties != 2 {
		t.Fatalf("counted %d, want 2", found.Counterparties)
	}
	if r := ledgerOf(t, e, "plain@stranger.example"); r.status != "unsure" {
		t.Fatalf("a dry run moved the row to %q", r.status)
	}
	if n := reopenAudits(t, e, "capture_pending_counterparty"); n != 0 {
		t.Fatalf("a dry run wrote %d audit rows", n)
	}
}

func TestReopenParkedStopsAtTheBatch(t *testing.T) {
	env := newCaptureEnv(t)
	seedParkedSenders(t, env)

	recovery := compose.NewProviderRecovery(env.e.Pool)
	got, err := recovery.Reopen(context.Background(), outageWindow(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counterparties != 1 {
		t.Fatalf("a batch of 1 reopened %d", got.Counterparties)
	}
	rest, err := recovery.Count(context.Background(), outageWindow())
	if err != nil {
		t.Fatal(err)
	}
	if rest.Counterparties != 1 {
		t.Fatalf("%d left after a batch of 1, want 1", rest.Counterparties)
	}
}

func TestReopenParkedRefusesAWindowThatEndsBeforeItStarts(t *testing.T) {
	e := integration.SetupSearch(t)
	now := time.Now()
	_, err := compose.NewProviderRecovery(e.Pool).Reopen(context.Background(),
		capturemod.ReopenWindow{From: now, To: now.Add(-time.Hour)}, 0)
	if err == nil {
		t.Fatal("an inverted window was accepted")
	}
}

func TestReopenParkedReopensExhaustedEnrichmentInTheWindowOnly(t *testing.T) {
	e := integration.SetupSearch(t)
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	store := capturemod.NewAutoEnrichStore(e.DB())

	seedCompany := func(name string) ids.CompanyID {
		id := ids.NewV7()
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), `
				INSERT INTO company (id, owner_id, display_name, name_source, source, captured_by)
				VALUES ($1, $2, $3, 'domain', 'connector:gmail', 'connector:gmail')`, id, e.Rep1, name); err != nil {
				return err
			}
			_, err := tx.Exec(context.Background(), `
				INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
				VALUES ($1, $2, true, 'connector:gmail', 'connector:gmail')`, id, strings.ToLower(name)+".example")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.CompanyKind](id)
	}
	fail := func(c ids.CompanyID, attempts int) {
		for range attempts {
			if err := store.MarkQueued(wsCtx, c, time.Minute); err != nil {
				t.Fatal(err)
			}
			if err := store.MarkResolved(wsCtx, c, "failed"); err != nil {
				t.Fatal(err)
			}
		}
	}
	plain, old, archived, spared := seedCompany("Plain"), seedCompany("Old"), seedCompany("Archived"), seedCompany("Spared")
	fail(plain, 2)
	fail(old, 2)
	fail(archived, 2)
	fail(spared, 1)
	if err := store.ExpireExhausted(wsCtx); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			UPDATE capture_auto_enrich_state SET last_attempt_at = last_attempt_at - interval '10 days'
			 WHERE company_id = $1`, old); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `UPDATE company SET archived_at = now() WHERE id = $1`, archived)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	recovery := compose.NewProviderRecovery(e.Pool)
	got, err := recovery.Reopen(context.Background(), outageWindow(), 0)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if got.Enrichments != 1 {
		t.Fatalf("reopened %d enrichments, want 1 (plain)", got.Enrichments)
	}
	attemptsOf := func(c ids.CompanyID) int {
		return countRows(t, e, `SELECT attempts FROM capture_auto_enrich_state WHERE company_id = '`+c.String()+`'`)
	}
	if attemptsOf(plain) != 0 || attemptsOf(old) != 2 || attemptsOf(archived) != 2 || attemptsOf(spared) != 1 {
		t.Fatalf("attempts after = plain %d old %d archived %d spared %d", attemptsOf(plain), attemptsOf(old), attemptsOf(archived), attemptsOf(spared))
	}
	due, err := store.ListDueCompanies(wsCtx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].CompanyID != plain {
		t.Fatalf("due after the reopen = %v, want only the reopened company", due)
	}
	if n := reopenAudits(t, e, "capture_auto_enrich_state"); n != 1 {
		t.Fatalf("%d audit rows, want 1", n)
	}
	again, err := recovery.Reopen(context.Background(), outageWindow(), 0)
	if err != nil || again != (compose.ParkedWork{}) {
		t.Fatalf("second run = %+v, %v; want nothing", again, err)
	}
}
