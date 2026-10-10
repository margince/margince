// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The three write tools a route runs through the registry: draft_follow_ups_for,
// progress_deal and qualify_lead. Each test sends one input to the route as a
// passport and to the tool over MCP. The effect, answer and refusals must agree.

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lend mints a second passport over the same workspace.
func (d *twoDoors) lend(t *testing.T, label string, scopes ...string) *twoDoors {
	t.Helper()
	bearer, _ := passportWithID(t, d.e, label, scopes...)
	return &twoDoors{e: d.e, bearer: bearer, mcp: apptest.NewMCPClient(d.e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))}
}

// with answers args plus one more member, leaving args as it was.
//
//craft:ignore naked-any a tool argument is whichever JSON value its schema declares
func with(args map[string]any, name string, value any) map[string]any {
	out := maps.Clone(args)
	out[name] = value
	return out
}

// openStages answers the seeded pipeline's open stages, in order.
func openStages(t *testing.T, e *apptest.AppEnv) []string {
	t.Helper()
	var pipelines struct {
		Data []struct {
			Stages []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK || len(pipelines.Data) == 0 {
		t.Fatalf("GET /v1/pipelines → %d", status)
	}
	var open []string
	for _, s := range pipelines.Data[0].Stages {
		if s.Semantic == "open" {
			open = append(open, s.ID)
		}
	}
	if len(open) < 2 {
		t.Fatalf("the seeded pipeline has %d open stages, want two to move between", len(open))
	}
	return open
}

func openDeal(t *testing.T, e *apptest.AppEnv, stages apptest.SeededStages, name string) string {
	t.Helper()
	return createdID(t, e, "/v1/deals", AnyMap{
		"name": name, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
	})
}

func dealStage(t *testing.T, e *apptest.AppEnv, deal string) string {
	t.Helper()
	var read struct {
		StageID string `json:"stage_id"`
	}
	if status := e.Call(t, "GET", "/v1/deals/"+deal, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /v1/deals/%s → %d", deal, status)
	}
	return read.StageID
}

// stagedProgressions answers the progress_deal approvals staged on one deal.
func stagedProgressions(t *testing.T, e *apptest.AppEnv, deal string) []string {
	t.Helper()
	rows, err := e.Owner.Query(t.Context(),
		`SELECT id::text FROM approval WHERE kind = 'progress_deal' AND target_entity_id = $1`, deal)
	if err != nil {
		t.Fatalf("reading the approvals staged on %s: %v", deal, err)
	}
	defer rows.Close()
	var staged []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		staged = append(staged, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return staged
}

type sharedProgress struct {
	Deal struct {
		RecordType string `json:"record_type"`
		Fields     struct {
			StageID string `json:"stage_id"`
		} `json:"fields"`
	} `json:"deal"`
}

// An open-to-open move runs on either door: the deal moves, the note is logged,
// and the audit and outbox rows of both name the agent.
func TestTheProgressRouteMovesADealAsTheToolDoes(t *testing.T) {
	d := newTwoDoors(t, "progress-doors", "read", "write")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	next := openStages(t, d.e)[1]
	viaREST := openDeal(t, d.e, stages, "Rest-moved renewal")
	viaTool := openDeal(t, d.e, stages, "Tool-moved renewal")
	args := map[string]any{"to_stage_id": next, "note": "Moved after the scoping call"}

	rest := d.rest(t, "POST", "/v1/deals/"+viaREST+"/progress", args)
	tool := d.tool(t, "progress_deal", with(args, "deal_id", viaTool))
	moved := sameRecords[sharedProgress](t, "progress", rest, tool)
	if moved.Deal.Fields.StageID != next {
		t.Errorf("the answer puts the deal at stage %s, want %s", moved.Deal.Fields.StageID, next)
	}
	for deal, answer := range map[string]json.RawMessage{viaREST: rest, viaTool: tool} {
		var noted struct {
			NoteActivityID string `json:"note_activity_id"`
		}
		if err := json.Unmarshal(answer, &noted); err != nil || noted.NoteActivityID == "" {
			t.Fatalf("the answer for %s names no note (%v): %s", deal, err, answer)
		}
		if stage := dealStage(t, d.e, deal); stage != next {
			t.Errorf("deal %s is at stage %s, want %s", deal, stage, next)
		}
		assertTheAgentWrote(t, d.e, deal, "advance_stage")
		assertTheAgentWrote(t, d.e, noted.NoteActivityID, "create")
	}
}

// A move to a won stage is confirm-first for an agent. Each door stages one
// approval and answers as the gate's own staging path does: 403
// approval_required naming the approval. Its retry under X-Approval-Token runs.
func TestAConfirmFirstProgressStagesOneApprovalOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "progress-staged", "read", "write")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	viaREST := openDeal(t, d.e, stages, "Rest-won renewal")
	viaTool := openDeal(t, d.e, stages, "Tool-won renewal")
	viaGate := openDeal(t, d.e, stages, "Gate-won renewal")
	win := AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}

	var refusal fileRefusal
	if status := d.e.Call(t, "POST", "/v1/deals/"+viaREST+"/progress", win, d.bearer, &refusal); status != http.StatusForbidden || refusal.Code != "approval_required" {
		t.Fatalf("a won move over REST → %d %q, want 403 approval_required", status, refusal.Code)
	}
	staged := stagedProgressions(t, d.e, viaREST)
	if len(staged) != 1 || !strings.Contains(refusal.Detail, staged[0]) {
		t.Fatalf("the REST door staged %v and answered %q, want one approval named in the answer", staged, refusal.Detail)
	}
	toolRefusal := d.mcp.CallRefused(t, "progress_deal", with(win, "deal_id", viaTool))
	if fromTool := stagedProgressions(t, d.e, viaTool); len(fromTool) != 1 || !strings.Contains(toolRefusal, fromTool[0]) {
		t.Errorf("the tool door staged %v and answered %q, want one approval named in the answer", fromTool, toolRefusal)
	}
	var gated fileRefusal
	if status := d.e.Call(t, "POST", "/v1/deals/"+viaGate+"/advance", win, d.bearer, &gated); status != http.StatusForbidden || gated.Code != refusal.Code {
		t.Errorf("the advance route answers a won move %d %q, and the progress route 403 %q; one rule answers both",
			status, gated.Code, refusal.Code)
	}

	if status := d.e.Call(t, "POST", "/v1/approvals/"+staged[0]+"/approve", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("approving %s → %d", staged[0], status)
	}
	redeem := maps.Clone(d.bearer)
	redeem["X-Approval-Token"] = staged[0]
	var redeemed json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/deals/"+viaREST+"/progress", win, redeem, &redeemed); status != http.StatusOK {
		t.Fatalf("the approved retry → %d %s", status, redeemed)
	}
	if stage := dealStage(t, d.e, viaREST); stage != stages.Won {
		t.Errorf("the approved deal is at stage %s, want the won stage", stage)
	}
	if again := stagedProgressions(t, d.e, viaREST); len(again) != 1 {
		t.Errorf("the redemption left %d approvals on the deal, want the one it redeemed", len(again))
	}
}

