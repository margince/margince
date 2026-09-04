// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// docs/reference/ai-certification.md and the ai-certification.json beside it —
// what the certification lane covers, as a page rather than a terminal table,
// and as a document a reader can query.
//
// Both are written from the same three trees `make e2e-ai-report` reads: the
// census of invocation sites this build ships, the scenario corpus, and the
// committed records. The judgement itself — current, partial, stale, absent,
// and why — is aicert.Readiness's, called here exactly as the report command
// calls it, so the page and the command can never disagree about what a band
// still claims. The page is then rendered from the JSON document, so the two
// files cannot disagree either.
//
// It is a test rather than a generator binary for the reason mcp-info.md is
// one: the page is only trustworthy if it FAILS when the trees move under it,
// and a build that regenerates silently would commit whatever it found.

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
)

// The three trees these files are rendered from, and the files themselves. The
// trees are named relative to this package, the files by walking up out of the
// Go module — the same reach mcp-info.md's renderer makes.
const (
	aiCertCorpusDir  = "corpus"
	aiCertRecordsDir = "records"
)

var (
	aiCertPage = filepath.Join("..", "..", "..", "..", "docs", "reference", "ai-certification.md")
	aiCertJSON = filepath.Join("..", "..", "..", "..", "docs", "reference", "ai-certification.json")
)

var updateAICert = flag.Bool("update-ai-cert", false,
	"rewrite docs/reference/ai-certification.{md,json} from the corpus, the records and the invocation-site census")

// aiCertRegenerate is the command a reader is sent to run — read by the drift
// failure and printed into the page's own opening line, so both quote whatever
// this constant says.
const aiCertRegenerate = "cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert"

// corpusLinkPrefix turns a repository path into a link a reader of
// docs/reference/ can follow: the page sits two directories below the root.
const corpusLinkPrefix = "../../"

// aiCertCorpusDocs is the corpus directory as the whole tree spells it, used
// for the README links in the page's head.
const aiCertCorpusDocs = "backend/internal/compose/aicert/"

// TestAICertificationPage renders both files and holds the committed copies to
// them.
func TestAICertificationPage(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the invocation-site census: %v", err)
	}
	corpus, err := aicert.LoadCorpus(aiCertCorpusDir, census)
	if err != nil {
		t.Fatalf("loading the certification corpus: %v", err)
	}
	records, err := aicert.LoadRecords(aiCertRecordsDir)
	if err != nil {
		t.Fatalf("loading the certification records: %v", err)
	}
	stamps, perScenario, err := aicert.CurrentStamps(context.Background(), corpus, census)
	if err != nil {
		t.Fatalf("stamping the corpus this build sends: %v", err)
	}
	rows, unclaimed := aicert.Readiness(
		aicert.Census{Sites: census.All(), Scopes: census.Scopes()}, stamps, perScenario, records,
	)

	doc := buildAICertDoc(rows, unclaimed, corpus, records)
	assertAICertDocCoversEverything(t, doc, rows, corpus, records)
	encoded, err := marshalAICertDoc(doc)
	if err != nil {
		t.Fatalf("encoding the certification document: %v", err)
	}
	page := renderAICertPage(doc)
	assertAICertPageCoversEverything(t, string(page), doc)
	syncAICertFile(t, aiCertJSON, encoded)
	syncAICertFile(t, aiCertPage, page)
}

