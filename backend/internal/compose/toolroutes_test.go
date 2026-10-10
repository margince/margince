// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// volumeLookups counts how often admission consults each volume counter. Each
// admission of a call reads the meter, so the count tells how many times a
// call was admitted.
type volumeLookups struct{ reads map[agentvolume.Counter]int }

func (v *volumeLookups) Read(_ context.Context, c agentvolume.Counter) agentvolume.Reading {
	v.reads[c]++
	return agentvolume.Reading{Counter: c}
}

func (v *volumeLookups) total() int {
	n := 0
	for _, count := range v.reads {
		n += count
	}
	return n
}

// slippingDoors is the slipping tool on a registry, with the contract router in
// front of it as contractAPI wires them. Every admission and charge is counted.
type slippingDoors struct {
	registry *agents.Registry
	router   http.Handler
	lookups  *volumeLookups
	charges  *countingCharges
	asked    []int
}

func newSlippingDoors(t *testing.T) *slippingDoors {
	t.Helper()
	d := &slippingDoors{
		lookups: &volumeLookups{reads: map[agentvolume.Counter]int{}},
		charges: &countingCharges{spent: map[agentvolume.Counter]int{}},
	}
	gate := auth.NewGate(fullSeat{}, auth.WithVolumeMeter(d.lookups))
	d.registry = agents.NewRegistry(nil, gate, agents.WithVolumeCharger(d.charges))
	idle := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)
	deal := ids.NewV7()
	agents.RegisterSlippingTools(d.registry, func(_ context.Context, quietDays int) ([]agents.SlippingDeal, error) {
		d.asked = append(d.asked, quietDays)
		return []agents.SlippingDeal{{
			DealID: deal, Name: "Quiet renewal", Stalled: true, LastActivityAt: &idle, CreatedAt: idle,
		}}, nil
	}, nil, deals.StalledThresholdDays)
	d.router = crmcontracts.HandlerWithOptions(Server{toolRegistry: d.registry}, crmcontracts.ChiServerOptions{
		BaseURL:     httpserver.BaseURL,
		Middlewares: []crmcontracts.MiddlewareFunc{agentGate(d.registry, nil, nil, nil, nil, nil, nil, gate)},
	})
	return d
}

func (d *slippingDoors) get(ctx context.Context, t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	d.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx))
	return recorder
}

func (d *slippingDoors) reset() {
	d.lookups.reads = map[agentvolume.Counter]int{}
	d.charges.spent = map[agentvolume.Counter]int{}
}

func readingAgent(ctx context.Context) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:slipping", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
}

// A route served by the registry is admitted by the registry alone. Over REST a
// passport meets the admission and charges of the same call over MCP, and gets
// the same payload.
func TestARegistryServedRouteIsAdmittedOnceAndAnswersAsTheTool(t *testing.T) {
	d := newSlippingDoors(t)
	ctx := readingAgent(t.Context())

	sealed, err := d.registry.Invoke(ctx, "whats_slipping_this_week", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("the MCP door's call: %v", err)
	}
	direct, _, err := unwrapToolEnvelope(sealed)
	if err != nil {
		t.Fatal(err)
	}
	lookups, charges := d.lookups.total(), d.charges.spent
	d.reset()

	recorder := d.get(ctx, t, "/v1/deals/slipping")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/deals/slipping as a passport → %d %s", recorder.Code, recorder.Body)
	}
	if !bytes.Equal(recorder.Body.Bytes(), direct) {
		t.Errorf("REST answered %s, the tool answered %s", recorder.Body, direct)
	}
	if got := d.lookups.total(); got != lookups {
		t.Errorf("the REST call consulted the volume meter %d times, the tool call %d — "+
			"the route was admitted more than once", got, lookups)
	}
	if !equalCounts(d.charges.spent, charges) {
		t.Errorf("the REST call charged %v, the tool call %v", d.charges.spent, charges)
	}
	if recorder.Header().Get(toolTraceHeader) == "" {
		t.Errorf("no %s header, so the call cannot be found in the audit log", toolTraceHeader)
	}
}