type sharedQualification struct {
	Filled map[string]struct {
		Value string `json:"value"`
	} `json:"filled"`
	Gaps []string `json:"gaps"`
}

// A lead with a corporate address and no company gets its company from the
// domain on either door, and the update names the agent.
func TestTheQualifyRouteFillsALeadAsTheToolDoes(t *testing.T) {
	d := newTwoDoors(t, "qualify-doors", "read", "write")
	viaREST := createdID(t, d.e, "/v1/leads", AnyMap{
		"full_name": "Ana Kessler", "email": "ana@kessler-maschinenbau.de", "source": "manual",
	})
	viaTool := createdID(t, d.e, "/v1/leads", AnyMap{
		"full_name": "Ben Kessler", "email": "ben@kessler-maschinenbau.de", "source": "manual",
	})

	qualified := sameRecords[sharedQualification](t, "qualification",
		d.rest(t, "POST", "/v1/leads/"+viaREST+"/qualify", nil),
		d.tool(t, "qualify_lead", map[string]any{"lead_id": viaTool}))
	if qualified.Filled["company_name"].Value == "" {
		t.Errorf("the answer fills no company from the corporate domain: %+v", qualified)
	}
	for _, lead := range []string{viaREST, viaTool} {
		assertTheAgentWrote(t, d.e, lead, "update")
	}
}

type sharedDrafts struct {
	Segment string `json:"segment"`
	Drafts  []struct {
		DealID          string `json:"deal_id"`
		DraftActivityID string `json:"draft_activity_id"`
	} `json:"drafts"`
}

func decodeDrafts(t *testing.T, raw json.RawMessage) sharedDrafts {
	t.Helper()
	var drafts sharedDrafts
	if err := json.Unmarshal(raw, &drafts); err != nil || len(drafts.Drafts) != 1 {
		t.Fatalf("the drafts answer does not hold one draft (%v): %s", err, raw)
	}
	return drafts
}

func quietDeal(t *testing.T, e *apptest.AppEnv, stages apptest.SeededStages, name string, idleDays int) string {
	t.Helper()
	deal := openDeal(t, e, stages, name)
	// No writer takes a past activity time; the idle rule reads these two columns.
	if _, err := e.Owner.Exec(t.Context(), `UPDATE deal SET created_at = now() - make_interval(days => $2),
		last_activity_at = now() - make_interval(days => $2) WHERE id = $1`, deal, idleDays); err != nil {
		t.Fatalf("backdating %q: %v", name, err)
	}
	return deal
}

