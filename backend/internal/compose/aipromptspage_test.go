// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

// docs/reference/ai-prompts.md — every instruction this build sends a model,
// and how many untrusted spans each request carries.
//
// Rendered by DRIVING each site's real case over its own committed corpus
// fixture and reading the requests it issued, exactly as certcanary_test.go
// does. A page that re-assembled prompts from the constants would be a second
// copy of them, free to drift from what production sends; this one cannot,
// because it reads the wire.
//
// The span count says what ONE REAL CALL CARRIED for that site's own committed
// scenario. It is deliberately NOT the site's batch capacity: capture_classify
// asks about ten messages in production and shows one span here, because its
// fixture holds one message. Reading this column as capacity would be reading
// the fixture as the code.
//
// It is still worth publishing, because it is measured rather than claimed: a
// site whose request carries several untrusted spans is one where a hostile
// item has neighbours to speak for, and that is the hazard
// docs/explanation/prompt-shape.md frames. A zero means no fenced region was
// found at all, which is itself worth seeing.
//
// The boundary nonce is canonicalised. It is fresh per call by design, so a
// page carrying the real one would differ on every run and say nothing.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

var updateAIPrompts = flag.Bool("update-ai-prompts", false,
	"rewrite docs/reference/ai-prompts.md from the prompts this build actually sends")

var aiPromptsPage = filepath.Join("..", "..", "..", "docs", "reference", "ai-prompts.md")

// sitePrompt is one site as the wire shows it.
type sitePrompt struct {
	task    string
	variant string
	system  string
	// spans is how many separately fenced untrusted regions this scenario's
	// request carried. NOT the site's capacity — see the file comment.
	spans int
	// requests is how many calls the case issued for one scenario.
	requests int
}

func TestTheAIPromptsPageIsCurrent(t *testing.T) {
	t.Parallel()
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	first := map[string]aicert.Scenario{}
	for _, sc := range scenarios {
		key := sc.Task + "/" + sc.Site
		if _, seen := first[key]; !seen {
			first[key] = sc
		}
	}
	var prompts []sitePrompt
	for _, site := range census.All() {
		key := string(site.Task) + "/" + site.Variant
		sc, ok := first[key]
		if !ok {
			// Every site owes a scenario, and certcanary_test.go is where that
			// is held. Skipping here rather than failing keeps one obligation
			// in one place.
			continue
		}
		got, readErr := readSitePrompt(census, sc)
		if readErr != nil {
			t.Errorf("%s: %v", key, readErr)
			continue
		}
		prompts = append(prompts, got)
	}
	if len(prompts) == 0 {
		// Under-recognition is the one failure this must not have: an empty
		// walk would rewrite the page to nothing and report success.
		t.Fatal("no site yielded a prompt, so this page would be rendered from nothing")
	}
	sort.Slice(prompts, func(i, j int) bool {
		if prompts[i].task != prompts[j].task {
			return prompts[i].task < prompts[j].task
		}
		return prompts[i].variant < prompts[j].variant
	})

	rendered := renderAIPromptsPage(prompts)
	if *updateAIPrompts {
		if writeErr := os.WriteFile(aiPromptsPage, []byte(rendered), 0o600); writeErr != nil {
			t.Fatalf("writing %s: %v", aiPromptsPage, writeErr)
		}
		return
	}
	published, err := os.ReadFile(aiPromptsPage)
	if err != nil {
		t.Fatalf("reading %s: %v", aiPromptsPage, err)
	}
	if string(published) != rendered {
		t.Errorf("%s is stale — it no longer matches the prompts this build sends.\n"+
			"    Regenerate it with: cd backend && go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts",
			aiPromptsPage)
	}
}

