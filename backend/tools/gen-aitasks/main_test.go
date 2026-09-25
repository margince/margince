// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"strings"
	"testing"
)

// minimalContract is a 2-tier, 2-task contract shaped like
// api/ai-tasks.yaml but small enough to assert exact literals against.
const minimalContract = `
tiers: [alpha, beta]

tasks:
  foo: {display_name: "Test task foo", ladder: [alpha, beta], execution_mode: background, on_budget_exhausted: queue, status: shipped, sites: [only]}
  bar: {display_name: "Test task bar", ladder: [beta, alpha], execution_mode: interactive, on_budget_exhausted: degrade, status: planned}

degrade_to:
  beta: alpha
  alpha: alpha
`

// TestEmitGoProducesTaskConstantsLaddersAndExecutionModes is the
// Step 1 mechanical property: feeding a minimal contract to emitGo
// produces the constant, the ladder literal, and the derived
// execution-mode table — the shape tasks_gen.go must have for the real contract.
func TestEmitGoProducesTaskConstantsLaddersAndExecutionModes(t *testing.T) {
	c, err := parseContract([]byte(minimalContract))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}

	out, err := emitGo(c, "deadbeef")
	if err != nil {
		t.Fatalf("emitGo: %v", err)
	}

	if !strings.Contains(out, `TaskFoo Task = "foo"`) {
		t.Errorf("generated source missing the TaskFoo constant:\n%s", out)
	}
	if !strings.Contains(out, "TaskFoo: {TierAlpha, TierBeta}") {
		t.Errorf("generated source missing the foo ladder literal:\n%s", out)
	}
	if !strings.Contains(out, "TaskFoo: ExecutionModeBackground") {
		t.Errorf("generated source does not emit TaskFoo as background:\n%s", out)
	}
	if !strings.Contains(out, "TaskBar: ExecutionModeInteractive") {
		t.Errorf("generated source does not emit TaskBar as interactive:\n%s", out)
	}
	if !strings.Contains(out, `const TaskContractHash = "deadbeef"`) {
		t.Errorf("generated source missing TaskContractHash:\n%s", out)
	}
}

// TestParseContractRejectsUnknownLadderTier is the fail-closed property:
// a ladder naming a tier absent from the top-level tiers list is a
// contract defect, not a runtime surprise — the error must name both the
// offending task and the unknown tier so the fix is obvious.
func TestParseContractRejectsUnknownLadderTier(t *testing.T) {
	const bad = `
tiers: [alpha]

tasks:
  foo: {display_name: "Test task foo", ladder: [alpha, gamma], execution_mode: background, on_budget_exhausted: queue, status: planned}

degrade_to:
  alpha: alpha
`
	_, err := parseContract([]byte(bad))
	if err == nil {
		t.Fatal("parseContract accepted a ladder naming an unknown tier")
	}
	if !strings.Contains(err.Error(), "foo") || !strings.Contains(err.Error(), "gamma") {
		t.Errorf("error does not name both the task and the unknown tier: %v", err)
	}
}

// TestParseContractRejectsUnknownDegradeToTier extends the same
// fail-closed rule to degrade_to: a key or value outside the tiers list
// is a contract defect.
func TestParseContractRejectsUnknownDegradeToTier(t *testing.T) {
	const bad = `
tiers: [alpha]

tasks:
  foo: {display_name: "Test task foo", ladder: [alpha], execution_mode: background, on_budget_exhausted: queue, status: planned}

degrade_to:
  alpha: gamma
`
	_, err := parseContract([]byte(bad))
	if err == nil {
		t.Fatal("parseContract accepted a degrade_to naming an unknown tier")
	}
	if !strings.Contains(err.Error(), "gamma") {
		t.Errorf("error does not name the unknown tier: %v", err)
	}
}

