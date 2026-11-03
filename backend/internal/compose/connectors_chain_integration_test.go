// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The import a connect asks for, over real migrated Postgres and the real queue
// insert. The consent's callback runs under the grantor principal the signed
// state rebuilds. These tests prove that principal can open a run, audit it and
// schedule its pager, which no fake engine can.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// chainWire is the connect flow over setupBackfillWire's registry and queue.
// That harness has already connected Rep1's gmail, so each callback here is a
// re-grant of a mailbox with no import yet.
type chainWire struct {
	*backfillWireEnv
	h      connectorHandlers
	google *recordingOAuth
}

// setupChainWire registers the real calendar connector over the same Google
// token stub as the mailbox, so a shared consent seals both from one exchange.
func setupChainWire(t *testing.T) chainWire {
	t.Helper()
	b := setupBackfillWire(t)
	google := &recordingOAuth{granted: gmailScopes}
	b.registry.Register(gcal.New(google, stubGcalAPI{}))
	return chainWire{backfillWireEnv: b, google: google, h: connectorHandlers{
		registry:      b.registry,
		authority:     backfillAuthority{},
		oauth:         google,
		gmailAPI:      stubGmailAPI{},
		gcalOAuth:     google,
		gcalAPI:       stubGcalAPI{},
		signer:        newStateSigner([]byte(testStateKey)),
		publicBaseURL: "https://app.test",
		apiBaseURL:    "https://api.test",
		importer:      b.handlers.importOnConnect(),
	}}
}

// connect starts a consent as Rep1 and returns its signed state, with the nonce
// cookie it set kept in the browser's jar.
func (c chainWire) connect(t *testing.T, jar browserJar, body string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gmail/connect", strings.NewReader(body)).WithContext(c.human)
	req.Header.Set("Content-Type", "application/json")
	c.h.ConnectConnector(rec, req, crmcontracts.CaptureProviderGmail)
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK || resp.AuthorizeUrl == nil {
		t.Fatalf("connect = %d %s (%v), want 200 with an authorize_url", rec.Code, rec.Body, err)
	}
	jar.keep(rec.Result())
	_, u := consentState(t, c.h, *resp.AuthorizeUrl)
	return u.Query().Get("state")
}

// callback returns from the provider the way the browser does: session-less,
// carrying only the jar, under the correlation id the HTTP layer mints.
func (c chainWire) callback(t *testing.T, jar browserJar, provider, state string, params crmcontracts.ConnectorOAuthCallbackParams) *httptest.ResponseRecorder {
	t.Helper()
	params.State = state
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/"+provider+"/callback", nil).WithContext(ctx)
	jar.attach(req)
	rec := httptest.NewRecorder()
	c.h.ConnectorOAuthCallback(rec, req, crmcontracts.CaptureProvider(provider), params)
	if rec.Code != http.StatusFound {
		t.Fatalf("%s callback = %d %s, want 302", provider, rec.Code, rec.Body)
	}
	jar.keep(rec.Result())
	return rec
}

func granted() crmcontracts.ConnectorOAuthCallbackParams {
	code := "the-code"
	return crmcontracts.ConnectorOAuthCallbackParams{Code: &code}
}

func (c chainWire) gmailConnection(t *testing.T) string {
	t.Helper()
	return c.env.WsScalar(t, `SELECT id::text FROM capture_connection WHERE user_id = $1 AND provider = 'gmail'`, c.env.Rep1)
}

// importOpened counts what one started import writes: its queued run, the
// connection's audit row naming the reach, and the pager behind the run.
func (c chainWire) importOpened(t *testing.T, connID string, estimate int) (runs, audits, pagers int) {
	t.Helper()
	runs = c.env.WsCount(t, `SELECT count(*) FROM capture_backfill
		WHERE connection_id = $1 AND status = 'queued' AND window_months = 6 AND total_estimate = $2`, connID, estimate)
	audits = c.env.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_connection' AND entity_id = $1 AND action = 'update'
		  AND after->>'backfill_window_months' = '6' AND actor_type = 'human' AND actor_id = $2`,
		connID, "human:"+c.env.Rep1.String())
	return runs, audits, pagersIn(t, c.env.Pool, c.env.WS)
}

// pagersIn counts every backfill pager queued for one workspace, so a run that
// should not exist cannot hide behind a run id nobody holds.
func pagersIn(t *testing.T, pool *pgxpool.Pool, ws ids.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM river_job WHERE kind = $1 AND args->>'workspace_id' = $2`,
		CaptureBackfillArgs{}.Kind(), ws.String()).Scan(&n); err != nil {
		t.Fatalf("counting pagers: %v", err)
	}
	return n
}