// readSitePrompt drives one site's real case and reads what it sent.
func readSitePrompt(census *aitasks.Registry, sc aicert.Scenario) (sitePrompt, error) {
	factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !bound {
		return sitePrompt{}, fmt.Errorf("no case is bound to this site")
	}
	prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if err != nil {
		return sitePrompt{}, fmt.Errorf("the site refused its own committed fixture: %w", err)
	}
	recorder := &recordingCompleter{reply: canaryReply}
	// A case may refuse the stand-in reply — that is its validator working, and
	// the requests it already issued are what this page reads.
	_, _ = prepared.Run(context.Background(), recorder)
	if len(recorder.requests) == 0 {
		return sitePrompt{}, fmt.Errorf("the case issued no request, so it has no prompt to publish")
	}
	req := recorder.requests[0]
	spans := 0
	for _, msg := range req.Messages {
		spans += fencedSpansIn(req.System, msg.Content)
	}
	return sitePrompt{
		task:     sc.Task,
		variant:  sc.Site,
		system:   promptfence.Canonicalize(req.System, req.System),
		spans:    spans,
		requests: len(recorder.requests),
	}, nil
}

// fencedSpansIn counts the separately wrapped untrusted regions in one turn, by
// the boundary the SYSTEM prompt declares. Counting the opening marker is what
// makes "how many untrusted items does this call carry" a fact read off the
// wire rather than a claim about the code.
func fencedSpansIn(system, content string) int {
	marker, declared := promptfence.MarkerIn(system)
	if !declared {
		return 0
	}
	return strings.Count(content, "<"+marker)
}

// renderAIPromptsPage writes the published markdown.
func renderAIPromptsPage(prompts []sitePrompt) string {
	var b strings.Builder
	// The HTML-comment marker is what exempts a generated page from the docs
	// length budget (gates/docspagelength_test.go). The prose below repeats it
	// for a human; the comment is what the gate reads.
	b.WriteString("<!-- Generated: do not edit by hand. -->\n\n")
	b.WriteString("# The prompts this build sends\n\n")
	b.WriteString("Generated. Do not edit by hand — run\n")
	b.WriteString("`cd backend && go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts`.\n\n")
	b.WriteString("Every instruction below was read off a real request, by driving that site's\n")
	b.WriteString("own certification case over its own committed fixture. It is what production\n")
	b.WriteString("sends, not a transcription of it.\n\n")
	b.WriteString("The data boundary is a random marker minted per call; it is shown here as a\n")
	b.WriteString("fixed placeholder so this page does not change on every run. Why it is random,\n")
	b.WriteString("and what follows from it, is in\n")
	b.WriteString("[prompt-shape.md](../explanation/prompt-shape.md).\n\n")

	b.WriteString("## Untrusted spans in one real call\n\n")
	b.WriteString("How many separately fenced regions the first request carried, for that\n")
	b.WriteString("site's own committed scenario.\n\n")
	b.WriteString("**This is not the site's batch capacity.** It is what one scenario produced.\n")
	b.WriteString("`capture_classify` asks about ten messages in production and shows 1 here,\n")
	b.WriteString("because its fixture holds one message. Read this as \"what a real call looked\n")
	b.WriteString("like\", never as \"what this site is willing to accept\".\n\n")
	b.WriteString("What it does show honestly: where a request carries SEVERAL untrusted spans,\n")
	b.WriteString("a hostile item has neighbours it could speak for — the hazard\n")
	b.WriteString("[prompt-shape.md](../explanation/prompt-shape.md) frames. A 0 means no fenced\n")
	b.WriteString("region was found in that call at all.\n\n")
	b.WriteString("| task | site | spans in this scenario | calls |\n|---|---|---:|---:|\n")
	for _, p := range prompts {
		fmt.Fprintf(&b, "| `%s` | `%s` | %d | %d |\n", p.task, p.variant, p.spans, p.requests)
	}

	b.WriteString("\n## The instructions\n\n")
	for _, p := range prompts {
		fmt.Fprintf(&b, "### `%s` / `%s`\n\n", p.task, p.variant)
		b.WriteString("<details><summary>system prompt</summary>\n\n```\n")
		b.WriteString(strings.TrimRight(p.system, "\n"))
		b.WriteString("\n```\n\n</details>\n\n")
	}
	return b.String()
}