// assertAICertDocCoversEverything is the guard the drift check cannot be: a
// builder that quietly dropped a site, a scenario or a stale reason would still
// match committed files rendered by the same broken builder, and they would
// read as full coverage of whatever happened to be emitted.
//
// So the document is checked against the trees rather than against itself:
// every shipped site, every scenario with a path that resolves, and a stated
// reason on every stale record.
func assertAICertDocCoversEverything(t *testing.T, doc aiCertDoc,
	rows []aicert.ReadinessRow, corpus []aicert.Scenario, records []aicert.Record,
) {
	t.Helper()
	sites := map[string]aiCertSite{}
	for _, site := range doc.Sites {
		sites[site.Key] = site
	}
	for _, row := range rows {
		if _, carried := sites[row.SiteKey()]; !carried {
			t.Errorf("the document has no entry for shipped site %s", row.SiteKey())
		}
	}
	assertAICertRecordsAllAccountedFor(t, doc, records)
	assertAICertCoverageMatchesTheLibrary(t, doc, rows)
	for _, site := range doc.Sites {
		for _, rec := range site.Records {
			if rec.State == aicert.StatusStale && rec.StaleReason == "" {
				t.Errorf("the stale record for %s on %s carries no reason", site.Key, rec.Binding.label())
			}
		}
	}
	// Keyed by site AND name: a name is unique within its task and nothing makes
	// it unique across the corpus, so keying by name alone would let one task's
	// scenario stand in for another's — the census would then read a dropped
	// case as present and check the wrong file for it.
	carried := map[string]aiCertScenario{}
	for _, site := range doc.Sites {
		for _, sc := range site.Scenarios {
			carried[site.Key+"/"+sc.Name] = sc
		}
	}
	for _, sc := range corpus {
		listed, carries := carried[sc.Task+"/"+sc.Site+"/"+sc.Name]
		if !carries {
			t.Errorf("scenario %s of %s/%s is in the corpus but not in the document", sc.Name, sc.Task, sc.Site)
			continue
		}
		if _, err := os.Stat(filepath.Join("..", "..", "..", "..", listed.File)); err != nil {
			t.Errorf("scenario %s is filed at %s, which does not resolve from the repository root: %v",
				sc.Name, listed.File, err)
		}
	}
}

// assertAICertRecordsAllAccountedFor holds the headline "Committed records" to
// what the tables below it enumerate. The total counts the tree; the tables
// carry a record only where it measured a shipped site, or as unclaimed. A
// record reaching neither would leave the page claiming more than it shows, and
// nothing else in this file would notice.
func assertAICertRecordsAllAccountedFor(t *testing.T, doc aiCertDoc, records []aicert.Record) {
	t.Helper()
	shown := map[string]bool{}
	for _, site := range doc.Sites {
		for _, rec := range site.Records {
			shown[site.Task+"/"+rec.Binding.Provider+"/"+rec.Binding.Model+"/"+rec.Binding.Env] = true
		}
	}
	for _, rec := range doc.Unclaimed {
		shown[rec.Task+"/"+rec.Binding.Provider+"/"+rec.Binding.Model+"/"+rec.Binding.Env] = true
	}
	for _, rec := range records {
		if !shown[aicert.RecordKey(rec)] {
			t.Errorf("committed record %s is counted in the totals but appears in no table", aicert.RecordKey(rec))
		}
	}
	if doc.Totals.Records != len(records) {
		t.Errorf("the totals claim %d committed records, and the tree holds %d", doc.Totals.Records, len(records))
	}
}

// assertAICertCoverageMatchesTheLibrary holds this file's scenario cell to
// aicert.ReadinessRow.Coverage(), which is what `make e2e-ai-report` prints for
// the same record. The document keeps numbers and the page renders them, so the
// cell is spelled twice; this is what keeps the two from drifting apart.
func assertAICertCoverageMatchesTheLibrary(t *testing.T, doc aiCertDoc, rows []aicert.ReadinessRow) {
	t.Helper()
	rendered := map[string]string{}
	for _, site := range doc.Sites {
		for _, rec := range site.Records {
			rendered[site.Key+" "+rec.Binding.label()] = coverageCell(rec)
		}
	}
	for _, row := range rows {
		if !row.Certified {
			continue
		}
		key := row.SiteKey() + " " + row.Binding()
		if got, shown := rendered[key]; shown && got != row.Coverage() {
			t.Errorf("%s renders coverage %s where the report command prints %s", key, got, row.Coverage())
		}
	}
}