func requireLocation(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestAConnectImportUnderTheCapStartsInTheCallback(t *testing.T) {
	c := setupChainWire(t)
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"settings","import_window":"6m"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/settings/connections/ok/gmail")

	connID := c.gmailConnection(t)
	runs, audits, pagers := c.importOpened(t, connID, c.gmail.messages)
	if runs != 1 || audits != 1 || pagers != 1 {
		t.Fatalf("queued runs, audit rows, pagers = %d, %d, %d; want one of each", runs, audits, pagers)
	}
	runID := ids.MustParse(c.env.WsScalar(t, `SELECT id::text FROM capture_backfill WHERE connection_id = $1`, connID))
	if n := pagersFor(t, c.env.Pool, runID); n != 1 {
		t.Errorf("%d pager(s) name the run, want 1: a queued run with no pager blocks its mailbox", n)
	}
}

// Above the cap nothing is spent before the reader sees the count: no run, no
// audit, no job. The landing names the held window so its row can offer it.
func TestAConnectImportOverTheCapLandsHeldWithItsEstimate(t *testing.T) {
	c := setupChainWire(t)
	c.gmail.messages = autoImportCap + 1
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"settings","import_window":"6m"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/settings/connections/ok/gmail?import=6m")

	connID := c.gmailConnection(t)
	if runs := c.env.WsCount(t, `SELECT count(*) FROM capture_backfill WHERE connection_id = $1`, connID); runs != 0 {
		t.Errorf("%d run(s) opened above the cap, want none", runs)
	}
	if _, audits, pagers := c.importOpened(t, connID, c.gmail.messages); audits != 0 || pagers != 0 {
		t.Errorf("audit rows, pagers = %d, %d above the cap; want none", audits, pagers)
	}
	if n := c.env.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'connected'`, connID); n != 1 {
		t.Errorf("the mailbox is not connected after a held import")
	}
	var preview crmcontracts.BackfillPreview
	if code, _ := c.do(c.human, t, c.previewBackfill(crmcontracts.CaptureProviderGmail), `{"window":"6m"}`, &preview); code != http.StatusOK {
		t.Fatalf("preview of the held window = %d, want 200", code)
	}
	if preview.EstimatedMessages != autoImportCap+1 {
		t.Errorf("the held window previews %d messages, want the %d the cap held back", preview.EstimatedMessages, autoImportCap+1)
	}
}

// Reconnecting a mailbox mid-import asks for a run that is already live. The
// grant is stored either way, so the landing is a plain success with the one run
// and its one pager untouched.
func TestAConnectImportThatCannotStartStillLandsConnected(t *testing.T) {
	c := setupChainWire(t)
	if code, _ := c.do(c.human, t, c.startBackfill(crmcontracts.CaptureProviderGmail), `{"window":"6m"}`, nil); code != http.StatusAccepted {
		t.Fatalf("opening the live run = %d, want 202", code)
	}
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"settings","import_window":"6m"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/settings/connections/ok/gmail")

	connID := c.gmailConnection(t)
	if runs, _, pagers := c.importOpened(t, connID, c.gmail.messages); runs != 1 || pagers != 1 {
		t.Errorf("queued runs, pagers = %d, %d; want the one live run and its pager", runs, pagers)
	}
	if n := c.env.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'connected'`, connID); n != 1 {
		t.Errorf("the re-granted mailbox is not connected")
	}
}

// One screen asks for mail, its calendar and an import. Unticking the calendar
// there costs only the calendar: the mailbox stays connected and its import
// queued, and no second consent opens.
func TestAnUntickedCalendarOnTheSharedScreenKeepsTheMailboxAndItsImport(t *testing.T) {
	c := setupChainWire(t)
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"settings","also_connect":"gcal","import_window":"6m"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/settings/connections/denied/gcal")

	connID := c.gmailConnection(t)
	if n := c.env.WsCount(t, `SELECT count(*) FROM capture_connection WHERE id = $1 AND status = 'connected'`, connID); n != 1 {
		t.Errorf("the granted mailbox is not connected after its calendar was unticked")
	}
	if runs, audits, pagers := c.importOpened(t, connID, c.gmail.messages); runs != 1 || audits != 1 || pagers != 1 {
		t.Errorf("queued runs, audit rows, pagers = %d, %d, %d; want the mail leg's one import", runs, audits, pagers)
	}
	if n := c.env.WsCount(t, `SELECT count(*) FROM capture_connection WHERE user_id = $1 AND provider = 'gcal'`, c.env.Rep1); n != 0 {
		t.Errorf("%d calendar connection(s) from a grant without the calendar scope, want none", n)
	}
}

// With both ticked, the one callback connects the calendar from the mail leg's
// grant: one exchange, and the calendar row records what Google granted.
func TestOneSharedConsentConnectsTheMailboxAndItsCalendar(t *testing.T) {
	c := setupChainWire(t)
	c.google.granted = append(slices.Clone(gmailScopes), gcal.ConsentScopes()...)
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"onboarding","also_connect":"gcal"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/onboarding/connect/ok/gcal")

	if c.google.exchanges != 1 {
		t.Errorf("the code was exchanged %d times, want once for both connections", c.google.exchanges)
	}
	if n := c.env.WsCount(t, `SELECT count(*) FROM capture_connection
		WHERE user_id = $1 AND provider = 'gcal' AND status = 'connected'
		  AND account_label = 'owner@example.com' AND $2 = ANY(provider_scopes)`, c.env.Rep1, gcal.ReadScope); n != 1 {
		t.Errorf("connected calendar rows holding the calendar grant = %d, want 1", n)
	}
}
