// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The description gate, derived from the composed registry rather than from a
// list of tools. NewRegistry builds the whole surface with no database — the
// same construction every other sweep over this surface uses — so a tool added,
// renamed or withdrawn reaches these assertions the commit it reaches the
// product, and there is nothing here to keep current by hand.
//
// Each rule is a named function over one spec, and each is proved against a
// spec that BREAKS it as well as against the surface that keeps it. A gate only
// ever run over a clean tree is a gate nobody has seen fail, and one written
// against a defect it cannot actually detect looks exactly like a passing one.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// minimumDescriptionRunes is the length below which a description cannot have
// answered the questions one is for. It is a floor against an entry filled in
// to satisfy the registry, not a style rule: the shortest written entry on this
// surface is several times this.
const minimumDescriptionRunes = 60

// renderableDescriptionDefect names what stops a description being text a
// client can render, or "" when there is none. Registration already refuses an
// empty one, so this is the check registration cannot make: that what was
// written says something, in characters that survive the wire.
func renderableDescriptionDefect(spec mcp.ToolSpec) string {
	if strings.TrimSpace(spec.Description) != spec.Description {
		return "the description is framed by whitespace, which a client renders verbatim"
	}
	if n := len([]rune(spec.Description)); n < minimumDescriptionRunes {
		return "the description is too short to have said what the tool is for, what it does not do, or what to keep from it"
	}
	for _, r := range spec.Description {
		if r < 0x20 || r == 0x7f {
			// Go and JSON string quoting agree on every character but these,
			// and the description is spliced into a JSON response.
			return "the description carries a control character, which Go would quote in a form JSON rejects"
		}
	}
	return ""
}

// governanceOnlyPhrases are the words the GENERATED description was made of.
// A written description may say that a person approves a call — that is a real
// limit a caller plans around — but reaching for these is the generated line
// coming back, and governance already reaches the client appended to the
// written half.
var governanceOnlyPhrases = []string{"Autonomy:", "passport scope", "Maps to ", "auto_execute", "confirmation_required"}

// restatedGovernance names the governance phrase a written description repeats,
// or "" when it states purpose instead. A description that explains how a tool
// is POLICED answers a question no model selecting a tool is asking, and the
// governance clause already answers it.
func restatedGovernance(spec mcp.ToolSpec) string {
	for _, phrase := range governanceOnlyPhrases {
		if strings.Contains(spec.Description, phrase) {
			return phrase
		}
	}
	return ""
}

func TestEveryRegisteredToolIsDescribedInTextAClientCanRender(t *testing.T) {
	for _, spec := range servedSurface(t).Specs() {
		if defect := renderableDescriptionDefect(spec); defect != "" {
			t.Errorf("%s: %s", spec.Name, defect)
		}
	}
}

func TestNoWrittenDescriptionRestatesGovernanceInsteadOfPurpose(t *testing.T) {
	for _, spec := range servedSurface(t).Specs() {
		if phrase := restatedGovernance(spec); phrase != "" {
			t.Errorf("%s: the written description carries %q, which the governance clause already states",
				spec.Name, phrase)
		}
	}
}

// A description that could belong to two tools has not told a model how to
// choose between them, which is the entire job. Exact equality is the only
// version of this a test can hold honestly — near-duplication is an editorial
// judgement — but it is the version that catches a copy-paste.
func TestNoTwoToolsShareADescription(t *testing.T) {
	if first, second, shared := duplicateDescription(servedSurface(t).Specs()); shared {
		t.Errorf("%s and %s are described identically, so nothing in the listing tells them apart", first, second)
	}
}

func duplicateDescription(specs []mcp.ToolSpec) (first, second string, shared bool) {
	owner := make(map[string]string, len(specs))
	for _, spec := range specs {
		if seen, dup := owner[spec.Description]; dup {
			return seen, spec.Name, true
		}
		owner[spec.Description] = spec.Name
	}
	return "", "", false
}