// assertAICertPageCoversEverything holds the rendered page to the document it
// was rendered from: a site the page drops is a site the reader cannot reach,
// however complete the JSON beside it is.
func assertAICertPageCoversEverything(t *testing.T, page string, doc aiCertDoc) {
	t.Helper()
	for _, site := range doc.Sites {
		if !strings.Contains(page, "#### `"+site.Key+"`") {
			t.Errorf("the page has no section for shipped site %s", site.Key)
		}
		if link := "(#" + aiCertSiteAnchor(site.Key) + ")"; !strings.Contains(page, link) {
			t.Errorf("shipped site %s is in no index row linking to %s", site.Key, link)
		}
		for _, sc := range site.Scenarios {
			if !strings.Contains(page, "`"+sc.Name+"`") {
				t.Errorf("scenario %s is in the document but not on the page", sc.Name)
			}
			if link := "(" + corpusLinkPrefix + sc.File + ")"; !strings.Contains(page, link) {
				t.Errorf("scenario %s is listed without a link to %s", sc.Name, link)
			}
		}
		for _, rec := range site.Records {
			if rec.StaleReason != "" && !strings.Contains(page, rec.StaleReason) {
				t.Errorf("the stale record for %s on %s states no reason on the page", site.Key, rec.Binding.label())
			}
		}
	}
}

// renderAICertPage builds the whole page from the document. Each section is its
// own writer, and each takes only what it renders, so a section can be read
// without holding the rest of the page in mind.
func renderAICertPage(doc aiCertDoc) []byte {
	var page strings.Builder
	writeAICertHead(&page)
	writeAICertTotals(&page, doc.Totals)
	writeAICertGlossary(&page)
	writeAICertIndex(&page, doc.Sites)
	writeAICertBindings(&page, doc.Bindings)
	writeAICertStale(&page, doc.Sites)
	writeAICertSites(&page, doc.Sites)
	writeAICertUnclaimed(&page, doc.Unclaimed)
	return []byte(page.String())
}

func writeAICertHead(page *strings.Builder) {
	page.WriteString("# AI certification\n\n")
	page.WriteString("<!-- Generated from the invocation-site census, the scenario corpus and the committed records; do not edit by hand. -->\n\n")
	page.WriteString("Which models have been measured against what this build actually sends, and how\n")
	page.WriteString("well each one did.\n\n")
	page.WriteString("Generated by `" + aiCertRegenerate + "`; do not edit by hand. It reads the same\n")
	page.WriteString("three trees `make e2e-ai-report` reads: the sites this build registers, the\n")
	page.WriteString("scenarios under [`backend/internal/compose/aicert/corpus/`](" + corpusLinkPrefix + aiCertCorpusDocs + "corpus/README.md),\n")
	page.WriteString("and the records under [`backend/internal/compose/aicert/records/`](" + corpusLinkPrefix + aiCertCorpusDocs + "records/README.md).\n\n")
	page.WriteString("[`ai-certification.json`](ai-certification.json) beside this page holds the same\n")
	page.WriteString("numbers, whole, for a reader who wants to ask a question this page does not\n")
	page.WriteString("answer. This page is rendered from that file.\n\n")
	page.WriteString("Nothing here is a merge gate. The certification lane is paid, manual and\n")
	page.WriteString("BYOK-gated, so a prompt edit turns a record `stale` rather than failing a build.\n\n")
	page.WriteString("How to add a case: [write-a-certification-case.md](../how-to/write-a-certification-case.md).\n")
	page.WriteString("How to certify a model: [certify-an-ai-model.md](../how-to/certify-an-ai-model.md).\n\n")
}

