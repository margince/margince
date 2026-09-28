// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
)

// writeDeclarationTables appends the ADR-0074 declaration half of
// tasks_gen.go: the status table, the invocation sites, the payload
// prohibition, the company-context policy, and the cost-unit rule names.
//
// Each table is reached through an accessor rather than an exported map. An
// exported map is a table any caller can mutate, which is not a contract —
// routing.go's existing wrapper around taskLadders is the same call.
//
// taskNames arrives already sorted so every emitted map literal keeps the one
// deterministic order the whole file is written in.
func writeDeclarationTables(b *strings.Builder, c contract, taskNames []string) {
	b.WriteString("// Status reports whether a task ships or is declared-but-unbuilt. A\n")
	b.WriteString("// planned task has no site, no corpus scenario, and no certification\n")
	b.WriteString("// record — which is what stops it presenting as certified.\n")
	b.WriteString(goConstBlockStart)
	fmt.Fprintf(b, "\tStatusShipped = %q\n\tStatusPlanned = %q\n", statusShipped, statusPlanned)
	b.WriteString(")\n\n")
	b.WriteString("var taskStatus = map[Task]string{\n")
	for _, name := range taskNames {
		fmt.Fprintf(b, "\t%s: %q,\n", taskConst(name), c.Tasks[name].Status)
	}
	b.WriteString("}\n\n")
	b.WriteString("// Status returns the declared status, or \"\" for a task this table does\n")
	b.WriteString("// not carry.\nfunc Status(t Task) string { return taskStatus[t] }\n\n")

	writeSiteTable(b, c, taskNames)

	writeAgentTable(b, c, taskNames)

	b.WriteString("// noPayloadTasks are the tasks whose content must NEVER reach\n")
	b.WriteString("// ai_call_payload, whatever the deployment's capture posture says. The\n")
	b.WriteString("// contract pins the prohibition as a hard property, not a default.\n")
	b.WriteString("var noPayloadTasks = map[Task]bool{\n")
	for _, name := range taskNames {
		if c.Tasks[name].NoPayload {
			fmt.Fprintf(b, "\t%s: true,\n", taskConst(name))
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// NoPayload reports the contract's payload prohibition for a task.\n")
	b.WriteString("func NoPayload(t Task) bool { return noPayloadTasks[t] }\n\n")

	writeLocalOnlyTable(b, c, taskNames)

	writeDecisionTable(b, c, taskNames)

	writeCompanyContextTable(b, c, taskNames)

	b.WriteString("// taskCostUnit names each priced task's unit rule. The arithmetic is\n")
	b.WriteString("// behaviour and lives in the estimator; naming the rule here is what\n")
	b.WriteString("// lets the build prove the mapping is total.\n")
	b.WriteString("var taskCostUnit = map[Task]string{\n")
	for _, name := range taskNames {
		if u := c.Tasks[name].CostUnit; u != "" {
			fmt.Fprintf(b, "\t%s: %q,\n", taskConst(name), u)
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// CostUnitFor returns the task's unit-rule name, or \"\" when unpriced.\n")
	b.WriteString("func CostUnitFor(t Task) string { return taskCostUnit[t] }\n\n")
	b.WriteString("// EmbedCostUnit is the embeddings workload's unit rule. embed is not a\n")
	b.WriteString("// task — no prompt, no text answer, no completion path — so it carries\n")
	b.WriteString("// its own contract section and its own accessor.\n")
	fmt.Fprintf(b, "func EmbedCostUnit() string { return %q }\n", c.Embed.CostUnit)
}

// writeLocalOnlyTable emits the egress prohibition: which tasks may be served
// only by a provider running on this machine.
//
// Both a map and a list, which is not duplication. The map answers "may this
// task go out" at a call site that already holds the task; the list is for a
// validator that has to walk every one, and deriving it by ranging the map
// would hand that walk a random order — so a refusal naming "the first
// offending task" would name a different one each run.
func writeLocalOnlyTable(b *strings.Builder, c contract, taskNames []string) {
	b.WriteString("// localOnlyTasks are the tasks whose prompt must never leave this\n")
	b.WriteString("// machine, whatever an operator binds the rungs of their ladder to.\n")
	b.WriteString("//\n")
	b.WriteString("// DECLARED, never inferred from a tier NAME. `local_small` is a rung and\n")
	b.WriteString("// which provider serves it is the operator's to set, so a guarantee\n")
	b.WriteString("// resting on the name holds only until somebody rebinds it — and the\n")
	b.WriteString("// rebinding is legal under every profile but sovereign.\n")
	b.WriteString("var localOnlyTasks = map[Task]bool{\n")
	for _, name := range taskNames {
		if c.Tasks[name].LocalOnly {
			fmt.Fprintf(b, "\t%s: true,\n", taskConst(name))
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// LocalOnly reports the contract's egress prohibition for a task: its\n")
	b.WriteString("// prompt may be served only by a provider running on this machine.\n")
	b.WriteString("func LocalOnly(t Task) bool { return localOnlyTasks[t] }\n\n")
	b.WriteString("// localOnlyTaskList is the same set in contract order.\n")
	b.WriteString("var localOnlyTaskList = []Task{\n")
	for _, name := range taskNames {
		if c.Tasks[name].LocalOnly {
			fmt.Fprintf(b, "\t%s,\n", taskConst(name))
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// LocalOnlyTasks returns the local-only tasks in contract order.\n")
	b.WriteString("func LocalOnlyTasks() []Task { return localOnlyTaskList }\n\n")
}

// writeDecisionTable emits which tasks have a decision form: a site whose
// question a decision model may answer, before the task's ladder is asked.
// A map and a list for the same reason as the local-only table; the list is
// in sorted name order, the order the whole file is written in.
func writeDecisionTable(b *strings.Builder, c contract, taskNames []string) {
	b.WriteString("// taskDecisions are the tasks that declare a decision form. DECLARED,\n")
	b.WriteString("// because a decision site needs an adapter that builds the typed question\n")
	b.WriteString("// from the same inputs its prompt reads, and the census holds the two in step.\n")
	b.WriteString("var taskDecisions = map[Task]bool{\n")
	for _, name := range taskNames {
		if c.Tasks[name].Decision {
			fmt.Fprintf(b, "\t%s: true,\n", taskConst(name))
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// TaskDecides reports whether a task declares a decision form.\n")
	b.WriteString("func TaskDecides(t Task) bool { return taskDecisions[t] }\n\n")
	b.WriteString("// decisionTaskList is the same set in sorted name order.\n")
	b.WriteString("var decisionTaskList = []Task{\n")
	for _, name := range taskNames {
		if c.Tasks[name].Decision {
			fmt.Fprintf(b, "\t%s,\n", taskConst(name))
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("// DecisionTasks returns the decision tasks in sorted name order.\n")
	b.WriteString("func DecisionTasks() []Task { return decisionTaskList }\n\n")
}

// writeCompanyContextTable emits the ADR-0065 policy table.
//
// `company_context: none` decodes to a non-nil empty policy, so the task
// still appears in the map with zero scopes. That distinction is the whole
// point of CompanyContextFor's bool: a DECLARED empty policy is a decision,
// an absent one is a contract defect.
func writeCompanyContextTable(b *strings.Builder, c contract, taskNames []string) {
	b.WriteString("// CompanyContextPolicy is the ADR-0065 anchor-company policy: which\n")
	b.WriteString("// scopes ride the prompt, under what character budget, and whether the\n")
	b.WriteString("// caller must ask for them. Scopes are contract NAMES; the composition\n")
	b.WriteString("// layer maps them to its own scope type, because this package must not\n")
	b.WriteString("// import a sibling module.\n")
	b.WriteString("type CompanyContextPolicy struct {\n\tScopes      []string\n\tTokenBudget int\n\tConditional bool\n}\n\n")
	b.WriteString("var taskCompanyContext = map[Task]CompanyContextPolicy{\n")
	for _, name := range taskNames {
		p := c.Tasks[name].CompanyContext
		if p == nil {
			continue
		}
		fmt.Fprintf(b, "\t%s: {", taskConst(name))
		if len(p.Scopes) > 0 {
			b.WriteString("Scopes: []string{")
			for i, s := range p.Scopes {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(b, "%q", s)
			}
			b.WriteString("}, ")
		}
		fmt.Fprintf(b, "TokenBudget: %d, Conditional: %t},\n", p.TokenBudget, p.Conditional)
	}
	b.WriteString("}\n\n")
	b.WriteString("// CompanyContextFor returns the task's declared policy. The bool reports\n")
	b.WriteString("// whether the contract DECLARES one at all — an undeclared policy is a\n")
	b.WriteString("// contract defect, distinct from a declared empty one.\n")
	b.WriteString("func CompanyContextFor(t Task) (CompanyContextPolicy, bool) {\n\tp, ok := taskCompanyContext[t]\n\treturn p, ok\n}\n\n")
}

// writeAgentTable emits the per-task scheduled-agent allowlists. It is its own
// function because the declaration tables it sits beside are already at the
// length a reader can hold, not because an agent table is a thing apart.
func writeAgentTable(b *strings.Builder, c contract, taskNames []string) {
	b.WriteString("// Agent is one agent_loop site — a scheduled agent — and Tools is what it\n")
	b.WriteString("// attaches. The listing rides in EVERY step of that agent's window, so\n")
	b.WriteString("// this list is both what the run may call and what it pays for in prompt.\n")
	b.WriteString("//\n")
	b.WriteString("// It NARROWS and never grants: every call still passes the same admission\n")
	b.WriteString("// gate against the same passport. A name here the passport does not admit\n")
	b.WriteString("// stays refused.\n")
	b.WriteString("type Agent struct {\n\tName  string\n\tTools []string\n}\n\n")
	b.WriteString("var taskAgents = map[Task][]Agent{\n")
	for _, name := range taskNames {
		agents := agentLoopSites(c.Tasks[name])
		if len(agents) == 0 {
			continue
		}
		fmt.Fprintf(b, "\t%s: {\n", taskConst(name))
		for _, agent := range agents {
			fmt.Fprintf(b, "\t\t{Name: %q, Tools: []string{", agent.Name)
			for i, tool := range agent.Tools {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(b, "%q", tool)
			}
			b.WriteString("}},\n")
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// AgentsFor returns the task's agent_loop sites in sorted name order. A\n")
	b.WriteString("// task that schedules none returns none.\n")
	b.WriteString("func AgentsFor(t Task) []Agent { return taskAgents[t] }\n\n")
}

// writeSiteTable emits each task's invocation sites in contract order.
func writeSiteTable(b *strings.Builder, c contract, taskNames []string) {
	b.WriteString("// Site is one named model-invocation site of a task. A task is NOT one\n")
	b.WriteString("// prompt: rate_extract has two, cold_start four. Kind says how the site\n")
	b.WriteString("// invokes the model, because an agent loop is a cumulative tool-fed\n")
	b.WriteString("// window and must not be described as a request factory. Thinking is the\n")
	b.WriteString("// level the router asks the site's requests to think at, empty for the\n")
	b.WriteString("// binding's own (sitethinking.go says who may override it).\n")
	b.WriteString("type Site struct {\n\tName     string\n\tKind     string\n\tThinking string\n}\n\n")
	b.WriteString(goConstBlockStart)
	fmt.Fprintf(b, "\tSiteKindOneShot   = %q\n", kindOneShot)
	fmt.Fprintf(b, "\tSiteKindMultiTurn = %q\n", kindMultiTurn)
	fmt.Fprintf(b, "\tSiteKindAgentLoop = %q\n", kindAgentLoop)
	b.WriteString(")\n\n")
	b.WriteString("var taskSites = map[Task][]Site{\n")
	for _, name := range taskNames {
		sites := c.Tasks[name].Sites
		if len(sites) == 0 {
			continue
		}
		fmt.Fprintf(b, "\t%s: {\n", taskConst(name))
		for _, s := range sites {
			if s.Thinking != "" {
				fmt.Fprintf(b, "\t\t{Name: %q, Kind: %q, Thinking: %q},\n", s.Name, s.Kind, s.Thinking)
				continue
			}
			fmt.Fprintf(b, "\t\t{Name: %q, Kind: %q},\n", s.Name, s.Kind)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// SitesFor returns the task's declared sites in contract order. A\n")
	b.WriteString("// planned task returns none.\n")
	b.WriteString("func SitesFor(t Task) []Site { return taskSites[t] }\n\n")
}