// The two rules above are only worth having if they fire. Each case here is a
// spec carrying exactly one defect, so a rule that silently stopped detecting
// its own subject fails here rather than passing over a clean tree forever.
func TestTheDescriptionRulesFailOnTheDefectsTheyDescribe(t *testing.T) {
	written := "Find people and organizations by name when you do not yet know which record you mean."
	for _, tc := range []struct {
		name string
		spec mcp.ToolSpec
	}{
		{"framed by whitespace", mcp.ToolSpec{Name: "t", Description: " " + written}},
		{"too short to have said anything", mcp.ToolSpec{Name: "t", Description: "Searches."}},
		// \x01 rather than a newline: TrimSpace catches trailing whitespace one
		// branch earlier, so a newline would have proved the framing rule twice
		// and the rune loop never once.
		{"carrying a control character", mcp.ToolSpec{Name: "t", Description: "Find people" + "\x01" + " by name, when you do not yet know which record you mean."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if renderableDescriptionDefect(tc.spec) == "" {
				t.Errorf("a description %s was reported as renderable", tc.name)
			}
		})
	}
	if renderableDescriptionDefect(mcp.ToolSpec{Name: "t", Description: written}) != "" {
		t.Error("a written description was reported as defective, so the rule refuses what the surface ships")
	}

	for _, phrase := range governanceOnlyPhrases {
		if restatedGovernance(mcp.ToolSpec{Name: "t", Description: written + " " + phrase + " read."}) == "" {
			t.Errorf("a description restating %q was not reported", phrase)
		}
	}
	if restatedGovernance(mcp.ToolSpec{Name: "t", Description: written}) != "" {
		t.Error("a purpose-stating description was reported as restating governance")
	}

	if _, _, shared := duplicateDescription([]mcp.ToolSpec{
		{Name: "a", Description: written}, {Name: "b", Description: written},
	}); !shared {
		t.Error("two tools described identically were not reported")
	}
}

// The two surfaces that serve this text must serve the SAME text. The REST
// endpoint's own contract says it "mirrors exactly what an MCP client sees from
// tools/list"; a second rendering here is how that promise quietly stops being
// true.
func TestTheOperatorConsoleServesTheTextAnMCPClientIsServed(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	specs := registry.Specs()
	listed := toolsListDescriptions(t, registry)
	served := agentToolsFromSpecs(specs)
	if len(served) != len(specs) {
		t.Fatalf("the console serves %d tools, the registry holds %d", len(served), len(specs))
	}
	for i, spec := range specs {
		tool := served[i]
		if want := listed[spec.Name]; tool.Description != want {
			t.Errorf("%s: the console serves %q, tools/list serves %q", spec.Name, tool.Description, want)
		}
		if tool.Title != spec.Title {
			t.Errorf("%s: the console does not serve the tool's written display title", spec.Name)
		}
	}
}