// writeAICertGlossary defines every state word, every column and every dash
// once, here. The page carries one table per shipped site and a reader arrives
// in the middle of it, so a caveat kept beside the table it qualifies is either
// repeated as many times as there are sites or missed.
func writeAICertGlossary(page *strings.Builder) {
	page.WriteString("## How to read this page\n\n")
	page.WriteString("| Word | What it means |\n|---|---|\n")
	page.WriteString("| Task | One job the product does with a model, such as `draft_reply` or `cold_start`. |\n")
	page.WriteString("| Site | One spot inside a task where the product asks a model something. A task can have several: `cold_start` asks four separate questions, so it has four sites. Everything on this page is counted per site, because that is what a test case names. |\n")
	page.WriteString("| Scenario | One test case: what the model is given, and what a good answer looks like. |\n")
	page.WriteString("| Binding | The exact setup a model was tested on: who supplies it, which model, and where it runs. A result counts for that setup only — the same model reached another way has to be tested again. |\n")
	page.WriteString("| Record | The saved result of one paid test run: one binding, one task. |\n")
	page.WriteString("| Band | The grade a record gives its setup. `certified`: good enough to ship. `supported_degraded`: it works, but worse. `not_supported`: do not ship it. |\n\n")
	page.WriteString("### The four states\n\n")
	page.WriteString("A state says whether a saved result still describes the product as it is\ntoday. It says nothing about how well the model did — that is the band.\n\n")
	page.WriteString("| State | What it claims |\n|---|---|\n")
	page.WriteString("| `current` | Still true. Nothing it tested has changed, and there is no test case it missed. |\n")
	page.WriteString("| `partial` | Still true about what it tested, but test cases have been added since that it never saw. The `Scenarios` column says how many of each. |\n")
	page.WriteString("| `stale` | Out of date. Something it tested has changed since — a test case, or the wording the product now sends. The grade no longer describes what ships, and the run has to be paid for again. |\n")
	page.WriteString("| `absent` | Never tested, on any setup. Not a failure — an honest gap. Its columns are dashes because nothing has measured it. |\n\n")
	page.WriteString("### The numbers\n\n")
	page.WriteString("| Column | What it says |\n|---|---|\n")
	page.WriteString("| Runs, Passed | How many times the model was asked, and how often it did what the test case wanted. |\n")
	page.WriteString("| Reliability | Passed divided by Runs. 1.00 is every attempt. |\n")
	page.WriteString("| `accepted`, `wrong_answer`, `invalid`, `abstained` | What kind of answer came back — not a pass/fail split. Some test cases want the model to decline, and an answer it gave instead is a failure even though it counts as `accepted`. |\n")
	page.WriteString("| Scenarios | How many of the site's test cases the saved result still covers, out of how many the site has today. |\n")
	page.WriteString("| Record p50, p95 | How long answers took: the middle one, and a slow one (only 1 in 20 was slower). Both belong to the whole test run, not to the single site whose table they appear in, so a run covering several sites shows the same pair on each. |\n")
	page.WriteString("| Slowest p95 | The worst p95 of any run in that row — never an average of them. Averaging these numbers would invent a figure nothing actually measured. |\n")
	page.WriteString("| `-` | Not measured. No runs is not a reliability of zero, and no timing is not a fast one. Older results carry one stamp for the whole task instead of one per test case, so their `Scenarios` cell is a dash too. |\n\n")
}

// writeAICertTotals opens with the numbers a reader wants before any table:
// how much of what ships is certified at all, and how much corpus and how many
// records are behind that answer.
func writeAICertTotals(page *strings.Builder, totals aiCertTotals) {
	page.WriteString("## Totals\n\n| | |\n|---|---:|\n")
	fmt.Fprintf(page, "| Shipped invocation sites | %d |\n", totals.Sites)
	fmt.Fprintf(page, "| … best state `current` | %d |\n", totals.SitesBestCurrent)
	fmt.Fprintf(page, "| … best state `partial` | %d |\n", totals.SitesBestPartial)
	fmt.Fprintf(page, "| … best state `stale` | %d |\n", totals.SitesBestStale)
	fmt.Fprintf(page, "| … `absent` on every binding | %d |\n", totals.SitesAbsent)
	fmt.Fprintf(page, "| Scenarios in the corpus | %d |\n", totals.Scenarios)
	fmt.Fprintf(page, "| Committed records | %d |\n", totals.Records)
	fmt.Fprintf(page, "| Bindings measured | %d |\n\n", totals.Bindings)
	page.WriteString("A site's *best* state is the strongest state any of its bindings reached. A\n")
	page.WriteString("site `current` on one model and `stale` on three is counted once, as\n")
	page.WriteString("`current`; the per-binding truth is in the tables below.\n\n")
}