// The control for the test above. A read the gate admits consults the meter in
// the gate, so a second admission of the slipping route would show.
func TestTheGateAdmitsAReadItDoesNotLeaveToTheRegistry(t *testing.T) {
	d := newSlippingDoors(t)
	router := chi.NewRouter()
	router.With(agentGate(d.registry, nil, nil, nil, nil, nil, nil,
		auth.NewGate(fullSeat{}, auth.WithVolumeMeter(d.lookups)))).
		Get("/v1/deals", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/deals", nil).WithContext(readingAgent(t.Context())))

	if recorder.Code != http.StatusOK || d.lookups.reads[agentvolume.Reads] != 1 {
		t.Fatalf("GET /v1/deals → %d after %d read-bound lookups, want 200 after 1",
			recorder.Code, d.lookups.reads[agentvolume.Reads])
	}
}

// The query parameters are the tool's arguments: one named passes through, one
// omitted leaves the tool its own default.
func TestTheSlippingRouteAsksTheToolAtTheWindowItNames(t *testing.T) {
	d := newSlippingDoors(t)
	ctx := readingAgent(t.Context())
	for _, target := range []string{"/v1/deals/slipping", "/v1/deals/slipping?quiet_days=19"} {
		if recorder := d.get(ctx, t, target); recorder.Code != http.StatusOK {
			t.Fatalf("GET %s → %d %s", target, recorder.Code, recorder.Body)
		}
	}
	if len(d.asked) != 2 || d.asked[0] != deals.StalledThresholdDays || d.asked[1] != 19 {
		t.Errorf("the lister was asked at %v, want [%d 19]", d.asked, deals.StalledThresholdDays)
	}

	recorder := d.get(ctx, t, "/v1/deals/slipping?quiet_days=400")
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("quiet_days=400 → %d, want 422 for a window past the tool's bound", recorder.Code)
	}
}

// A human calls the same route with a session. The registry admits a human as
// any other route does, on their own RBAC at the store.
func TestAHumanReadsTheSlippingRoute(t *testing.T) {
	d := newSlippingDoors(t)
	ctx := principal.WithActor(principal.WithWorkspaceID(t.Context(), ids.NewV7()),
		principal.Principal{Type: principal.PrincipalHuman, ID: "user:" + ids.NewV7().String()})

	recorder := d.get(ctx, t, "/v1/deals/slipping")

	var body struct {
		Deals []struct {
			Name string `json:"name"`
		} `json:"deals"`
	}
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &body) != nil {
		t.Fatalf("a human's GET /v1/deals/slipping → %d %s", recorder.Code, recorder.Body)
	}
	if len(body.Deals) != 1 || body.Deals[0].Name != "Quiet renewal" {
		t.Errorf("a human was answered %+v, want the one quiet deal", body.Deals)
	}
}

func equalCounts(a, b map[agentvolume.Counter]int) bool {
	if len(a) != len(b) {
		return false
	}
	for counter, n := range a {
		if b[counter] != n {
			return false
		}
	}
	return true
}

// commandCall runs serveToolCommand on one request and answers the arguments
// the tool was invoked with, nil when it was not invoked.
func commandCall(t *testing.T, body string, headers map[string]string) (map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	var invoked map[string]any
	invoke := func(_ context.Context, _ string, in json.RawMessage) (json.RawMessage, error) {
		if err := json.Unmarshal(in, &invoked); err != nil {
			t.Fatalf("the tool was handed arguments that are not an object: %s", in)
		}
		return json.RawMessage(`{"data":{}}`), nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/deals/x/progress", bytes.NewBufferString(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	serveToolCommand(recorder, req, invoke, "progress_deal", pathArgument{"deal_id", "from-the-path"})
	return invoked, recorder
}

// The REST spellings of the retry key and the approval reach the tool as its
// own arguments. The deal is the one the path names.
func TestACommandRouteHandsTheToolItsHeadersAndItsPath(t *testing.T) {
	invoked, _ := commandCall(t, `{"deal_id":"from-the-body","to_stage_id":"s"}`, map[string]string{
		idempotencyKeyHeader: "monday", approvalTokenHeader: "approval-1",
	})
	want := map[string]any{
		"deal_id": "from-the-path", "to_stage_id": "s", "idempotency_key": "monday", "approval_id": "approval-1",
	}
	if !reflect.DeepEqual(invoked, want) {
		t.Errorf("the tool was invoked with %v, want %v", invoked, want)
	}
	if empty, _ := commandCall(t, "", nil); !reflect.DeepEqual(empty, map[string]any{"deal_id": "from-the-path"}) {
		t.Errorf("an empty body invoked the tool with %v, want only the path's deal", empty)
	}
}

// A header and a body member that name two different keys are refused before
// the tool runs: the call cannot claim both.
func TestACommandRouteRefusesAHeaderTheBodyContradicts(t *testing.T) {
	invoked, recorder := commandCall(t, `{"idempotency_key":"tuesday"}`, map[string]string{idempotencyKeyHeader: "monday"})
	if recorder.Code != http.StatusUnprocessableEntity || invoked != nil {
		t.Errorf("a contradicting key answered %d and invoked %v, want 422 and no run", recorder.Code, invoked)
	}
	if invoked, _ := commandCall(t, `{"idempotency_key":"monday"}`, map[string]string{idempotencyKeyHeader: "monday"}); invoked == nil {
		t.Error("a header repeating the body's key was refused, want it run")
	}
}