func TestParseContractRejectsExecutionModeBudgetPolicyMismatch(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		policy     string
		wantDetail string
	}{
		{name: "interactive queues", mode: "interactive", policy: "queue", wantDetail: "interactive execution_mode requires"},
		{name: "background degrades", mode: "background", policy: "degrade", wantDetail: "background execution_mode requires"},
		{name: "unknown mode", mode: "scheduled", policy: "queue", wantDetail: "execution_mode must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := strings.ReplaceAll(`
tiers: [alpha]

tasks:
  foo: {display_name: "Test task foo", ladder: [alpha], execution_mode: MODE, on_budget_exhausted: POLICY, status: planned}

degrade_to:
  alpha: alpha
`, "MODE", tt.mode)
			raw = strings.ReplaceAll(raw, "POLICY", tt.policy)
			_, err := parseContract([]byte(raw))
			if err == nil {
				t.Fatal("parseContract accepted an invalid execution-mode and budget-policy pairing")
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("error %q does not explain the invalid pairing", err)
			}
		})
	}
}

// The shipped contract must parse under the generator that compiles it. This
// is the pair-landing guard: a mirrored contract carrying fields the parser
// does not know fails generation rather than silently dropping policy.
func TestParseContractAcceptsTheShippedDeclaration(t *testing.T) {
	raw, err := os.ReadFile("../../api/ai-tasks.yaml")
	if err != nil {
		t.Fatalf("reading the shipped contract: %v", err)
	}
	c, err := parseContract(raw)
	if err != nil {
		t.Fatalf("the shipped contract does not parse: %v", err)
	}

	verdict, ok := c.Tasks["capture_counterparty_verdict"]
	if !ok {
		t.Fatal("capture_counterparty_verdict is missing from the contract")
	}
	if !verdict.NoPayload {
		t.Error("capture_counterparty_verdict must declare no_payload: true")
	}
	if verdict.Status != "shipped" {
		t.Errorf("status = %q, want shipped", verdict.Status)
	}

	// A task is not one prompt: rate_extract has always had two.
	if got := len(c.Tasks["rate_extract"].Sites); got != 2 {
		t.Errorf("rate_extract declares %d sites, want 2 (pricing, fx)", got)
	}
	if got := len(c.Tasks["cold_start"].Sites); got != 4 {
		t.Errorf("cold_start declares %d sites, want 4", got)
	}

	// agent_loop is the engine; every one of its sites is a scheduled agent,
	// run in a cumulative tool-fed window and attaching its own tools.
	loop := c.Tasks["agent_loop"].Sites
	if len(loop) == 0 {
		t.Error("agent_loop declares no sites, so no scheduled agent runs on it")
	}
	for _, site := range loop {
		if site.Kind != "agent_loop" || len(site.Tools) == 0 {
			t.Errorf("agent_loop site %+v, want kind agent_loop with its own tools", site)
		}
	}

	// A bare site name defaults to one_shot.
	if got := c.Tasks["rate_extract"].Sites[0].Kind; got != "one_shot" {
		t.Errorf("a bare site name got kind %q, want one_shot", got)
	}

	if c.Embed.CostUnit != "per_entity" {
		t.Errorf("embed.cost_unit = %q, want per_entity", c.Embed.CostUnit)
	}
}

// status governs the whole census: a shipped task owes sites, a planned task
// owes none. Both directions are refused at generation, not discovered later.
func TestValidateRefusesAStatusAndSitesMismatch(t *testing.T) {
	base := `tiers: [cheap_cloud]
degrade_to: {cheap_cloud: cheap_cloud}
tasks:
  t:
    display_name: "Test task t"
    ladder: [cheap_cloud]
    execution_mode: background
    on_budget_exhausted: queue
`
	for name, tail := range map[string]string{
		"shipped without sites": "    status: shipped\n",
		"planned with sites":    "    status: planned\n    sites: [x]\n",
		"unknown status":        "    status: someday\n    sites: [x]\n",
		"unknown kind":          "    status: shipped\n    sites: [{name: x, kind: loop}]\n",
		"duplicate site":        "    status: shipped\n    sites: [x, x]\n",
	} {
		if _, err := parseContract([]byte(base + tail)); err == nil {
			t.Errorf("%s: parsed successfully, want a refusal", name)
		}
	}
}

// A company-context policy that cannot do what it says is an editing mistake,
// and generation is where every task is in view at once. A repeated scope, a
// scope list with no budget to render it under, and a budget or a condition on
// a policy that selects nothing are all refused rather than compiled into a
// table whose entry silently does nothing.
func TestValidateRefusesAnIncoherentCompanyContextPolicy(t *testing.T) {
	base := `tiers: [cheap_cloud]
degrade_to: {cheap_cloud: cheap_cloud}
tasks:
  t:
    display_name: "Test task t"
    ladder: [cheap_cloud]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
`
	for name, tc := range map[string]struct {
		tail       string
		wantDetail string
	}{
		"repeated scope": {
			tail:       "    company_context: {scopes: [identity, identity], token_budget: 300}\n",
			wantDetail: `scope "identity" is declared twice`,
		},
		"scopes without a budget": {
			tail:       "    company_context: {scopes: [identity]}\n",
			wantDetail: "positive token_budget",
		},
		"budget without scopes": {
			tail:       "    company_context: {token_budget: 300}\n",
			wantDetail: "selects no scopes",
		},
		"conditional without scopes": {
			tail:       "    company_context: {conditional: true}\n",
			wantDetail: "selects no scopes",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseContract([]byte(base + tc.tail))
			if err == nil {
				t.Fatal("parsed successfully, want a refusal")
			}
			if !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("error %q does not explain the defect (want %q)", err, tc.wantDetail)
			}
		})
	}
}

// mergeSafetyBase is a minimal-but-valid contract the merge-safety tests
// append a defect to. Each of them asks one question: can a second
// declaration of something reach this generator without being seen?
const mergeSafetyBase = `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: shipped
    sites: [{name: only, kind: one_shot}]
`

// TestAITasksRejectsDuplicateKey pins the property a merged contract rests
// on: a key declared twice is refused, never last-write-wins. An extension
// fragment that re-declared a core task would otherwise silently override
// its routing ladder — and whichever copy survived would be the one nobody
// reviewed.
//
// yaml.v3 already refuses duplicate mapping keys (its decoder's uniqueKeys
// default), at every depth and irrespective of KnownFields. That makes this
// test a pin on a third-party default rather than on our own code, which is
// exactly why it is worth having: the property is load-bearing here and
// currently owned by nothing in this repo.
func TestAITasksRejectsDuplicateKey(t *testing.T) {
	for name, raw := range map[string]string{
		"a task declared twice": mergeSafetyBase + `  foo:
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
`,
		"a top-level block declared twice": mergeSafetyBase + "tiers: [alpha]\n",
		"a field declared twice": `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha]
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
`,
		"a duplicate key inside a site mapping": `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: shipped
    sites:
      - name: only
        name: other
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseContract([]byte(raw))
			if err == nil {
				t.Fatal("parseContract accepted a duplicate key")
			}
			if !strings.Contains(err.Error(), "already defined") {
				t.Fatalf("error %q does not report the duplicate", err)
			}
		})
	}
}

