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
	// DIFFERENT authors. See the batchShape constants for what each means.
	Batch string `json:"batch"`
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

// batchShape is what a site does with untrusted items, and it is the column a
// reader comes to this page for.
//
// Written out rather than derived, for aitaskregistry's reason: the fact that
// decides it is whether several MUTUALLY UNTRUSTED authors share one prompt,
// and no walk of the syntax can see that. transcript_propose fences its spans
// in a loop and is still ONE transcript from one author; capture_classify does
// the same and carries ten strangers. The two read identically to a scanner.
//
// What keeps the list honest is that it may not go short: every registered site
// must appear below or TestTheAIPromptsPageIsCurrent fails, so a site added
// tomorrow is classified deliberately rather than defaulting to whatever is
// safest to write.
type batchShape string

const (
	// severalAuthors: more than one party's text in one prompt, so a hostile
	// item has a neighbour it could speak for. The parties may be unrelated
	// strangers (capture_classify) or the two sides of one conversation
	// (signal_extract) — the hazard is the same shape either way, and
	// signal_extract's own comment names it.
	severalAuthors batchShape = "several authors"
	// oneAuthorManySpans: several fenced spans, all written by ONE party — a
	// transcript's lines, a document's parts, one member's own writing
	// samples. There is no second author to put words in anyone's mouth.
	oneAuthorManySpans batchShape = "one author, several spans"
	// oneItemPerCall: one untrusted item, deliberately, because a wrong answer
	// is consequential. The isolation IS the protection.
	oneItemPerCall batchShape = "ONE per call (deliberate)"
	// singleSubject: reads one company, deal, meeting or page. The question
	// does not arise.
	singleSubject batchShape = "single subject"
)

// siteBatchShape is the classification, one line per registered site.
var siteBatchShape = map[string]batchShape{
	"account_scan/company_scan":              singleSubject,
	"agent_loop/loop":                        singleSubject,
	"brief_ranking/rank":                     singleSubject,
	"capture_classify/classify":              severalAuthors,
	"capture_confidentiality_verdict/thread": oneItemPerCall,
	"capture_counterparty_verdict/verdict":   oneItemPerCall,
	"cert_judge/judge":                       singleSubject,
	"cold_start/acts":                        singleSubject,
	"cold_start/company_message":             singleSubject,
	"cold_start/field_extract":               singleSubject,
	"cold_start/sitereadmessage":             singleSubject,
	"corpus_ask/corpus_ask":                  severalAuthors,
	"deal_health/deal_status":                singleSubject,
	"document_extract/fields":                oneAuthorManySpans,
	"draft_reply/account":                    singleSubject,
	"draft_reply/contact":                    singleSubject,
	"draft_reply/first":                      singleSubject,
	"draft_reply/intro":                      singleSubject,
	"draft_reply/intro_note":                 singleSubject,
	"draft_reply/reply":                      singleSubject,
	"enrich/signature":                       singleSubject,
	"growth_fit/growth_fit":                  singleSubject,
	"offer_draft/draft":                      singleSubject,
	"owed_verdict/owed":                      severalAuthors,
	"propose_roles/committee":                severalAuthors,
	"rate_extract/fx":                        singleSubject,
	"rate_extract/pricing":                   singleSubject,
	"signal_extract/thread_events":           severalAuthors,
	"site_extract/profile":                   singleSubject,
	"site_fact_extract/page_facts":           singleSubject,
	"site_triage/triage":                     singleSubject,
	"stage_evidence_extract/criteria":        severalAuthors,
	"summarize/company_ask":                  singleSubject,
	"summarize/company_brief":                singleSubject,
	"summarize/company_dossier":              singleSubject,
	"summarize/contact_brief":                singleSubject,
	"summarize/meeting_brief":                singleSubject,
	"summarize/meeting_plan":                 singleSubject,
	"transcript_propose/next_steps":          oneAuthorManySpans,
	"voice_build/demo_draft":                 oneAuthorManySpans,
	"voice_build/derive":                     oneAuthorManySpans,
	"voice_build/eval_draft":                 oneAuthorManySpans,
	"voice_build/eval_scores":                oneAuthorManySpans,
	"weekly_learnings/learn":                 singleSubject,
	"weekly_review/narrative":                singleSubject,
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
	shape batchShape
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
			// Skipping would write a page missing this site and still report
			// success — the page would be short and nothing would say so.
			// certcanary_test.go holds the same obligation for its own reason;
			// this one is held here because THIS test writes the artifact.
			t.Errorf("site %s has no corpus scenario, so it cannot be published — "+
				"the page would be short and would not say so", key)
			continue
		}
		got, refused, readErr := readSitePrompt(census, sc)
		if readErr != nil {
			t.Errorf("%s: %v", key, readErr)
			continue
		}
		if refused != nil {
			t.Logf("%s: the case refused the stand-in reply (%v); publishing the %d request(s) it had already issued",
				key, refused, got.requests)
		}
		shape, classified := siteBatchShape[key]
		if !classified {
			t.Errorf("site %s has no batch classification — add it to siteBatchShape. "+
				"The question is whether several MUTUALLY UNTRUSTED authors share one of its prompts, "+
				"which decides whether a hostile item has a neighbour to speak for", key)
			continue
		}
		got.shape = shape
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
			Task: p.task, Site: p.variant, Batch: string(p.shape),
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

	b.WriteString("## Which sites batch, and what one real call carried\n\n")
	b.WriteString("**batch** is the column that matters. It answers: does one prompt ever hold\n")
	b.WriteString("untrusted text from SEVERAL DIFFERENT AUTHORS?\n\n")
	b.WriteString("| value | meaning |\n|---|---|\n")
	b.WriteString("| `several authors` | more than one party's text in one prompt, so a hostile item has a neighbour it could speak for. Unrelated strangers (`capture_classify`) or the two sides of one conversation (`signal_extract`) — the same hazard either way. |\n")
	b.WriteString("| `one author, several spans` | several fenced regions, all written by ONE party — a transcript's lines, a document's parts. No second author to put words in anyone's mouth. |\n")
	b.WriteString("| `ONE per call (deliberate)` | one item, on purpose, because a wrong answer creates a record or shows somebody's mail. The isolation IS the protection. |\n")
	b.WriteString("| `single subject` | reads one company, deal, meeting or page. The question does not arise. |\n\n")
	b.WriteString("**spans in this scenario** is a measurement, not a capacity.\n\n")
	b.WriteString("**This is not the site's batch capacity.** It is what one scenario produced.\n")
	b.WriteString("`capture_classify` asks about ten messages in production and shows 1 here,\n")
	b.WriteString("because its fixture holds one message. Read this as \"what a real call looked\n")
	b.WriteString("like\", never as \"what this site is willing to accept\".\n\n")
	b.WriteString("What it does show honestly: where a request carries SEVERAL untrusted spans,\n")
	b.WriteString("a hostile item has neighbours it could speak for — the hazard\n")
	b.WriteString("[prompt-shape.md](../explanation/prompt-shape.md) frames. A 0 means no fenced\n")
	b.WriteString("region was found in that call at all.\n\n")
	b.WriteString("| task | site | batch | spans in this scenario | calls |\n|---|---|---|---:|---:|\n")
	for _, p := range doc.Sites {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %d | %d |\n", p.Task, p.Site, p.Batch, p.SpansInScenario, p.Calls)
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
