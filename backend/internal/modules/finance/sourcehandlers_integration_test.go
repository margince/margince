// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

// Managing the accounting source over HTTP against a real database. It covers
// who may connect, what a refused connect leaves behind, and when a sync is
// queued.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	unreadableProvider = "broken_ledger"
	otherProvider      = "other_ledger"
)

// offeredSources stands at the Sources seam: three providers on offer, one of
// which answers its connect check with a failure.
type offeredSources struct{}

func (offeredSources) Providers() []SourceOption {
	return []SourceOption{
		{Provider: unreadableProvider, Description: "A source that cannot be read."},
		{Provider: otherProvider, Description: "A second ledger."},
		{Provider: fixtureProviderName, Description: "The suite's own ledger."},
	}
}

func (offeredSources) Open(provider string, _ SourceConnection) (Provider, bool) {
	switch provider {
	case unreadableProvider:
		return unreadableSource{}, true
	case otherProvider, fixtureProviderName:
		return fixtureSource{}, true
	}
	return nil, false
}

type unreadableSource struct{ fixtureSource }

func (unreadableSource) Check(context.Context) error {
	return errors.New("the source refused the token")
}

// recordingQueue counts the syncs a handler queued, and refuses when told to.
type recordingQueue struct {
	queued int
	refuse error
}

func (q *recordingQueue) QueueSync(context.Context) error {
	q.queued++
	return q.refuse
}

// directorySource lists a customer directory, and runs onLedger before each
// ledger it answers so a test can change the database mid-pass.
type directorySource struct {
	fixtureSource
	customers []SourceCustomer
	onLedger  func()
}

func (d directorySource) Customers(context.Context) ([]SourceCustomer, error) {
	return d.customers, nil
}

func (d directorySource) InvoicesFor(ctx context.Context, customer string) (SourceLedger, error) {
	if d.onLedger != nil {
		d.onLedger()
	}
	return d.fixtureSource.InvoicesFor(ctx, customer)
}

// sourceAdmin is the finance fixture with this test's connection as the only
// live one, and the handlers wired to offeredSources and a recordingQueue.
type sourceAdmin struct {
	*financeEnv
	handlers Handlers
	queue    *recordingQueue
}

func setupSourceAdmin(t *testing.T) *sourceAdmin {
	t.Helper()
	e := setupFinance(t)
	// The package's tests share one database, and readConnection takes the
	// newest live connection. Retiring the earlier tests' connections keeps a
	// disconnect here from surfacing one of theirs.
	e.ownerExec(t, `UPDATE finance_connection SET archived_at = now()
		 WHERE id <> $1 AND archived_at IS NULL`, e.connection)
	queue := &recordingQueue{}
	return &sourceAdmin{
		financeEnv: e,
		handlers:   Handlers{store: e.store}.WithSources(offeredSources{}, queue),
		queue:      queue,
	}
}

// as answers the fixture's workspace acting as a human holding grants.
func (a *sourceAdmin) as(scope principal.RowScope, grants map[string]principal.ObjectGrant) context.Context {
	return a.asUser(ids.NewV7(), scope, grants)
}

func (a *sourceAdmin) asUser(
	user ids.UUID, scope principal.RowScope, grants map[string]principal.ObjectGrant,
) context.Context {
	return principal.WithActor(a.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RoleKeys: []string{"member"}, Objects: grants, RowScope: scope},
	})
}

var everyAction = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}

func (a *sourceAdmin) admin() context.Context {
	return a.as(principal.RowScopeAll, map[string]principal.ObjectGrant{
		objectIntegrations: everyAction, "company": everyAction,
	})
}