// TestAITasksRejectsSecondDocument refuses a `---` and anything after it.
// Only the first document is decoded, but the fingerprint tasks_gen.go
// carries (TaskContractHash) is the sha256 of the whole FILE — so tasks
// after a separator would be hashed as if they governed routing while
// reaching no table at all, and every downstream drift gate compares one
// generated half against the other rather than against the file.
func TestAITasksRejectsSecondDocument(t *testing.T) {
	raw := mergeSafetyBase + `---
tiers: [beta]
tasks:
  smuggled:
    display_name: "Test task smuggled"
    ladder: [beta]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
`
	c, err := parseContract([]byte(raw))
	if err == nil {
		t.Fatalf("parseContract accepted a second document; the smuggled task reached neither table (tasks = %v)", c.sortedTaskNames())
	}
	if !strings.Contains(err.Error(), "more than one YAML document") {
		t.Fatalf("error %q does not explain the second-document refusal", err)
	}

	// Malformed bytes after the separator are refused too, and reported as
	// what they are. Treating an unreadable tail as "no second document"
	// would let a truncated or corrupted file through on the strength of
	// its first half.
	t.Run("an unreadable tail is reported, not swallowed", func(t *testing.T) {
		_, err := parseContract([]byte(mergeSafetyBase + "---\n*undefined_anchor\n"))
		if err == nil || !strings.Contains(err.Error(), "reading past the first document") {
			t.Fatalf("err = %v, want the read failure to surface", err)
		}
	})
}

