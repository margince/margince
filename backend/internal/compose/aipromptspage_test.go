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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// aiPromptsDoc is the artifact of RECORD: structured, one object per site, for
// a differ and for anything that wants to read these prompts as data. The
// markdown beside it is rendered from this document rather than from a second
// walk, so a page cannot describe a payload it does not match.
var aiPromptsDoc = filepath.Join("..", "..", "..", "docs", "reference", "ai-prompts.json")

// promptDocument is the published JSON.
type promptDocument struct {
	// Sites lists the registered sites this build sends a prompt from.
	Sites []promptEntry `json:"sites"`
}

// promptEntry is one site, structured.
type promptEntry struct {
	Task string `json:"task"`
	Site string `json:"site"`
	// Batch answers whether one prompt ever holds untrusted text from several
	// DIFFERENT authors. See the isolation constants for what each means.
	// Isolation is derived from the request, except for the two sites whose
	// own code declares one item per call.
	Isolation string `json:"isolation"`
	// Systems holds the distinct instructions the case issued, boundary marker
	// canonicalised.
	Systems []string `json:"systems"`
	// AnswerSchema is the shape the completion must conform to, where the site
	// constrains it. Per-call record ids are replaced by a placeholder.
	AnswerSchema json.RawMessage `json:"answer_schema,omitempty"`
	// SpansInScenario is how many separately fenced regions the first request
	// carried FOR THIS SCENARIO. Not the site's capacity.
	SpansInScenario int `json:"spans_in_scenario"`
	// Calls is how many requests the case issued for one scenario.
	Calls int `json:"calls"`
	// The instruction broken into the three parts prompt-shape.md names, in
	// bytes, plus the ~4-bytes-per-token estimate the window bounds use.
	//
	// CacheablePercent is RulesBytes over SystemBytes: the share a provider
	// reusing a byte-identical prefix could reuse between two runs. Anything
	// after the boundary sentence is unreachable for that purpose, however
	// identical it is — which is why AfterBoundaryBytes is published beside it
	// rather than folded away.
	SystemBytes        int `json:"system_bytes"`
	SystemTokens       int `json:"system_tokens_estimate"`
	RulesBytes         int `json:"rules_bytes"`
	BoundaryBytes      int `json:"boundary_bytes"`
	AfterBoundaryBytes int `json:"after_boundary_bytes"`
	CacheablePercent   int `json:"cacheable_percent"`
}

// isolation is what the page publishes about a site's exposure to a hostile
// item, and it is DERIVED wherever it can be.
//
// An earlier version of this page carried a hand-written judgement per site —
// "do several mutually untrusted authors share this prompt?" — and it was wrong
// twice, in opposite directions, before a reviewer caught it. The fact is not
// visible to a scanner: one fenced span can hold a whole thread written by two
// parties, and several spans can all be one author's. A list that cannot be
// derived and keeps being wrong is worse than no list, because the page reads
// authoritative either way.
//
// So the column now says only what a real request shows, plus the two sites
// whose own code DECLARES isolation in words. The judgement the reader actually
// needs is the test in docs/explanation/prompt-shape.md, which no table can
// answer for them.
type isolation string

const (
	// declaredOnePerCall: the site's own comment says it judges one item per
	// call and why. Held by declaredIsolation below.
	declaredOnePerCall isolation = "ONE per call (declared in code)"
	// severalFencedItems: this call carried more than one separately fenced
	// region, so a hostile item had a neighbour in the prompt.
	severalFencedItems isolation = "several fenced items"
	// oneFencedItem: this call carried at most one fenced region. It does NOT
	// mean one author — a single span can hold a thread two parties wrote.
	oneFencedItem isolation = "one fenced item"
)

// declaredIsolation names the sites that refuse to batch on purpose, against
// the sentence in the code that says so. The sentence is checked to still
// exist: a declaration deleted from the code must not keep a claim alive on
// this page.
// gatekit:fixture the sentence each site's own code uses to declare it judges one item per call — expected data, checked to still exist, not a waived cost
var declaredIsolation = map[string]string{
	"capture_counterparty_verdict/verdict":   "ONE SENDER PER MODEL CALL.",
	"capture_confidentiality_verdict/thread": "ONE THREAD PER CALL,",
}