// The tool listing may take at most listingBudgetNumerator/listingBudgetDenominator
// of the runner's prompt ceiling. The listing lives in the system prompt, which
// elision never touches — only the transcript gives way — so a catalog that grew
// past this would not overflow, it would quietly leave the run less and less
// room for the observations it is reasoning over.
//
// It was 1/2, and the comment there said half was "generous next to where the
// surface sits today". That stopped being true at 33 tools: the catalog reached
// ~11,900 tokens against a 12,000 bound, and the two tools after it did not fit.
//
// Raised to 5/8 rather than answered by trimming copy, and the choice is worth
// stating because the cheaper option is the wrong one. What is in these
// descriptions is the ONE thing measured to move tool selection — A2 took
// gemini from 0.80 to 0.87 by making tools say what they are for, and took one
// restraint scenario from 0/3 to 3/3 on a single sentence. Cutting that to fit a
// fraction chosen when the catalog was smaller would trade a measured gain for
// an unmeasured one. 5/8 still leaves 9,000 tokens for the goal and the
// transcript, which is a working run.
//
// Scope-filtering the listing did NOT earn 1/2 back, and the arithmetic belongs
// here because the intuition runs the other way. A run is now offered only what
// its passport admits, which cuts the typical run hard — a read-scoped one
// renders ~5,200 tokens rather than ~12,745. But this measures the WHOLE catalog
// deliberately: an all-scope passport is a legitimate configuration, it is
// offered every tool, and at 35 tools that is still ~12,745 — past 1/2 (12,000).
// Re-pointing this at a narrower principal would lower the bound by measuring
// something smaller than the worst case, which is the same failure as raising
// one to fit and harder to see afterwards.
//
// So this stays a ceiling on growth against the all-scope run, and it is closer
// than it looks. The listing is O(catalog): the next few tools reach 5/8 too,
// and what scope filtering leaves behind is mostly schemas, which are half the
// bytes. Deferring those is a protocol change — a model needs the schema to CALL
// a tool — so it wants its own decision rather than being reached for the next
// time this bound is hit.
//
// Publishing the per-record_type write vocabulary as a document rather than
// reciting it in two tool descriptions bought most of the room back: the listing
// measures ~12,132 tokens against the 15,000 bound, ~2,868 of headroom where
// there was ~1,355. That is a measurement and not a constant — descriptions move
// with every change that touches one — so treat it as the order of the headroom.
// What is asserted below is the bound.
//
// The room is deliberately NOT spent by lowering the fraction here. It is what
// the next tools are for, and re-tightening the bound in the change that creates
// the room would spend it before anyone can argue for how.
//
// RAISED to 2/3 on 2026-08-21, with the measurement that argues for it.
//
// 5/8 was set when the listing measured ~12,132 against 15,000. Four tools
// later — whoami, list_colleagues, apply_tag, remove_tag, each answering a
// question the surface previously could not — it measures ~15,377, and the
// last three changes each paid for themselves by cutting prose the tools
// needed: an alias nobody could discover, a language rule trimmed to one
// clause, four tag verbs cut to two. That is the bound spending features
// rather than bounding growth.
//
// The repeated-boilerplate wins were taken first, and they were real:
// retryKeyProperty prints once per mutating tool (eighteen), approval_id's
// description ten times, timestampNote eight. Trimming those three bought
// ~200 tokens between them. What is left is per-tool schema, which is the
// tool, and prose already cut to the bone.
//
// 2/3 of 24,000 is 16,000 — the run still keeps a third of its window, and the
// ~600 tokens this adds is the room the next verb needs rather than a licence
// to stop counting. What has NOT changed is that the listing is O(catalog):
// the honest next answer is scope-filtering or deferring schemas, both of them
// protocol decisions, and both still ahead of us.
//
// RAISED to 17/24 on 2026-08-22, with the measurement that argues for it.
//
// 2/3 was set when the listing measured ~15,377. One tool later —
// read_project_360, the project page read under the same per-section gates
// the HTTP route applies, and a read the surface owed because every one of
// its parts was already agent-readable — it measured ~15,997 before the tool
// and ~16,160 after it, against 16,000.
//
// WHAT CHANGED ON 2026-08-23: the bound stopped being global (#2355).
//
// Every raise above was paid for the same way, and the pattern is the finding:
// the whole catalog was measured because a run offered the whole catalog was a
// legal configuration — Job.Tools empty read as no narrowing, so nothing said
// otherwise. That is no longer true. Each scheduled agent declares its tools in
// api/ai-tasks.yaml, compose refuses to assemble one that does not, and no file
// outside two sanctioned ones may build a Job at all.
//
// So the fraction now bounds a DECLARED agent's listing, and the arithmetic it
// was fighting is gone: the whole catalog is ~16,829 tokens, and the fattest
// agent that actually runs is ~2,190. The bound is ~7.5x the worst real case.
//
// The fraction itself is deliberately UNCHANGED at 17/24. Re-tightening it in
// the change that creates the room would spend the room before anyone can argue
// for how — the same objection this comment has made twice already.
const (
	listingBudgetNumerator   = 17
	listingBudgetDenominator = 24
)

// wholeCatalogBudgetNumerator/Denominator bound the WHOLE catalog, and this is
// a floor rather than a budget — the distinction is the point.
//
// The certification lane really does offer the WHOLE catalog: 21 of the 23
// agent_loop corpus scenarios declare `tools: catalog`, resolved through
// agentLoopCatalog(), each building a real window. (The count is deliberately
// not written here — it said 56 while the surface served 67, and a number in a
// comment nothing checks is one more thing to go quietly wrong.) If that stopped fitting,
// NOTHING WOULD BREAK LOUDLY — window.bounded() elides the transcript only,
// stops at two messages, and sends whatever remains, and the bound providers'
// real contexts dwarf 24,000. The scenarios would keep passing while measuring
// a prompt larger than this build's own stated envelope, and no test anywhere
// would say so.
//
// That is what this holds: the lane's measured envelope, not its survival. A
// certification turn is ONE turn — goal, grounding, one reply, no accumulating
// transcript — so it does not need the 7/24 a forty-step run reserves. 7/8 of
// 24,000 leaves 3,000 for those three, which is ample for a one-turn replay.
//
// No feature is expected to argue with this number, and one that has to is a
// signal about the CEILING rather than about itself. That happened at a
// PromptTokenCeiling of 24,000, where this floor left 63 tokens for a 67-tool
// catalog and the next verb anyone added failed here (margince/margince#3882).
// The ceiling is now derived from the local provider's cap rather than picked.
//
// THE ROOM IS STILL SMALL: a few hundred tokens, which is one or two more
// verbs. That is not an oversight to trim the ceiling for — the slack the
// ceiling holds covers prompt bytes this side cannot count, and spending it
// here buys tool descriptions at the price of truncating runs. When this fails
// again, the question is which tools the certification lane needs to offer at
// once, not how to make the number bigger.
//
// The per-agent bound above is the one that rations anything.
const (
	wholeCatalogBudgetNumerator   = 7
	wholeCatalogBudgetDenominator = 8
)