// TestAITasksRejectsUnknownField closes the hole KnownFields(true) alone
// does not: yaml.Node.Decode builds its own decoder and does NOT inherit
// the outer decoder's KnownFields setting, so every block with a custom
// UnmarshalYAML is a gap in the strictness parseContract thinks it has. A
// typo there is not a missing declaration but a DIFFERENT one — `kinds:
// agent_loop` on a site leaves Kind at the one_shot default, which is the
// opposite certification posture.
func TestAITasksRejectsUnknownField(t *testing.T) {
	cases := map[string]struct {
		raw   string
		field string
	}{
		"top level": {raw: mergeSafetyBase + "unexpected: 1\n", field: "unexpected"},
		"inside a site mapping": {raw: `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: shipped
    sites: [{name: only, kinds: agent_loop}]
`, field: "kinds"},
		"inside a company_context mapping": {raw: `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
    company_context: {scopes: [identity], token_budget: 300, conditionals: true}
`, field: "conditionals"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseContract([]byte(tc.raw))
			if err == nil {
				t.Fatalf("parseContract silently dropped the unknown field %q", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error %q does not name the unknown field %q", err, tc.field)
			}
		})
	}
}

// The strict read must not cost the shorthand spellings the contract
// actually uses: a bare site name, and `company_context: none`. Both reach
// their custom unmarshaller as SCALAR nodes, which never touch the mapping
// decoder — this pins that the strictness was added on the mapping arm only.
func TestStrictDecodingKeepsTheScalarShorthands(t *testing.T) {
	raw := `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: shipped
    sites: [bare]
    company_context: none
`
	c, err := parseContract([]byte(raw))
	if err != nil {
		t.Fatalf("a shorthand spelling was refused: %v", err)
	}
	site := c.Tasks["foo"].Sites[0]
	if site.Name != "bare" || site.Kind != kindOneShot {
		t.Fatalf("site = %+v, want the bare name defaulted to %s", site, kindOneShot)
	}
	if cc := c.Tasks["foo"].CompanyContext; cc == nil || len(cc.Scopes) != 0 {
		t.Fatalf("company_context = %+v, want the empty policy", cc)
	}
}

// The coherent spellings must keep parsing: a scoped policy with a budget, the
// conditional variant, and the `none` scalar every task without context uses.
func TestValidateAcceptsEveryCoherentCompanyContextPolicy(t *testing.T) {
	base := `tiers: [cheap_cloud]
degrade_to: {cheap_cloud: cheap_cloud}
tasks:
  t:
    display_name: "Test task t"
    ladder: [cheap_cloud]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
`
	for name, tail := range map[string]string{
		"scoped":      "    company_context: {scopes: [identity, offer], token_budget: 300}\n",
		"conditional": "    company_context: {scopes: [identity], token_budget: 300, conditional: true}\n",
		"none":        "    company_context: none\n",
		"undeclared":  "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseContract([]byte(base + tail)); err != nil {
				t.Fatalf("a coherent policy was refused: %v", err)
			}
		})
	}
}

// agentContract declares one agent_loop task whose two sites are two
// scheduled agents, each attaching its own tools. `bar` is the control: a
// task with no agent_loop site has no tool listing to attach anything to.
const agentContract = `
tiers: [alpha, beta]

tasks:
  foo:
    display_name: "Test task foo"
    ladder: [alpha, beta]
    execution_mode: background
    on_budget_exhausted: queue
    status: shipped
    sites:
      - name: overnight_sweep
        kind: agent_loop
        tools: [list_records, log_activity]
      - name: morning_brief
        kind: agent_loop
        tools: [list_records, read_record]
  bar: {display_name: "Test task bar", ladder: [beta, alpha], execution_mode: interactive, on_budget_exhausted: degrade, status: planned}

degrade_to:
  beta: alpha
  alpha: alpha
`

