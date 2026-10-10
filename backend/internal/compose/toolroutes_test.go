// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