// Every written description rides in every step of the window of every agent
// that attaches its tool, and nothing in the loop notices if they grow. Each
// declared agent's listing is measured by rendering it — the runner's own
// renderer, not a second spelling of its format — and its tokens are estimated
// by the ~4-bytes rule the window itself estimates with, so this holds the real
// string against the real ceiling.
func TestEachAgentsToolListingLeavesItsRunRoomInTheWindow(t *testing.T) {
	for _, spec := range mustScheduledAgents() {
		if over := listingOverBudget(spec.Name, specsNamed(t, spec.Tools)); over != "" {
			t.Error(over)
		}
	}
}

// listingOverBudget names what is wrong with an agent's listing, or "" when
// nothing is. It is a function over one agent's specs rather than a loop body
// so the refusal can be proved against a listing that breaks it — no shipped
// agent is anywhere near the bound (the fattest is under a seventh of it), so a
// gate written inline here would never once have been seen to fire.
func listingOverBudget(agent string, specs []mcp.ToolSpec) string {
	budget := runner.PromptTokenCeiling * listingBudgetNumerator / listingBudgetDenominator
	tokens := len(runner.ToolListing(specs)) / 4
	if tokens <= budget {
		return ""
	}
	return fmt.Sprintf(
		"agent %q offers a tool listing of ~%d tokens against the %d it may take of a %d-token "+
			"window — the listing is never elided, so what grows here comes out of the observations "+
			"this run is reasoning over",
		agent, tokens, budget, runner.PromptTokenCeiling)
}

// The bound is only worth having if it fires. The whole catalog is ~16,829
// tokens and the budget is 17,000, so even every tool at once does not break it
// — which is the change working, and also why the failing case has to be built
// rather than borrowed.
func TestTheAgentListingBudgetRefusesAListingThatWouldFillTheWindow(t *testing.T) {
	all := servedSurface(t).Specs()
	if over := listingOverBudget("an_agent_attaching_everything_twice", append(append([]mcp.ToolSpec{}, all...), all...)); over == "" {
		t.Error("a listing of twice the whole catalog was reported as within budget, so this bound " +
			"would not stop an agent from filling its own window")
	}
	if over := listingOverBudget("morning_brief", specsNamed(t, []string{"read_record"})); over != "" {
		t.Errorf("a one-tool listing was reported over budget: %s", over)
	}
}

// The whole catalog is not a run's listing, but it IS the certification lane's,
// and the lane has no other statement of what it costs.
func TestTheWholeCatalogStillFitsTheCertificationLanesWindow(t *testing.T) {
	tokens := len(runner.ToolListing(servedSurface(t).Specs())) / 4
	floor := runner.PromptTokenCeiling * wholeCatalogBudgetNumerator / wholeCatalogBudgetDenominator
	if tokens > floor {
		t.Errorf("the whole catalog renders ~%d tokens against the %d this build's window allows it — "+
			"21 of the agent_loop corpus scenarios offer exactly this surface, and nothing would fail "+
			"loudly: the window elides its transcript and sends anyway, so those scenarios would go on "+
			"passing while measuring a prompt larger than the envelope this build claims",
			tokens, floor)
	}
}

// specsNamed resolves an agent's declared tool names against the served
// surface. A name with no tool behind it is TestEveryAgentSpecNamesRegisteredTools'
// finding, not this one — but it must not silently shrink the listing measured
// here, because a menu that is under budget only because a tool went missing is
// the same number for the opposite reason.
func specsNamed(t *testing.T, names []string) []mcp.ToolSpec {
	t.Helper()
	byName := map[string]mcp.ToolSpec{}
	for _, spec := range servedSurface(t).Specs() {
		byName[spec.Name] = spec
	}
	out := make([]mcp.ToolSpec, 0, len(names))
	for _, name := range names {
		spec, registered := byName[name]
		if !registered {
			t.Fatalf("agent tool %q resolves to no registered tool, so this measurement is of a "+
				"listing the product never renders", name)
		}
		out = append(out, spec)
	}
	return out
}