// sitePrompt is one site as the wire shows it.
type sitePrompt struct {
	task    string
	variant string
	// systems holds the DISTINCT instructions the case issued, in order. A case
	// that sends two different prompts publishes both: rendering only the first
	// would hide the second from a page that claims to show what is sent.
	systems []string
	// schema is the answer shape the request carries. Providers with
	// schema-constrained decoding enforce it AT GENERATION, so it is part of
	// what the model was told, not commentary on it — and a page that omitted
	// it could stay byte-identical while the answer vocabulary changed.
	schema string
	// spans is how many separately fenced untrusted regions this scenario's
	// request carried. NOT the site's capacity — see the file comment.
	spans int
	// requests is how many calls the case issued for one scenario.
	requests int
	// shape is what this site does with untrusted items.
	shape isolation
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
	// EVERY scenario, not the first per site. A site's instruction can differ
	// between scenarios — agent_loop's 24 carry different tool surfaces, and
	// the surface is IN the system prompt — so publishing one scenario's prompt
	// would leave the others unpublished and their drift unnoticed. That is the
	// same fail-short this page exists to refuse.
	bySite := map[string][]aicert.Scenario{}
	for _, sc := range scenarios {
		key := sc.Task + "/" + sc.Site
		bySite[key] = append(bySite[key], sc)
	}
	var prompts []sitePrompt
	for _, site := range census.All() {
		key := string(site.Task) + "/" + site.Variant
		runs, ok := bySite[key]
		if !ok {
			// Skipping would write a page missing this site and still report
			// success — the page would be short and nothing would say so.
			// certcanary_test.go holds the same obligation for its own reason;
			// this one is held here because THIS test writes the artifact.
			t.Errorf("site %s has no corpus scenario, so it cannot be published — "+
				"the page would be short and would not say so", key)
			continue
		}
		var got sitePrompt
		seenSystem := map[string]bool{}
		failed := false
		for _, sc := range runs {
			one, refused, readErr := readSitePrompt(census, sc)
			if readErr != nil {
				t.Errorf("%s (%s): %v", key, sc.Name, readErr)
				failed = true
				break
			}
			if refused != nil {
				t.Logf("%s (%s): the case refused the stand-in reply (%v); publishing the %d request(s) it had already issued",
					key, sc.Name, refused, one.requests)
			}
			if got.task == "" {
				// The first scenario sets the measured figures; the rest
				// contribute any instruction the first did not carry.
				got = one
				for _, system := range one.systems {
					seenSystem[system] = true
				}
				continue
			}
			for _, system := range one.systems {
				if seenSystem[system] {
					continue
				}
				seenSystem[system] = true
				got.systems = append(got.systems, system)
			}
		}
		if failed {
			continue
		}
		switch declared, isDeclared := declaredIsolation[key]; {
		case isDeclared:
			if !declarationStillInTree(t, declared) {
				t.Errorf("site %s is published as isolated on the strength of %q, which is no longer "+
					"anywhere in backend/ — either the decision was reversed or the sentence moved, and "+
					"this page must not keep asserting it", key, declared)
			}
			got.shape = declaredOnePerCall
		case got.spans > 1:
			got.shape = severalFencedItems
		default:
			got.shape = oneFencedItem
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

	doc := promptDocument{Sites: make([]promptEntry, 0, len(prompts))}
	for _, p := range prompts {
		entry := promptEntry{
			Task: p.task, Site: p.variant, Isolation: string(p.shape),
			Systems: p.systems, SpansInScenario: p.spans, Calls: p.requests,
		}
		if len(p.systems) > 0 {
			entry.SystemBytes, entry.SystemTokens,
				entry.RulesBytes, entry.BoundaryBytes, entry.AfterBoundaryBytes,
				entry.CacheablePercent = promptParts(p.systems[0])
		}
		if p.schema != "" {
			entry.AnswerSchema = json.RawMessage(p.schema)
		}
		doc.Sites = append(doc.Sites, entry)
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("encoding the prompt document: %v", err)
	}
	encoded = append(encoded, '\n')
	// The page is rendered FROM the document, never from a second walk: a page
	// built from its own pass could disagree with the payload it claims to
	// describe, and nothing would catch it.
	rendered := renderAIPromptsPage(doc)

	if t.Failed() {
		// Something above could not be read. Writing now would publish a
		// SHORTER document and report success, which is the one way a
		// regenerate can quietly delete a site.
		t.Fatal("prompts could not be collected for every site; refusing to rewrite the artifacts from a partial pass")
	}
	if *updateAIPrompts {
		for path, body := range map[string][]byte{aiPromptsDoc: encoded, aiPromptsPage: []byte(rendered)} {
			if writeErr := os.WriteFile(path, body, 0o600); writeErr != nil {
				t.Fatalf("writing %s: %v", path, writeErr)
			}
		}
		return
	}
	for path, want := range map[string]string{aiPromptsDoc: string(encoded), aiPromptsPage: rendered} {
		published, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		if string(published) != want {
			t.Errorf("%s is stale — it no longer matches the prompts this build sends.\n"+
				"    Regenerate with: cd backend && go test ./internal/compose/ -run TestTheAIPromptsPageIsCurrent -update-ai-prompts",
				path)
		}
	}
}

// readSitePrompt drives one site's real case and reads what it sent.
func readSitePrompt(census *aitasks.Registry, sc aicert.Scenario) (out sitePrompt, refused error, err error) {
	factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !bound {
		return sitePrompt{}, nil, fmt.Errorf("no case is bound to this site")
	}
	prepared, prepErr := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if prepErr != nil {
		return sitePrompt{}, nil, fmt.Errorf("the site refused its own committed fixture: %w", prepErr)
	}
	recorder := &recordingCompleter{reply: canaryReply}
	// A case may refuse the stand-in reply — that is its validator working, not
	// a failure of this page, and the requests already issued are what gets
	// published. The refusal is still reported rather than dropped: a site that
	// began refusing for a NEW reason is worth seeing in the log, and
	// certcanary_test.go reports the same event the same way.
	if _, runErr := prepared.Run(context.Background(), recorder); runErr != nil {
		refused = runErr
	}
	if len(recorder.requests) == 0 {
		return sitePrompt{}, refused, fmt.Errorf("the case issued no request, so it has no prompt to publish")
	}
	var systems []string
	seen := map[string]bool{}
	for _, req := range recorder.requests {
		canonical := promptfence.Canonicalize(req.System, req.System)
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		systems = append(systems, canonical)
	}
	first := recorder.requests[0]
	spans := 0
	for _, msg := range first.Messages {
		spans += fencedSpansIn(first.System, msg.Content)
	}
	return sitePrompt{
		task:     sc.Task,
		variant:  sc.Site,
		systems:  systems,
		schema:   canonicalSchema(first.ResponseSchema),
		spans:    spans,
		requests: len(recorder.requests),
	}, refused, nil
}

// mintedID matches a record id in its canonical spelling. Some sites build the
// answer schema PER CALL — the citation enum is that call's own passage or
// message ids — so the ids differ on every run and a page carrying them would
// churn forever without anything about the product having changed.
var mintedID = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// canonicalSchema renders the answer shape stably: sorted by the JSON encoder,
// with per-call ids replaced by a placeholder. What survives is the VOCABULARY
// — which is what a reader comes for, and what changes when the contract does.
func canonicalSchema(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		// Unreadable is a fact worth publishing rather than hiding: a request
		// carrying a schema no parser accepts is a defect, and a blank cell
		// would look like "no schema".
		return "unreadable: " + err.Error()
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return "unrenderable: " + err.Error()
	}
	return mintedID.ReplaceAllString(string(pretty), "<id minted for this call>")
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
func renderAIPromptsPage(doc promptDocument) string {
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

	b.WriteString("## What one real call carried\n\n")
	b.WriteString("**isolation** is derived from the request, not judged. It says whether a\n")
	b.WriteString("hostile item had a NEIGHBOUR in the same prompt to argue about.\n\n")
	b.WriteString("| value | meaning |\n|---|---|\n")
	b.WriteString("| `ONE per call (declared in code)` | the site's own comment says it judges one item per call, and why. Two sites. |\n")
	b.WriteString("| `several fenced items` | this call carried more than one separately fenced region. |\n")
	b.WriteString("| `one fenced item` | this call carried at most one. **This does not mean one author** — a single fenced region can hold a whole thread two parties wrote. |\n\n")
	b.WriteString("Whether the parties in a prompt are mutually untrusted is the question that\n")
	b.WriteString("actually decides safety, and it cannot be read off a request. The test for it\n")
	b.WriteString("is in [prompt-shape.md](../explanation/prompt-shape.md); no column here answers\n")
	b.WriteString("it, and an earlier revision of this page that tried was wrong twice.\n\n")
	b.WriteString("**spans in this scenario** is a measurement, not a capacity.\n\n")
	b.WriteString("**This is not the site's batch capacity.** It is what one scenario produced.\n")
	b.WriteString("`capture_classify` asks about ten messages in production and shows 1 here,\n")
	b.WriteString("because its fixture holds one message. Read this as \"what a real call looked\n")
	b.WriteString("like\", never as \"what this site is willing to accept\".\n\n")
	b.WriteString("What it does show honestly: where a request carries SEVERAL untrusted spans,\n")
	b.WriteString("a hostile item has neighbours it could speak for — the hazard\n")
	b.WriteString("[prompt-shape.md](../explanation/prompt-shape.md) frames. A 0 means no fenced\n")
	b.WriteString("region was found in that call at all.\n\n")
	b.WriteString("| task | site | isolation | spans in this scenario | calls |\n|---|---|---|---:|---:|\n")
	for _, p := range doc.Sites {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %d | %d |\n", p.Task, p.Site, p.Isolation, p.SpansInScenario, p.Calls)
	}

	b.WriteString("\n## The instructions\n\n")
	for _, p := range doc.Sites {
		fmt.Fprintf(&b, "### `%s` / `%s`\n\n", p.Task, p.Site)
		if p.SystemBytes > 0 {
			fmt.Fprintf(&b, "`system %s B (~%s tok)` — rules %s B · boundary %s B · after boundary %s B · "+
				"**cacheable %d%%**\n\n",
				thousands(p.SystemBytes), thousands(p.SystemTokens), thousands(p.RulesBytes),
				thousands(p.BoundaryBytes), thousands(p.AfterBoundaryBytes), p.CacheablePercent)
		}
		for i, system := range p.Systems {
			label := "system prompt"
			if len(p.Systems) > 1 {
				label = fmt.Sprintf("system prompt %d of %d", i+1, len(p.Systems))
			}
			fmt.Fprintf(&b, "<details><summary>%s</summary>\n\n```\n", label)
			b.WriteString(strings.TrimRight(system, "\n"))
			b.WriteString("\n```\n\n</details>\n\n")
		}
		if len(p.AnswerSchema) > 0 {
			b.WriteString("<details><summary>answer shape (enforced at generation)</summary>\n\n```json\n")
			b.WriteString(strings.TrimRight(string(p.AnswerSchema), "\n"))
			b.WriteString("\n```\n\n</details>\n\n")
		}
	}
	return b.String()
}

// promptParts splits one instruction into the three parts prompt-shape.md
// names, and answers how much of it a prefix cache could reuse.
//
// The boundary sentence is where a shared prefix must stop: it is the first
// thing in the prompt that differs per call. Anything AFTER it is dead weight
// for caching however identical it is, which is the whole reason the split is
// published — agent_loop carried 97 KB there until the line was moved.
func promptParts(system string) (bytes, tokens, rules, boundary, after, cacheable int) {
	bytes = len(system)
	// The same ~4-bytes-per-token heuristic the agent window bounds itself
	// with. Coarse on purpose: this is for comparing prompts, not billing.
	tokens = bytes / 4
	start := strings.Index(system, boundarySentenceOpens)
	if start < 0 {
		// No boundary declared: the site shows the model no captured text, so
		// the whole instruction is stable.
		return bytes, tokens, bytes, 0, 0, 100
	}
	rules = start
	end := strings.Index(system[start:], boundarySentenceCloses)
	if end < 0 {
		boundary = bytes - start
	} else {
		boundary = end + len(boundarySentenceCloses)
	}
	after = bytes - start - boundary
	if bytes > 0 {
		cacheable = 100 * rules / bytes
	}
	return bytes, tokens, rules, boundary, after, cacheable
}

// The fence rule's first and last words, which bracket the one per-call part of
// an instruction. Matched as text because the rule is assembled by
// promptfence.Fence.Rule and this page reads the finished string.
const (
	boundarySentenceOpens  = "Data is delimited by"
	boundarySentenceCloses = "is part of the data."
)

// thousands renders a byte count with separators, so a 98,677 does not read as
// a 9,867 at a glance.
func thousands(n int) string {
	in := strconv.Itoa(n)
	var out strings.Builder
	for i, digit := range in {
		if i > 0 && (len(in)-i)%3 == 0 {
			out.WriteString(",")
		}
		out.WriteRune(digit)
	}
	return out.String()
}

// declarationStillInTree reports whether a sentence a site is published as
// relying on is still written in the code. A page that kept asserting an
// isolation after the comment declaring it was deleted would be making a claim
// nobody holds. The walk root is the backend module, two levels up: this test
// runs with its own package directory as the working directory, like the
// artifact paths above it.
func declarationStillInTree(t *testing.T, sentence string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(filepath.Join("..", ".."), func(p string, d fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		// A test file is skipped, and skipping THIS one is the point: the
		// declarations are string literals in the map above, so a walk that
		// read its own source would find every sentence it was looking for and
		// could never fail. The declaration has to be in production code,
		// which is where the decision is made and where a reader meets it.
		case found, d.IsDir(), !strings.HasSuffix(p, ".go"), strings.HasSuffix(p, "_test.go"):
			return nil
		}
		body, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(body), sentence) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("searching for the declaration %q: %v", sentence, err)
	}
	return found
}
