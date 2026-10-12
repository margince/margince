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
	p.WriteString("Regenerate with `" + agentToolBudgetCommand + "`.\n")
	p.WriteString("The figures are what each scheduled agent's step costs the window it runs in before its\n")
	p.WriteString("transcript: the frame, the tool listing and the step schema. Token counts use the\n")
	p.WriteString("~4-bytes-per-token estimate the window itself uses, over what the runner's own\n")
	p.WriteString("renderer produces (`runner.FixedStepCost`).\n\n")

	p.WriteString("## Why this page exists\n\n")
	p.WriteString("A scheduled agent's tool listing is written into the system prompt of **every step**\n")
	p.WriteString("of its run, and each tool's input schema rides every step a second time, as its branch\n")
	p.WriteString("of the step schema the provider enforces. The window elides neither; only the\n")
	p.WriteString("transcript gives way. So a tool attached to an agent is paid for twice on every turn of\n")
	p.WriteString("every run for as long as that agent exists, and what it displaces is the observations\n")
	p.WriteString("the run is reasoning over.\n\n")
	p.WriteString("`agent_loop` is the engine; each of its sites in\n")
	p.WriteString("[`backend/api/ai-tasks.yaml`](../../backend/api/ai-tasks.yaml) is one scheduled agent, and\n")
	p.WriteString("the site's `tools:` are the only tools that run is offered. Read the numbers below\n")
	p.WriteString("before adding one.\n\n")
	fmt.Fprintf(&p, "The window is %d tokens. What an agent's step pays before its transcript (the frame,\n"+
		"the listing and the step schema) may take %d of them (%d/%d).\n\n",
		b.PromptCeiling, b.AgentBudget, listingBudgetNumerator, listingBudgetDenominator)
	p.WriteString("## No run is offered the whole catalog\n\n")
	p.WriteString("A run attaches the tools its goal needs. The whole served catalog is listed below for\n")
	p.WriteString("scale only: no scheduled run, and no certification scenario, is ever offered it. These\n")
	p.WriteString("are the checks that fail when that stops being true:\n\n")
	p.WriteString("| Check | Refuses |\n|---|---|\n")
	for _, held := range b.Held {
		fmt.Fprintf(&p, "| `%s` | %s |\n", held.Check, held.Refuses)
	}
	p.WriteString("\nNothing bounds how many tools a run attaches below the whole catalog except the\n")
	p.WriteString("listing budget above; whether each attached tool is one its goal needs is a reviewer's\n")
	p.WriteString("judgement. An MCP client connecting from outside is served the whole catalog by\n")
	p.WriteString("`tools/list`, but that is its own agent's window rather than a run of this engine.\n\n")
	fmt.Fprintf(&p, "Before any tool is listed the frame itself costs **%d tokens**: the output contract,\n", b.Catalog.Frame)
	p.WriteString("the rules and the prompt fence. It is shown here because moving a rule from each\n")
	p.WriteString("tool's schema into the frame costs one sentence per run instead of one per tool.\n")
	p.WriteString("It is part of every agent's per-step figure below, so a frame that grows a paragraph\n")
	p.WriteString("spends it on every run of every agent.\n\n")

	p.WriteString("## The declared agents\n\n")
	p.WriteString("| Agent | Tools | Of served | Listing | Step schema | Per step | Of the window | Headroom | Dangling refs | Temptation |\n")
	p.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, a := range b.Agents {
		fmt.Fprintf(&p, "| `%s` | %d | %d of %d | %d | %d | %d | %d%% | %d | %d | %d |\n",
			a.Name, len(a.Tools), len(a.Tools), a.OfServed, a.Listing, a.StepSchema, a.Tokens, a.PercentOf,
			a.Headroom, len(a.Dangling), a.Temptation)
	}
	fmt.Fprintf(&p, "| _whole served catalog's listing, for scale; no run is offered it_ | %d |  | %d |  |  | %d%% |  |  |  |\n\n",
		b.Catalog.Tools, b.Catalog.Tokens, percentOf(b.Catalog.Tokens, b.PromptCeiling))

	for _, a := range b.Agents {
		fmt.Fprintf(&p, "### `%s`\n\n", a.Name)
		p.WriteString(verbatimBlock(a.Goal) + "\n")
		fmt.Fprintf(&p, "Attaches %d tools and pays %d tokens on every step (%d listing, %d step schema), leaving\n"+
			"%d of its budget and %d tokens of the\n",
			len(a.Tools), a.Tokens, a.Listing, a.StepSchema, a.Headroom, b.PromptCeiling-a.Tokens)
		p.WriteString("window for the goal, the grounding and everything it reads.\n\n")
		for _, tool := range a.Tools {
			fmt.Fprintf(&p, "- `%s`\n", tool)
		}
		p.WriteString("\n")
		if len(a.Dangling) > 0 {
			fmt.Fprintf(&p, "**%d dangling cross-references**: this agent's own tool copy points at tools it\n", len(a.Dangling))
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
	p.WriteString("description, as that agent's run reads it, while not itself attached. The run's listing\n")
	p.WriteString("already drops an Instead that names a tool outside the offer, so what is left is in\n")
	p.WriteString("Limits and Retain. The rule counts *any mention*, because several disambiguation\n")
	p.WriteString("sentences name a tool without a `Use X when …` clause.\n\n")
	p.WriteString("Use this count to order decisions rather than as a goal. Adding every referenced tool\n")
	p.WriteString("would grow either shipped agent to about 30 tools and ~10,500 tokens, six times the\n")
	p.WriteString("menu. Lowering it is not always an improvement either: a tool its goal needs raises\n")
	p.WriteString("this count whenever its copy names a neighbour.\n\n")
	p.WriteString("**Temptation weight** sums, over an agent's tools, how many of that agent's own\n")
	fmt.Fprintf(&p, "certification scenarios (of %d in the corpus) name the tool as the wrong reach. Each\n", b.Corpus.Scenarios)
	p.WriteString("scenario certifies one scheduled agent's window (its goal and its tools), so a near\n")
	p.WriteString("miss is a tool that run can see and reach for.\n\n")
	p.WriteString("The weight is counted from what scenarios declare; it is not an observed error rate.\n")
	p.WriteString("Each scenario lists the tools its goal makes tempting, in a `near_misses:` list beside its\n")
	p.WriteString("expected step. A weight of 5 does not mean a model went wrong five times; it means five\n")
	p.WriteString("scenarios name a tool on this menu as the reach to avoid. The measurement that\n")
	p.WriteString("would replace it is sampling real runs for chosen-vs-wanted.\n\n")
	if len(b.Corpus.ReadByProse) > 0 {
		fmt.Fprintf(&p, "**%d scenarios declare no near misses**, so theirs are read out of rubric prose by\n",
			len(b.Corpus.ReadByProse))
		p.WriteString("matching registered tool names minus the scenario's own answer. That is wrong in\n")
		p.WriteString("both directions: it counts a tool a rubric merely quotes, and misses one the\n")
		p.WriteString("rubric names in words, since \"a search would re-find the contact\" is search_records\n")
		p.WriteString("to a reader and nothing to a matcher. They are named rather than absorbed:\n\n")
		for _, name := range b.Corpus.ReadByProse {
			fmt.Fprintf(&p, "- %s\n", name)
		}
		p.WriteString("\n")
	}
	if len(b.Corpus.Unoffered) > 0 {
		fmt.Fprintf(&p, "**%d declared near misses name a tool their run is never offered** and are not counted:\n\n", len(b.Corpus.Unoffered))
		for _, u := range b.Corpus.Unoffered {
			fmt.Fprintf(&p, "- %s\n", u)
		}
		p.WriteString("\n")
	}
	if len(b.Corpus.Skipped) > 0 {
		fmt.Fprintf(&p, "**%d scenarios were skipped by the scan** and are named here rather than dropped:\n\n", len(b.Corpus.Skipped))
		for _, s := range b.Corpus.Skipped {
			fmt.Fprintf(&p, "- %s\n", s)
		}
		p.WriteString("\n")
	}

	p.WriteString("## What each tool costs, largest first\n\n")
	fmt.Fprintf(&p, "Median %d tokens, mean %d, across %d served tools.\n\n",
		b.Catalog.Median, b.Catalog.Mean, b.Catalog.Tools)
	p.WriteString("Each row is one tool rendered alone, so the rows do not add up to the catalog total:\n")
	p.WriteString("every row carries its own rounding, and the catalog figure divides the whole rendered\n")
	p.WriteString("listing once. Read a row as what that tool costs a menu.\n\n")
	p.WriteString("| Tool | Tokens | Named as the wrong reach in |\n|---|---:|---:|\n")
	reach := map[string]int{}
	for _, r := range b.WrongReach {
		reach[r.Name] = r.Scenarios
	}
	for _, row := range b.ToolCost {
		named := ""
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
	p.WriteString("- [mcp-info.md](mcp-info.md): the whole served surface as a client receives it,\n")
	p.WriteString("  including the output schemas a run is never charged for.\n")
	p.WriteString("- [agent-tools.md](agent-tools.md): the governed catalog: what each tool is, what it\n")
	p.WriteString("  costs in passport scope, and whether a human must approve it.\n")
	return []byte(p.String())
}

// percentOf matches the payload's own percent_of_ceiling arithmetic, so the
// catalog row and the agent rows above it are the same kind of number. A column
// that mixed 4% with 70.2% would invite the reader to compare precisions rather
// than shares.
func percentOf(n, of int) int { return n * 100 / of }