// servedSurface is the core tool surface these rules are held against.
//
// It is the CORE catalog and not the composed one. Reaching the composed set
// from here would mean importing the composition module, which only a role main
// may do (TestCompositionWiredOnlyFromCmd) — and that boundary is worth more
// than the coverage would be. An extension tool is not unchecked in its place:
// Register applies the same per-tool bounds to every tool that comes through
// it, core and extension alike, so no single unit can blow the listing on its
// own. What this leaves unmeasured is a tree that adds MANY units at once,
// which is an installation's own arithmetic to do.
func servedSurface(t *testing.T) *agents.Registry {
	t.Helper()
	return NewRegistry(nil, SendPath{})
}

// toolsListDescriptions is what an MCP client is actually served, read off a
// real request to the real hosted handler — not off the helper the console also
// calls. That distinction is the whole point of the test that uses it:
// comparing two callers of one function proves they agree with each other and
// nothing about what reaches the wire.
//
// It runs over a real server rather than a ResponseRecorder because the handler
// extends its own write deadline per request, which a recorder cannot do — a
// recorder answers 500 before the dispatcher is ever reached.
func toolsListDescriptions(t *testing.T, registry *agents.Registry) map[string]string {
	t.Helper()
	// Every scope, because tools/list is scope-filtered: a caller holding less
	// would be served a shorter listing, and the comparison would silently skip
	// whatever it could not see.
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:descriptions", OnBehalfOf: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeDraft,
			principal.ScopeWrite, principal.ScopeSend, principal.ScopeEnrich),
	})
	handler := agents.NewHTTPHandler(registry,
		func(*http.Request) (context.Context, error) { return ctx, nil },
		nil, "margince-crm", "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("building the tools/list request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// A plain JSON answer, not the SSE framing the handler also offers: this
	// test is about the text in the response, and the stream would wrap it.
	req.Header.Set("Accept", "application/json")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("calling tools/list: %v", err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.Errorf("closing the tools/list response: %v", err)
		}
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading the tools/list response: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("tools/list answered %d: %s", res.StatusCode, body)
	}
	var decoded struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding tools/list: %v\n%s", err, body)
	}
	if len(decoded.Result.Tools) == 0 {
		t.Fatal("tools/list advertised nothing, so there is no served text to compare against")
	}
	out := make(map[string]string, len(decoded.Result.Tools))
	for _, tool := range decoded.Result.Tools {
		out[tool.Name] = tool.Description
	}
	return out
}

// Every served schema is valid JSON and arrives compacted.
//
// schema() compacts what it is given because each InputSchema is rendered
// verbatim into the listing that rides every run, and the literals are written
// indented for the reader of the source. A source-level check cannot see the
// thirteen schemas built by concatenation — they are only a schema once the
// constants are joined — so this asserts on the SERVED specs, where both kinds
// have become the same thing.
//
// The valid-JSON half matters because schema() passes a malformed literal
// through untouched rather than failing: that keeps the mistake visible, and
// this is the test it is visible to.
func TestEverySchemaIsValidJSONAndArrivesCompacted(t *testing.T) {
	for _, spec := range servedSurface(t).Specs() {
		var shape any
		if err := json.Unmarshal(spec.InputSchema, &shape); err != nil {
			t.Errorf("%s: the input schema is not valid JSON: %v", spec.Name, err)
			continue
		}
		// Compared against json.Compact rather than a re-marshal: Marshal also
		// re-escapes (< > & become \u003c and friends), which changes the
		// length without changing the whitespace this is about.
		var compact bytes.Buffer
		if err := json.Compact(&compact, spec.InputSchema); err != nil {
			t.Errorf("%s: the input schema does not compact: %v", spec.Name, err)
			continue
		}
		if len(spec.InputSchema) != compact.Len() {
			t.Errorf("%s: the served schema is %d bytes where its compact form is %d — "+
				"the difference is whitespace, and it rides every prompt",
				spec.Name, len(spec.InputSchema), compact.Len())
		}
	}
}
