// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The reader's half of docs/reference/agent-tool-budget.*.
//
// Every per-agent FIGURE it prints is read from the JSON payload beside it,
// including the derived ones — `percent_of_ceiling` is taken rather than
// recomputed, so the two artifacts cannot say different things about the same
// agent. What the page still computes for itself is presentational: the
// catalog's share of the window, and how much of the window is left once a
// menu is subtracted. Those exist only here and have no payload field to
// disagree with.

import (
	"fmt"
	"strings"
)

func renderAgentToolBudgetPage(b agentToolBudget) []byte {
	var p strings.Builder
	p.WriteString("# What each agent's tool menu costs\n\n")
	p.WriteString("<!-- Generated together with agent-tool-budget.json; do not edit by hand. -->\n\n")
	p.WriteString(b.Note + "\n\n")
	p.WriteString("**This page is generated, and an edit made here is lost.**\n\n")

	p.WriteString("## Why this page exists\n\n")
	p.WriteString("A scheduled agent's tool listing is written into the system prompt of **every step**\n")
	p.WriteString("of its run, and the window never elides it — only the transcript gives way. So a tool\n")
	p.WriteString("attached to an agent is paid for on every turn of every run for as long as that agent\n")
	p.WriteString("exists, and what it displaces is the observations the run is reasoning over.\n\n")
	p.WriteString("Each agent declares its tools in [`backend/api/ai-tasks.yaml`](../../backend/api/ai-tasks.yaml)\n")
	p.WriteString("under `agent_loop`'s `agents:`. Read the numbers below **before** adding one.\n\n")
	fmt.Fprintf(&p, "The window is %d tokens. An agent's listing may take %d of them (%d/%d). The whole\n",
		b.PromptCeiling, b.AgentBudget, listingBudgetNumerator, listingBudgetDenominator)
	fmt.Fprintf(&p, "served catalog is held to %d — a floor for the certification lane, not a budget any\n", b.CatalogFloor)
	p.WriteString("feature is expected to argue with.\n\n")
	fmt.Fprintf(&p, "Before any tool is listed the frame itself costs **%d tokens** — the output contract,\n", b.Catalog.Frame)
	p.WriteString("the rules and the prompt fence. It is published here because a rule moved OUT of the\n")
	p.WriteString("per-tool schemas and INTO the frame trades tools × a sentence for one × a sentence,\n")
	p.WriteString("and only the first half is held by a bound: the floor above measures the LISTING\n")
	p.WriteString("alone. A frame that grows a paragraph spends it on every run of every agent.\n\n")

	p.WriteString("## The declared agents\n\n")
	p.WriteString("| Agent | Tools | Tokens | Of the window | Headroom | Dangling refs | Temptation |\n")
	p.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	for _, a := range b.Agents {
		fmt.Fprintf(&p, "| `%s` | %d | %d | %d%% | %d | %d | %d |\n",
			a.Name, len(a.Tools), a.Tokens, a.PercentOf,
			a.Headroom, len(a.Dangling), a.Temptation)
	}
	fmt.Fprintf(&p, "| _whole served catalog, for scale_ | %d | %d | %d%% | — | — | — |\n\n",
		b.Catalog.Tools, b.Catalog.Tokens, percentOf(b.Catalog.Tokens, b.PromptCeiling))

	for _, a := range b.Agents {
		fmt.Fprintf(&p, "### `%s`\n\n", a.Name)
		fmt.Fprintf(&p, "> %s\n\n", a.Goal)
		fmt.Fprintf(&p, "Attaches %d tools for %d tokens, leaving %d of its budget and %d tokens of the\n",
			len(a.Tools), a.Tokens, a.Headroom, b.PromptCeiling-a.Tokens)
		p.WriteString("window for the goal, the grounding and everything it reads.\n\n")
		for _, tool := range a.Tools {
			fmt.Fprintf(&p, "- `%s`\n", tool)
		}
		p.WriteString("\n")
		if len(a.Dangling) > 0 {
			fmt.Fprintf(&p, "**%d dangling cross-references** — this agent's own tool copy points at tools it\n", len(a.Dangling))
			p.WriteString("cannot call, so a run may spend a step discovering the refusal:\n\n")
			for _, d := range a.Dangling {
				fmt.Fprintf(&p, "- %s\n", d)
			}
			p.WriteString("\n")
		}
	}

	p.WriteString("## How to read the two derived columns\n\n")
	p.WriteString("Both are prose heuristics over text that was written for humans. They are useful for\n")
	p.WriteString("ordering decisions and wrong to optimise against.\n\n")
	p.WriteString("**Dangling references** is any registered tool name appearing in an attached tool's\n")
	p.WriteString("description while not itself attached. The rule is deliberately *any mention*, not a\n")
	p.WriteString("`Use X when …` clause — several disambiguation sentences carry no \"Use\" at all.\n\n")
	p.WriteString("**It is a diagnostic, never a target.** Closing a menu under this relation is\n")
	p.WriteString("unaffordable: either shipped agent's tools close to 30 tools and ~10,500 tokens, six\n")
	p.WriteString("times the menu. And lowering it is not always an improvement — adding\n")
	p.WriteString("`review_commitments` to the sweep *raises* this count while cutting that agent's\n")
	p.WriteString("temptation weight almost in half.\n\n")
	fmt.Fprintf(&p, "**Temptation weight** sums, over an agent's tools, how many of the %d certification\n", b.Corpus.Scenarios)
	fmt.Fprintf(&p, "scenarios name that tool as the WRONG reach. %d of those scenarios offer the model the\n", b.Corpus.OfferingCatalog)
	p.WriteString("whole catalog and score which tool it picks, so the confusions it names were chosen\n")
	p.WriteString("against the real surface rather than guessed.\n\n")
	p.WriteString("**It is a rubric-mention heuristic, not an observed error rate.** The count is\n")
	p.WriteString("registered tool names appearing in a scenario's rubric prose, minus that scenario's\n")
	p.WriteString("own expected step. A weight of 5 does not mean a model went wrong five times; it\n")
	p.WriteString("means five scenario rubrics name a tool on this menu as the reach to avoid. The\n")
	p.WriteString("measurement that would replace it is sampling real runs for chosen-vs-wanted.\n\n")
	p.WriteString("**Two limits, both real.** A scenario's near-misses live only in its `rubric:` free\n")
	p.WriteString("text, so the count is read by matching registered tool names in that prose minus the\n")
	p.WriteString("scenario's own answer — which over-counts, because a rubric quotes the right tool's\n")
	p.WriteString("copy and that copy names others. And each count was measured under a *different*\n")
	p.WriteString("scenario's goal, so summing them over one agent's fixed goal borrows precision the\n")
	p.WriteString("number does not have. Read it as an ordering of which tools cause trouble on this\n")
	p.WriteString("surface, not as a prediction about one agent.\n\n")
	if len(b.Corpus.Skipped) > 0 {
		fmt.Fprintf(&p, "**%d scenarios were skipped by the scan** and are named here rather than dropped:\n\n", len(b.Corpus.Skipped))
		for _, s := range b.Corpus.Skipped {
			fmt.Fprintf(&p, "- %s\n", s)
		}
		p.WriteString("\n")
	} else {
		p.WriteString("Every scenario in the corpus was read; none was skipped.\n\n")
	}

	p.WriteString("## What each tool costs, largest first\n\n")
	fmt.Fprintf(&p, "Median %d tokens, mean %d, across %d served tools.\n\n",
		b.Catalog.Median, b.Catalog.Mean, b.Catalog.Tools)
	p.WriteString("**These do not sum to the catalog total.** Each row is one tool rendered alone and\n")
	p.WriteString("divided by four, so every row carries its own rounding; the catalog figure divides\n")
	p.WriteString("the whole rendered listing once. Read a row as what that tool costs a menu, not as\n")
	p.WriteString("a term in an addition.\n\n")
	p.WriteString("| Tool | Tokens | Named as the wrong reach in |\n|---|---:|---:|\n")
	reach := map[string]int{}
	for _, r := range b.WrongReach {
		reach[r.Name] = r.Scenarios
	}
	for _, row := range b.ToolCost {
		named := "—"
		if n := reach[row.Name]; n > 0 {
			named = fmt.Sprintf("%d scenario", n)
			if n > 1 {
				named += "s"
			}
		}
		fmt.Fprintf(&p, "| `%s` | %d | %s |\n", row.Name, row.Tokens, named)
	}
	p.WriteString("\n")

	p.WriteString("## Related\n\n")
	p.WriteString("- [mcp-info.md](mcp-info.md) — the whole served surface as a client receives it,\n")
	p.WriteString("  including the output schemas a run is never charged for.\n")
	p.WriteString("- [agent-tools.md](agent-tools.md) — the governed catalog: what each tool is, what it\n")
	p.WriteString("  costs in passport scope, and whether a human must approve it.\n")
	return []byte(p.String())
}

// percentOf matches the payload's own percent_of_ceiling arithmetic, so the
// catalog row and the agent rows above it are the same kind of number. A column
// that mixed 4% with 70.2% would invite the reader to compare precisions rather
// than shares.
func percentOf(n, of int) int { return n * 100 / of }