// The declaration is only worth having if it reaches the binary, and in an
// order that does not move between runs: the contract lists the sweep first,
// and the emitted table is still walked in sorted name order.
func TestEmitGoProducesTheDeclaredAgentToolAttachment(t *testing.T) {
	c, err := parseContract([]byte(agentContract))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	out, err := emitGo(c, "deadbeef")
	if err != nil {
		t.Fatalf("emitGo: %v", err)
	}
	for _, want := range []string{
		`{Name: "morning_brief", Tools: []string{"list_records", "read_record"}}`,
		`{Name: "overnight_sweep", Tools: []string{"list_records", "log_activity"}}`,
		"func AgentsFor(t Task) []Agent",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated source missing %s:\n%s", want, out)
		}
	}
	// Read inside the agent table: the site table above it keeps contract order.
	_, table, _ := strings.Cut(out, "var taskAgents")
	brief := strings.Index(table, `Name: "morning_brief"`)
	sweep := strings.Index(table, `Name: "overnight_sweep"`)
	if brief < 0 || sweep < 0 || brief > sweep {
		t.Errorf("agents are not emitted in sorted name order, so the generated file moves between runs")
	}
}

// Each refusal below is a way the declaration could be wrong that a runtime
// would discover late or not at all. ADR-0074's rule for every field in this
// contract is that absent-by-accident is a BUILD error.
func TestParseContractRefusesAMalformedAgentDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contract string
		wantErr  string
	}{
		{
			// The allowlist is the whole point of the declaration, and the
			// runner refuses a job carrying none.
			name:     "an agent declaring no tools",
			contract: strings.Replace(agentContract, "tools: [list_records, read_record]", "tools: []", 1),
			wantErr:  "declares no tools",
		},
		{
			name:     "an agent whose tools key is absent",
			contract: strings.Replace(agentContract, "        tools: [list_records, read_record]\n", "", 1),
			wantErr:  "declares no tools",
		},
		{
			// Only an agent_loop site runs a tool-fed window, so tools on any
			// other kind describe a listing that is never assembled.
			name: "tools on a site that is not an agent_loop",
			contract: strings.Replace(agentContract,
				"      - name: morning_brief\n        kind: agent_loop", "      - name: morning_brief\n        kind: one_shot", 1),
			wantErr: "only an agent_loop site has a tool listing",
		},
		{
			name:     "an agent named outside the identifier rule",
			contract: strings.Replace(agentContract, "name: morning_brief", "name: Morning-Brief", 1),
			wantErr:  "must match",
		},
		{
			name:     "the same tool attached twice",
			contract: strings.Replace(agentContract, "tools: [list_records, read_record]", "tools: [read_record, read_record]", 1),
			wantErr:  "twice",
		},
		{
			// strictdecode.go exists because a custom UnmarshalYAML is a hole in
			// KnownFields. A typo here must fail rather than leave the agent
			// with no allowlist at all.
			name:     "a typo for the tools key",
			contract: strings.Replace(agentContract, "        tools: [list_records, read_record]", "        tool: [list_records, read_record]", 1),
			wantErr:  "field tool not found",
		},
		{
			// The retired spelling must not decode into silence: a contract
			// still carrying agents{} is refused by name.
			name:     "the retired agents mapping",
			contract: strings.Replace(agentContract, "  bar: {", "    agents: {x: {tools: [a]}}\n  bar: {", 1),
			wantErr:  "field agents not found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseContract([]byte(tc.contract))
			if err == nil {
				t.Fatalf("the contract was accepted, so %s reaches the binary", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("the refusal does not say what is wrong: want it to mention %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// contractWith declares one task t on a one-tier contract, its mode, budget
// posture, status and any further fields given by def. A shipped task gets a
// site so the status rule is satisfied and only the field under test decides.
func contractWith(def string) string {
	sites := ""
	if strings.Contains(def, "status: shipped") {
		sites = "\n    sites: [only]"
	}
	return `tiers: [cheap_cloud]
degrade_to: {cheap_cloud: cheap_cloud}
tasks:
  t:
    display_name: "Test task t"
    ladder: [cheap_cloud]
    ` + def + sites + "\n"
}

// A decision attempt is a network call before the ladder, and only a
// background task's deadline has room for it; a planned task has no site for
// an adapter to answer. Both are refused at generation, by name.
func TestADecisionTaskMustShipAndRunInTheBackground(t *testing.T) {
	for name, def := range map[string]string{
		"interactive": "execution_mode: interactive\n    on_budget_exhausted: degrade\n    status: shipped",
		"planned":     "execution_mode: background\n    on_budget_exhausted: queue\n    status: planned",
	} {
		_, err := parseContract([]byte(contractWith(def + "\n    decision: true")))
		if err == nil || !strings.Contains(err.Error(), "decision: true needs") {
			t.Errorf("%s: a decision task must be refused, got %v", name, err)
		}
	}
	shipped := "execution_mode: background\n    on_budget_exhausted: queue\n    status: shipped\n    decision: true"
	if _, err := parseContract([]byte(contractWith(shipped))); err != nil {
		t.Errorf("a shipped background task may declare a decision form, got %v", err)
	}
}

const decisionContract = `
tiers: [alpha, beta]

tasks:
  zed: {display_name: "Test task zed", ladder: [alpha], execution_mode: background, on_budget_exhausted: queue, status: shipped, sites: [only], decision: true}
  foo: {display_name: "Test task foo", ladder: [alpha, beta], execution_mode: background, on_budget_exhausted: queue, status: shipped, sites: [only]}
  abe: {display_name: "Test task abe", ladder: [beta], execution_mode: background, on_budget_exhausted: queue, status: shipped, sites: [only], decision: true, local_only: true}

degrade_to:
  beta: alpha
  alpha: alpha
`

// The declaration reaches the binary as a map for the call site holding a task
// and a list for a walk over every one, the list in sorted name order so the
// generated file does not move between runs. An undeclared task is absent
// from both, which is what keeps a decision lane off a site nobody adapted.
func TestTheDecisionTableListsEveryDeclaredTaskInNameOrder(t *testing.T) {
	c, err := parseContract([]byte(decisionContract))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	out, err := emitGo(c, "deadbeef")
	if err != nil {
		t.Fatalf("emitGo: %v", err)
	}
	for _, want := range []string{
		"var taskDecisions = map[Task]bool{\n\tTaskAbe: true,\n\tTaskZed: true,\n}",
		"func TaskDecides(t Task) bool { return taskDecisions[t] }",
		"var decisionTaskList = []Task{\n\tTaskAbe,\n\tTaskZed,\n}",
		"func DecisionTasks() []Task { return decisionTaskList }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated source missing %q:\n%s", want, out)
		}
	}
}

// The egress page answers for the decision lane too: a decision task's text
// can reach the bound decision model, and a local-only one only a local
// model. A task without the declaration says so.
func TestTheEgressPageNamesEachTasksDecisionReachAndLocalOnlyDeclaration(t *testing.T) {
	c, err := parseContract([]byte(decisionContract))
	if err != nil {
		t.Fatalf("parseContract: %v", err)
	}
	page := string(emitEgressDoc(c))
	for task, want := range map[string]string{
		"abe": "| `abe` | `beta` | no | no | yes | only a local decision model | shipped |",
		"foo": "| `foo` | `alpha` → `beta` | no | no | no | — | shipped |",
		"zed": "| `zed` | `alpha` | no | no | no | the bound decision model | shipped |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the egress row for %s is not %q:\n%s", task, want, page)
		}
	}
}