// writeAICertIndex is the jump table. The page carries a section per site, more
// than a reader will scroll, and the cells beside each link are what they would
// have scrolled to find.
func writeAICertIndex(page *strings.Builder, sites []aiCertSite) {
	page.WriteString("## Index\n\n")
	fmt.Fprintf(page, "### Sites (%d)\n\n", len(sites))
	page.WriteString("Which model to run each site on, and what that choice rests on.\n\n")
	page.WriteString("| Site | Best model tested | Band | Reliability | State | Scenarios | Records |\n")
	page.WriteString("|---|---|---|---:|---|---:|---:|\n")
	for _, site := range sites {
		fmt.Fprintf(page, "| [`%s`](#%s) | %s | `%s` | %d | %d |\n",
			site.Key, aiCertSiteAnchor(site.Key), aiCertPickCells(site.Recommended), site.BestState,
			len(site.Scenarios), len(site.Records))
	}
	page.WriteString("\n")
	page.WriteString("**Best model tested** is the strongest result that still describes what ships.\n")
	page.WriteString("Only a `current` or `partial` record is eligible; among those the pick is the\n")
	page.WriteString("best band, then the best reliability, then the fastest.\n\n")
	page.WriteString("Read the band beside it before running anything on it. `certified` is a safe\n")
	page.WriteString("pick. `supported_degraded` works, but worse. `not_supported` means the best\n")
	page.WriteString("model anyone has measured on that site still is not good enough to ship — it\n")
	page.WriteString("names work to do, not a model to choose. A dash means every result for that\n")
	page.WriteString("site is out of date or missing, and the State column says which.\n\n")
}

// aiCertPickCells renders the three cells naming the recommendation: which
// binding, what band it reached, and how reliable it was. A site whose every
// record is out of date names nothing at all — printing its best band anyway
// would point at a measurement of a product that no longer exists.
func aiCertPickCells(pick *aiCertPick) string {
	if pick == nil {
		return aicert.Unmeasured + " | " + aicert.Unmeasured + " | " + aicert.Unmeasured
	}
	return fmt.Sprintf("`%s` | `%s` | %s", pick.Binding.label(), pick.Band, reliabilityCell(pick.Reliability))
}

// aiCertSiteAnchor is the fragment GitHub derives from a site's heading. A site
// key is `task/variant` over the identifier characters a Go package name allows,
// so lowercasing and dropping the slash is the whole of the rule here: the
// section `cold_start/acts` is reached as #cold_startacts.
func aiCertSiteAnchor(siteKey string) string {
	var fragment strings.Builder
	for _, r := range strings.ToLower(siteKey) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			fragment.WriteRune(r)
		}
	}
	return fragment.String()
}

// writeAICertBindings is the certification table per provider and model — one
// line per deployment somebody has actually paid to measure.
func writeAICertBindings(page *strings.Builder, bindings []aiCertBinding) {
	page.WriteString("## Certification by provider and model\n\n")
	page.WriteString("One row per binding, folded over every site it measured. Sites is how many\n")
	page.WriteString("shipped sites this binding has been run against, not how many exist; the\n")
	page.WriteString("state columns split those sites by whether the measurement still describes\n")
	page.WriteString("what this build sends, and the band columns split the same sites by the\n")
	page.WriteString("verdict each reached. Each record's own p50 and p95 are in the site tables.\n\n")
	page.WriteString("| Provider | Model | Env | Sites | `current` | `partial` | `stale` | Runs | Passed | Reliability | Slowest p95 | `certified` | `supported_degraded` | `not_supported` |\n")
	page.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, b := range bindings {
		fmt.Fprintf(page, "| `%s` | `%s` | `%s` | %d | %d | %d | %d | %d | %d | %s | %s | %d | %d | %d |\n",
			b.Binding.Provider, b.Binding.Model, b.Binding.Env, b.Sites,
			b.SitesCurrent, b.SitesPartial, b.SitesStale,
			b.Runs, b.Passed, reliabilityCell(b.Reliability), latencyCell(b.SlowestP95MS),
			b.Bands.Certified, b.Bands.SupportedDegraded, b.Bands.NotSupported)
	}
	page.WriteString("\n")
}