func call(ctx context.Context, method, body string, serve func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/finance/source", strings.NewReader(body)).WithContext(ctx)
	serve(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, status, rec.Body.String())
	}
	if code == "" {
		return
	}
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decoding the problem: %v", err)
	}
	if problem.Code != code {
		t.Fatalf("problem code = %q, want %q (body: %s)", problem.Code, code, rec.Body.String())
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestTheSourceCardShowsTheLiveConnectionAndWhatIsOnOffer(t *testing.T) {
	a := setupSourceAdmin(t)

	rec := call(a.admin(), http.MethodGet, "", a.handlers.GetFinanceSource)
	expect(t, rec, http.StatusOK, "")
	state := decode[crmcontracts.FinanceSourceState](t, rec)
	if state.Connection == nil || state.Connection.Provider != fixtureProviderName {
		t.Fatalf("connection = %+v, want the fixture's live %s", state.Connection, fixtureProviderName)
	}
	if len(state.Available) != 3 || state.Available[2].Description != "The suite's own ledger." {
		t.Errorf("available = %+v, want every offered source with its description", state.Available)
	}

	bare := Handlers{store: NewStore(a.store.db, a.store.rates)}
	rec = call(a.admin(), http.MethodGet, "", bare.GetFinanceSource)
	if got := decode[crmcontracts.FinanceSourceState](t, rec); len(got.Available) != 0 || got.Available == nil {
		t.Errorf("an unwired build offers %+v, want an empty list rather than null", got.Available)
	}

	reader := a.as(principal.RowScopeAll, map[string]principal.ObjectGrant{"finance": {Read: true}})
	expect(t, call(reader, http.MethodGet, "", a.handlers.GetFinanceSource), http.StatusForbidden, "permission_denied")
}

func TestABuildWithoutSourcesAnswersNotImplemented(t *testing.T) {
	a := setupSourceAdmin(t)
	bare := Handlers{store: a.store}
	expect(t, call(a.admin(), http.MethodPut, `{"provider":"test_ledger"}`, bare.ConnectFinanceSource),
		http.StatusNotImplemented, "")
	expect(t, call(a.admin(), http.MethodPost, "", bare.SyncFinanceSource), http.StatusNotImplemented, "")
}

// A source that cannot be read is refused before any row exists, so the sweep
// never meets a connection it fails on every six hours.
func TestConnectingAnUnreadableSourceLeavesNothingBehind(t *testing.T) {
	a := setupSourceAdmin(t)
	ctx := a.admin()
	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")

	rec := call(ctx, http.MethodPut, `{"provider":"broken_ledger"}`, a.handlers.ConnectFinanceSource)
	expect(t, rec, http.StatusBadGateway, "provider_unusable")
	if strings.Contains(rec.Body.String(), "token") {
		t.Errorf("the refusal repeats what the source said: %s", rec.Body.String())
	}
	// A source this build does not compose is a fault of the body's provider
	// field, answered like every other body fault.
	for _, body := range []string{`{"provider":"absent_ledger"}`, `{"provider":""}`} {
		refused := call(ctx, http.MethodPut, body, a.handlers.ConnectFinanceSource)
		expect(t, refused, http.StatusUnprocessableEntity, "validation_error")
		if !strings.Contains(refused.Body.String(), `"field":"provider"`) {
			t.Errorf("%s: the refusal does not name provider: %s", body, refused.Body.String())
		}
	}

	state := decode[crmcontracts.FinanceSourceState](t, call(ctx, http.MethodGet, "", a.handlers.GetFinanceSource))
	if state.Connection != nil {
		t.Errorf("a refused connect left %+v connected", state.Connection)
	}
	if a.queue.queued != 0 {
		t.Errorf("%d syncs queued for a source that never connected", a.queue.queued)
	}
}

func TestConnectingRecordsOneSourceAndQueuesItsFirstSync(t *testing.T) {
	a := setupSourceAdmin(t)
	ctx := a.admin()
	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")
	mark := auditWatermark(a.ctx, t, a.financeEnv)

	rec := call(ctx, http.MethodPut, `{"provider":"other_ledger"}`, a.handlers.ConnectFinanceSource)
	expect(t, rec, http.StatusOK, "")
	first := decode[crmcontracts.FinanceSourceState](t, rec).Connection
	if first == nil || first.Provider != otherProvider || first.Status != statusConnectionConnecting {
		t.Fatalf("connection = %+v, want %s connecting", first, otherProvider)
	}
	if a.queue.queued != 1 {
		t.Errorf("%d syncs queued, want the first one", a.queue.queued)
	}
	if got := auditRowsByEntity(a.ctx, t, a.financeEnv, mark)[entityConnection]; got != 1 {
		t.Errorf("%d connection audit rows, want the create", got)
	}

	// Connecting the source already connected answers it unchanged.
	again := decode[crmcontracts.FinanceSourceState](t,
		call(ctx, http.MethodPut, `{"provider":"other_ledger"}`, a.handlers.ConnectFinanceSource)).Connection
	if again == nil || again.Id != first.Id {
		t.Errorf("a repeated connect answered %+v, want connection %s unchanged", again, first.Id)
	}

	expect(t, call(ctx, http.MethodPut, `{"provider":"test_ledger"}`, a.handlers.ConnectFinanceSource),
		http.StatusConflict, "conflict")
}

func TestOnlyAHumanWithTheIntegrationsGrantConnects(t *testing.T) {
	a := setupSourceAdmin(t)
	body := `{"provider":"test_ledger"}`

	reader := a.as(principal.RowScopeAll, map[string]principal.ObjectGrant{objectIntegrations: {Read: true}})
	expect(t, call(reader, http.MethodPut, body, a.handlers.ConnectFinanceSource), http.StatusForbidden, "permission_denied")
	expect(t, call(reader, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusForbidden, "permission_denied")
	expect(t, call(reader, http.MethodPost, "", a.handlers.SyncFinanceSource), http.StatusForbidden, "permission_denied")

	agent := principal.WithActor(a.ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{objectIntegrations: everyAction},
			RowScope: principal.RowScopeAll,
		},
	})
	expect(t, call(agent, http.MethodPut, body, a.handlers.ConnectFinanceSource), http.StatusForbidden, "permission_denied")
	expect(t, call(agent, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusForbidden, "permission_denied")
	expect(t, call(a.admin(), http.MethodPut, `{"provider":"test_ledger","extra":1}`, a.handlers.ConnectFinanceSource),
		http.StatusUnprocessableEntity, "")
}

// Disconnecting keeps the mirrored rows and stops the sync; a second disconnect
// finds nothing to do and says so with the same answer.
func TestDisconnectingStopsTheSyncAndAuditsTheTransition(t *testing.T) {
	a := setupSourceAdmin(t)
	ctx := a.admin()
	mark := auditWatermark(a.ctx, t, a.financeEnv)

	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")
	if status := a.connectionStatus(t, a.connection); status != statusDisconnected {
		t.Errorf("connection status = %q, want %q", status, statusDisconnected)
	}
	if got := auditRowsByEntity(a.ctx, t, a.financeEnv, mark)[entityConnection]; got != 1 {
		t.Errorf("%d connection audit rows, want the one transition", got)
	}
	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")
	if got := auditRowsByEntity(a.ctx, t, a.financeEnv, mark)[entityConnection]; got != 1 {
		t.Errorf("%d connection audit rows after a disconnect with nothing connected, want still 1", got)
	}
}

func TestASyncIsQueuedOnlyForAConnectedSource(t *testing.T) {
	a := setupSourceAdmin(t)
	ctx := a.admin()

	rec := call(ctx, http.MethodPost, "", a.handlers.SyncFinanceSource)
	expect(t, rec, http.StatusAccepted, "")
	if got := decode[crmcontracts.RefreshAccepted](t, rec); got.Status != crmcontracts.RefreshAcceptedStatusEnqueued {
		t.Errorf("accepted status = %q, want enqueued", got.Status)
	}
	if a.queue.queued != 1 {
		t.Fatalf("%d syncs queued, want 1", a.queue.queued)
	}

	a.queue.refuse = errors.New("the job queue is unreachable")
	expect(t, call(ctx, http.MethodPost, "", a.handlers.SyncFinanceSource), http.StatusInternalServerError, "")

	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")
	a.queue.refuse = nil
	expect(t, call(ctx, http.MethodPost, "", a.handlers.SyncFinanceSource), http.StatusNotFound, "not_found")
	if a.queue.queued != 2 {
		t.Errorf("%d syncs queued in all, want 2: none once the source is gone", a.queue.queued)
	}
}
