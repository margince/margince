// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// GET /v1/deals/slipping is whats_slipping_this_week served over REST. These
// tests drive the real HTTP stack with a real passport and a live volume meter.
// Both doors give the same deals for the same input, each call is admitted
// once, and the home screen keeps its shorter quiet window.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type slippingApp struct {
	e        *apptest.AppEnv
	meter    *agentvolume.Meter
	bearer   map[string]string
	passport ids.UUID
	mcp      *apptest.MCPClient
	deals    map[string]string // name → id
}

// newSlippingApp seeds three open deals through the deals API: quiet ninety
// days, quiet three weeks, and touched yesterday.
func newSlippingApp(t *testing.T, slug string) *slippingApp {
	t.Helper()
	meter := agentvolume.New(redistest.Client(t), agentvolume.Limits{}, agentvolume.DefaultWindow)
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"), compose.WithAgentVolume(meter),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Slipping Doors", slug+"@fable.test", "Admin")
	bearer, passport := passportWithID(t, e, "slipping agent", "read")
	app := &slippingApp{
		e: e, meter: meter, bearer: bearer, passport: passport, deals: map[string]string{},
		mcp: apptest.NewMCPClient(e, strings.TrimPrefix(bearer["Authorization"], "Bearer ")),
	}
	stages := apptest.DiscoverSeededPipeline(t, e)
	for name, idleDays := range map[string]int{"Quiet since spring": 90, "Quiet three weeks": 21, "Touched yesterday": 1} {
		var deal struct {
			ID string `json:"id"`
		}
		if status := e.Call(t, "POST", "/v1/deals", AnyMap{
			"name": name, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
		}, nil, &deal); status != http.StatusCreated {
			t.Fatalf("seeding %q → %d", name, status)
		}
		// No writer takes a past activity time; the idle rule reads these two columns.
		if _, err := e.Owner.Exec(t.Context(), `UPDATE deal SET created_at = now() - make_interval(days => $2),
			last_activity_at = now() - make_interval(days => $2) WHERE id = $1`, deal.ID, idleDays); err != nil {
			t.Fatalf("backdating %q: %v", name, err)
		}
		app.deals[name] = deal.ID
	}
	return app
}

// rest answers the route's body as the given caller, nil headers being the
// admin's own session.
func (a *slippingApp) rest(t *testing.T, query string, headers map[string]string) json.RawMessage {
	t.Helper()
	var body json.RawMessage
	if status := a.e.Call(t, "GET", "/v1/deals/slipping"+query, nil, headers, &body); status != http.StatusOK {
		t.Fatalf("GET /v1/deals/slipping%s → %d %s", query, status, body)
	}
	return body
}

func (a *slippingApp) tool(t *testing.T, args map[string]any) json.RawMessage {
	t.Helper()
	return a.mcp.CallOK(t, "whats_slipping_this_week", args).Envelope(t).Data
}

// charged runs one call and answers what it moved on the passport's counters.
func (a *slippingApp) charged(t *testing.T, call func()) map[agentvolume.Counter]int {
	t.Helper()
	ctx := asPassport(t, a.e, a.passport)
	counters := []agentvolume.Counter{agentvolume.Calls, agentvolume.Reads}
	was := map[agentvolume.Counter]int{}
	for _, c := range counters {
		was[c] = a.meter.Read(ctx, c).Observed
	}
	call()
	moved := map[agentvolume.Counter]int{}
	for _, c := range counters {
		moved[c] = a.meter.Read(ctx, c).Observed - was[c]
	}
	return moved
}

func slippingNames(t *testing.T, body json.RawMessage) []string {
	t.Helper()
	var answer struct {
		Deals []struct {
			Name string `json:"name"`
		} `json:"deals"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		t.Fatalf("the slipping answer does not decode: %v\n%s", err, body)
	}
	names := make([]string, 0, len(answer.Deals))
	for _, d := range answer.Deals {
		names = append(names, d.Name)
	}
	return names
}

func compacted(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	return out.String()
}

// The same input gets the same answer on both doors, at the default window and
// at a shorter one the caller names.
func TestTheSlippingRouteAndToolAnswerAlike(t *testing.T) {
	a := newSlippingApp(t, "slipping-doors")
	for _, tc := range []struct {
		query string
		args  map[string]any
		want  []string
	}{
		{"", map[string]any{}, []string{"Quiet since spring"}},
		{"?quiet_days=19", map[string]any{"quiet_days": 19}, []string{"Quiet since spring", "Quiet three weeks"}},
	} {
		rest, tool := a.rest(t, tc.query, a.bearer), a.tool(t, tc.args)
		if compacted(t, rest) != compacted(t, tool) {
			t.Errorf("GET /v1/deals/slipping%s answered\n%s\nthe tool answered\n%s", tc.query, rest, tool)
		}
		if got := slippingNames(t, rest); !slices.Equal(got, tc.want) {
			t.Errorf("GET /v1/deals/slipping%s = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// One REST call is one admitted call. It spends what the same call over MCP
// spends, and the call ceiling moves by one.
func TestASlippingRouteCallIsChargedOnce(t *testing.T) {
	a := newSlippingApp(t, "slipping-charge")
	overREST := a.charged(t, func() { a.rest(t, "?quiet_days=19", a.bearer) })
	overMCP := a.charged(t, func() { a.tool(t, map[string]any{"quiet_days": 19}) })

	if overREST[agentvolume.Calls] != 1 {
		t.Errorf("one REST call moved the call ceiling by %d, want 1", overREST[agentvolume.Calls])
	}
	if overREST[agentvolume.Reads] == 0 || overREST[agentvolume.Reads] != overMCP[agentvolume.Reads] {
		t.Errorf("REST charged %d reads, MCP %d, for the same answer", overREST[agentvolume.Reads], overMCP[agentvolume.Reads])
	}
}

// A human reads the route with their session. The home screen keeps its
// shorter window, so it lists the deal quiet three weeks that the route's
// default leaves out.
func TestAHumanReadsSlippingDealsAndTheAttentionLaneKeepsItsWindow(t *testing.T) {
	a := newSlippingApp(t, "slipping-human")
	if got := slippingNames(t, a.rest(t, "", nil)); !slices.Equal(got, []string{"Quiet since spring"}) {
		t.Errorf("a human's GET /v1/deals/slipping = %v, want the deal quiet since spring", got)
	}

	var feed struct {
		AtRisk []struct {
			ID string `json:"id"`
		} `json:"at_risk"`
	}
	if status := a.e.Call(t, "GET", "/v1/attention", nil, nil, &feed); status != http.StatusOK {
		t.Fatalf("GET /v1/attention → %d", status)
	}
	atRisk := make([]string, 0, len(feed.AtRisk))
	for _, item := range feed.AtRisk {
		atRisk = append(atRisk, item.ID)
	}
	if !slices.Contains(atRisk, a.deals["Quiet three weeks"]) || slices.Contains(atRisk, a.deals["Touched yesterday"]) {
		t.Errorf("the attention lane lists %v; want the deal quiet three weeks (%s) and not the one touched yesterday",
			atRisk, a.deals["Quiet three weeks"])
	}
}