// writeAICertStale is the question this page exists to answer for a release
// decision: what is no longer a claim, and what it would cost to make it one
// again. A count alone cannot answer the second half.
func writeAICertStale(page *strings.Builder, sites []aiCertSite) {
	page.WriteString("## Stale records, and why\n\n")
	type staleRow struct{ site, binding, reason string }
	var stale []staleRow
	for _, site := range sites {
		for _, rec := range site.Records {
			if rec.State == aicert.StatusStale {
				stale = append(stale, staleRow{site.Key, rec.Binding.label(), rec.StaleReason})
			}
		}
	}
	if len(stale) == 0 {
		page.WriteString("None: every committed record still describes the scenarios, and the prompts,\n")
		page.WriteString("that this build sends.\n\n")
		return
	}
	page.WriteString("A record goes stale when a scenario it measured changes, or when the prompt\n")
	page.WriteString("this build now builds from that scenario changes. The stamp covers both, so a\n")
	page.WriteString("prompt edit invalidates a record that never mentioned it. A scenario the\n")
	page.WriteString("record never saw does not make it stale; that is `partial`.\n\n")
	page.WriteString("*Predates per-scenario stamps* means the record carries one stamp for the\n")
	page.WriteString("whole task and nothing finer, so it can only say that something it measured\n")
	page.WriteString("moved — never which case. Re-certifying it costs the whole task; a record\n")
	page.WriteString("with per-scenario stamps names the cases, and costs only those.\n\n")
	page.WriteString("Re-certify with `make e2e-ai TASK=<task> MODEL=<provider:model>` (paid: real\n")
	page.WriteString("model, real network).\n\n")
	page.WriteString("| Site | Binding | Why it is stale |\n|---|---|---|\n")
	for _, row := range stale {
		fmt.Fprintf(page, "| `%s` | `%s` | %s |\n", row.site, row.binding, row.reason)
	}
	page.WriteString("\n")
}

// writeAICertSites is the body: every shipped site, the scenarios it is scored
// against today, and every record measured on it. The scenario list is the
// half a reader cannot get from the terminal report at all — a band says how
// well a model did, and only the corpus says at WHAT.
func writeAICertSites(page *strings.Builder, sites []aiCertSite) {
	page.WriteString("## Sites, their scenarios and their records\n\n")
	page.WriteString("One section per task, one subsection per site it ships. The scenario table\n")
	page.WriteString("says what a site is scored against; the record table says how each binding\n")
	page.WriteString("did. What the columns mean is in [How to read this page](#how-to-read-this-page).\n\n")
	task := ""
	for _, site := range sites {
		if site.Task != task {
			task = site.Task
			fmt.Fprintf(page, "### `%s`\n\n", task)
			writeAICertTaskContext(page, sites, task)
		}
		writeAICertSite(page, site)
	}
}

// writeAICertTaskContext states, once per task, what its contract has
// production prepend that no certification run was served. The cert lane has no
// database to assemble company context from, and for most tasks that costs
// nothing — their contract prepends none. Where it does, the certified prompt
// is short exactly what production always supplies, and no column can say so.
func writeAICertTaskContext(page *strings.Builder, sites []aiCertSite, task string) {
	for _, site := range sites {
		if site.Task != task || len(site.MissingContextScopes) == 0 {
			continue
		}
		fmt.Fprintf(page, "Certified without the company context production prepends (`%s`): "+
			"this lane runs with no database to assemble it from.\n\n",
			strings.Join(site.MissingContextScopes, "`, `"))
		return
	}
}