func notesOn(t *testing.T, e *apptest.AppEnv, deal string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity_link WHERE deal_id = $1`, deal).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Each door drafts on the worst slipping deal it finds. A drafted deal is no
// longer quiet, so the second door drafts on the next one. A REST retry under
// the same Idempotency-Key answers the first drafts and writes none.
func TestTheFollowUpRouteDraftsAsTheToolDoesAndARetryWritesNothing(t *testing.T) {
	d := newTwoDoors(t, "follow-up-doors", "read", "draft")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	winter := quietDeal(t, d.e, stages, "Quiet since winter", 120)
	spring := quietDeal(t, d.e, stages, "Quiet since spring", 90)
	args := AnyMap{"segment": "slipping", "limit": 1}
	keyed := maps.Clone(d.bearer)
	keyed["Idempotency-Key"] = "follow-ups-monday"

	var first json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/deals/follow-up-drafts", args, keyed, &first); status != http.StatusOK {
		t.Fatalf("POST /v1/deals/follow-up-drafts → %d %s", status, first)
	}
	viaREST := decodeDrafts(t, first)
	viaTool := decodeDrafts(t, d.tool(t, "draft_follow_ups_for", args))
	if viaREST.Segment != viaTool.Segment || viaREST.Drafts[0].DealID != winter || viaTool.Drafts[0].DealID != spring {
		t.Errorf("the route drafted %+v and the tool %+v, want the winter deal then the spring one", viaREST, viaTool)
	}
	for _, drafted := range []sharedDrafts{viaREST, viaTool} {
		assertTheAgentWrote(t, d.e, drafted.Drafts[0].DraftActivityID, "create")
	}

	before := notesOn(t, d.e, winter)
	var retried json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/deals/follow-up-drafts", args, keyed, &retried); status != http.StatusOK {
		t.Fatalf("the retry → %d %s", status, retried)
	}
	if compacted(t, retried) != compacted(t, first) {
		t.Errorf("the retry answered\n%s\nwant the first call's answer\n%s", retried, first)
	}
	if after := notesOn(t, d.e, winter); after != before {
		t.Errorf("the retry wrote %d more activities on the winter deal, want none", after-before)
	}
}

// A passport lent without the scope a write needs is refused on the route as on
// the tool. A record that does not exist is not found on either.
func TestTheWriteRoutesRefuseAsTheirToolsDo(t *testing.T) {
	d := newTwoDoors(t, "write-refusals", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := openDeal(t, d.e, stages, "Untouched renewal")
	lead := createdID(t, d.e, "/v1/leads", AnyMap{"full_name": "Cleo Unqualified", "source": "manual"})
	next := openStages(t, d.e)[1]
	writer := d.lend(t, "write-refusals writer", "read", "write", "draft")
	unknown := ids.NewV7().String()

	for _, tc := range []struct {
		name, path, tool string
		as               *twoDoors
		body             AnyMap
		args             map[string]any
		status           int
		code             string
	}{
		{
			"drafts without the draft cap", "/v1/deals/follow-up-drafts", "draft_follow_ups_for", d,
			AnyMap{"segment": "slipping"},
			map[string]any{"segment": "slipping"},
			http.StatusForbidden, scopeRefusalCode,
		},
		{
			"a move without the write cap", "/v1/deals/" + deal + "/progress", "progress_deal", d,
			AnyMap{"to_stage_id": next},
			map[string]any{"deal_id": deal, "to_stage_id": next},
			http.StatusForbidden, scopeRefusalCode,
		},
		{
			"a qualification without the write cap", "/v1/leads/" + lead + "/qualify", "qualify_lead", d,
			AnyMap{},
			map[string]any{"lead_id": lead},
			http.StatusForbidden, scopeRefusalCode,
		},
		{
			"a move of no deal", "/v1/deals/" + unknown + "/progress", "progress_deal", writer,
			AnyMap{"to_stage_id": next},
			map[string]any{"deal_id": unknown, "to_stage_id": next},
			http.StatusNotFound, "not_found",
		},
		{
			"a qualification of no lead", "/v1/leads/" + unknown + "/qualify", "qualify_lead", writer,
			AnyMap{},
			map[string]any{"lead_id": unknown},
			http.StatusNotFound, "not_found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var refusal fileRefusal
			if status := d.e.Call(t, "POST", tc.path, tc.body, tc.as.bearer, &refusal); status != tc.status || refusal.Code != tc.code {
				t.Errorf("POST %s → %d %q, want %d %s", tc.path, status, refusal.Code, tc.status, tc.code)
			}
			tc.as.mcp.CallRefused(t, tc.tool, tc.args)
		})
	}
	if stage := dealStage(t, d.e, deal); stage != stages.Open {
		t.Errorf("a refused move left the deal at stage %s, want it unmoved", stage)
	}
}