func writeAICertSite(page *strings.Builder, site aiCertSite) {
	fmt.Fprintf(page, "#### `%s`\n\n", site.Key)
	fmt.Fprintf(page, "Scope a run of it can claim: `%s`.\n\n", site.Scope)
	writeAICertScenarios(page, site.Scenarios)
	writeAICertSiteRecords(page, site.Records)
}

// writeAICertScenarios lists what this site is actually scored against, with a
// link to each case. A scenario's file name and its `name:` are different
// strings — nothing else in the tree maps one to the other — so the link is the
// only way a reader gets from this page to the case.
func writeAICertScenarios(page *strings.Builder, scenarios []aiCertScenario) {
	if len(scenarios) == 0 {
		// TestLoadCorpusCoversEveryShippedSite forbids this, so it can only be
		// read as that gate having been removed rather than as a site nobody got
		// to yet.
		page.WriteString("**No scenario covers this site**, so its prompt ships uncertified.\n\n")
		return
	}
	fmt.Fprintf(page, "Scenarios (%d):\n\n", len(scenarios))
	page.WriteString("| Scenario | Expects | Case |\n|---|---|---|\n")
	for _, sc := range scenarios {
		fmt.Fprintf(page, "| `%s` | `%s` | [%s](%s) |\n",
			sc.Name, sc.Expects, filepath.Base(sc.File), corpusLinkPrefix+sc.File)
	}
	page.WriteString("\n")
}

func writeAICertSiteRecords(page *strings.Builder, records []aiCertRecord) {
	if len(records) == 0 {
		page.WriteString("No record: this site has never been certified on any binding.\n\n")
		return
	}
	fmt.Fprintf(page, "Records (%d):\n\n", len(records))
	page.WriteString("| Binding | State | Scenarios | Band | Runs | Passed | Reliability | Record p50 | Record p95 | `accepted` | `wrong_answer` | `invalid` | `abstained` |\n")
	page.WriteString("|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, rec := range records {
		fmt.Fprintf(page, "| `%s` | `%s` | %s | `%s` | %d | %d | %s | %s | %s | %d | %d | %d | %d |\n",
			rec.Binding.label(), rec.State, coverageCell(rec), rec.Band,
			rec.Runs, rec.Passed, reliabilityCell(rec.Reliability),
			latencyCell(rec.LatencyP50MS), latencyCell(rec.LatencyP95MS),
			rec.Reported.Accepted, rec.Reported.WrongAnswer, rec.Reported.Invalid, rec.Reported.Abstained)
	}
	page.WriteString("\n")
}

// coverageCell is the scenario count behind a state: a `partial` is only
// actionable once a reader knows whether it is 9 of 10 or 1 of 10.
func coverageCell(rec aiCertRecord) string {
	if rec.ScenariosMeasured == nil {
		return aicert.Unmeasured
	}
	return strconv.Itoa(*rec.ScenariosMeasured) + "/" + strconv.Itoa(rec.ScenariosTotal)
}

// latencyCell renders a measured latency, or a dash when there is none. A
// record with no measurement has no latency, and printing 0ms for it would read
// as the fastest binding on the page rather than as the absence of a number.
func latencyCell(ms *int64) string {
	if ms == nil {
		return aicert.Unmeasured
	}
	return strconv.FormatInt(*ms, 10) + "ms"
}

// reliabilityCell renders a pass rate, or a dash when there is nothing to
// divide. Zero runs is not a reliability of zero.
func reliabilityCell(rate *float64) string {
	if rate == nil {
		return aicert.Unmeasured
	}
	return strconv.FormatFloat(*rate, 'f', 2, 64)
}

// writeAICertUnclaimed names each committed record no row above could carry. A
// record nobody enumerates reads as no record at all, which is the failure this
// whole page exists to remove.
func writeAICertUnclaimed(page *strings.Builder, unclaimed []aiCertUnclaimed) {
	if len(unclaimed) == 0 {
		return
	}
	page.WriteString("## Records no shipped site claims\n\n")
	// The task and the binding, not a filename: a model name carrying a slash is
	// folded differently on disk than in a binding, and a path spelled here that
	// nothing opens is worse than no path at all.
	page.WriteString("| Task | Binding | Why no row carries it |\n|---|---|---|\n")
	for _, rec := range unclaimed {
		fmt.Fprintf(page, "| `%s` | `%s` | %s |\n", rec.Task, rec.Binding.label(), rec.Reason)
	}
	page.WriteString("\n")
}

// scenariosBySite indexes the corpus the way the page reads it, each site's
// cases ordered by name so the files are stable across regenerations.
func scenariosBySite(corpus []aicert.Scenario) map[string][]aicert.Scenario {
	bySite := map[string][]aicert.Scenario{}
	for _, sc := range corpus {
		key := sc.Task + "/" + sc.Site
		bySite[key] = append(bySite[key], sc)
	}
	for _, list := range bySite {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	return bySite
}

// aiCertTaskGroups is the order of the files: tasks alphabetically, and each
// task's sites in the order the census yielded them.
func aiCertTaskGroups(rows []aicert.ReadinessRow) ([]string, map[string][]aicert.ReadinessRow) {
	byTask := map[string][]aicert.ReadinessRow{}
	var tasks []string
	for _, row := range rows {
		task := string(row.Site.Task)
		if _, seen := byTask[task]; !seen {
			tasks = append(tasks, task)
		}
		byTask[task] = append(byTask[task], row)
	}
	sort.Strings(tasks)
	return tasks, byTask
}

// siteKeysOf lists a task's sites once each, in the order the census yielded
// them, so the files' shape follows the census rather than a map.
func siteKeysOf(rows []aicert.ReadinessRow) []string {
	var keys []string
	seen := map[string]bool{}
	for _, row := range rows {
		if key := row.SiteKey(); !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

// syncAICertFile compares a rendered file against its committed copy, or
// rewrites it under -update-ai-cert.
//
// The failure names the resolved path and the regeneration command: this
// package reaches both files by walking up out of the Go module, so a package
// move surfaces here as a missing file and the person doing the move needs to
// be told what to fix rather than handed a bare "no such file".
func syncAICertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	if *updateAICert {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatalf("rewriting %s: %v", path, err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		absolute, resolveErr := filepath.Abs(path)
		if resolveErr != nil {
			absolute = path
		}
		t.Fatalf("reading the committed %s (resolved to %s): %v\n"+
			"This test renders it from the corpus, the records and the census, and reaches it by "+
			"walking up out of internal/compose/aicert. If this package has moved, fix the walk-up in "+
			"aiCertPage and aiCertJSON. Otherwise regenerate with: %s", path, absolute, err, aiCertRegenerate)
	}
	if bytes.Equal(got, want) {
		return
	}
	t.Errorf("%s is stale — it no longer matches the corpus, the records and the census it is rendered from.\n"+
		"Regenerate it with: %s\nand commit the result together with the change that moved them.\n%s",
		path, aiCertRegenerate, firstAICertDifference(string(got), string(want)))
}

// firstAICertDifference reports the first line where two renderings diverge. A
// full dump of the file is unreadable in test output; the first divergent line
// is what a reader needs to see which row moved.
func firstAICertDifference(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			return "first difference at line " + strconv.Itoa(i+1) + ":\n  committed: " +
				truncateAICertLine(gotLines[i]) + "\n  rendered:  " + truncateAICertLine(wantLines[i])
		}
	}
	return fmt.Sprintf("the committed file has %d lines, the rendered one %d", len(gotLines), len(wantLines))
}

// truncateAICertLine keeps one divergent row readable in test output. A table
// row of counts is short; a prose line from the head is not, and a wrapped
// terminal hides which half actually differs.
func truncateAICertLine(line string) string {
	const limit = 160
	if len(line) <= limit {
		return line
	}
	return line[:limit] + "…"
}
