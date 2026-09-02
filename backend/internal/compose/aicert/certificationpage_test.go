// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// docs/reference/ai-certification.md — what the certification lane covers, as a
// page rather than a terminal table.
//
// It is rendered from the same three trees `make e2e-ai-report` reads: the
// census of invocation sites this build ships, the scenario corpus, and the
// committed records. The judgement itself — current, partial, stale, absent,
// and why — is aicert.Readiness's, called here exactly as the report command
// calls it, so the page and the command can never disagree about what a band
// still claims.
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

// The three trees this page is rendered from, and the page itself. The trees
// are named relative to this package, the page by walking up out of the Go
// module — the same reach mcp-info.md's renderer makes.
const (
	aiCertCorpusDir  = "corpus"
	aiCertRecordsDir = "records"
)

var aiCertPage = filepath.Join("..", "..", "..", "..", "docs", "reference", "ai-certification.md")

var updateAICert = flag.Bool("update-ai-cert", false,
	"rewrite docs/reference/ai-certification.md from the corpus, the records and the invocation-site census")

// aiCertRegenerate is the command a reader is sent to run — read by the drift
// failure and printed into the page's own opening line, so both quote whatever
// this constant says.
const aiCertRegenerate = "cd backend && go test ./internal/compose/aicert/ -run TestAICertificationPage -update-ai-cert"

// corpusLinkPrefix turns a scenario's loader path into a link a reader of
// docs/reference/ can follow. The corpus is read at a path relative to this
// package, and the page sits two directories below the repository root.
const corpusLinkPrefix = "../../backend/internal/compose/aicert/"

// TestAICertificationPage renders the page and holds the committed copy to it.
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
		aicert.Census{Sites: census.All(), Scopes: census.Scopes()}, stamps, perScenario, records)

	page := renderAICertPage(rows, unclaimed, corpus, records)
	assertAICertPageCoversEverything(t, string(page), rows, corpus)
	syncAICertPage(t, page)
}

// assertAICertPageCoversEverything is the guard the drift check cannot be: a
// renderer that quietly dropped a site, a scenario or a stale row would still
// match a committed page rendered by the same broken renderer, and the page
// would read as full coverage of whatever happened to be emitted.
//
// So the census is checked against the trees rather than against itself: every
// shipped site, every scenario with the link that reaches its file, and a
// stated reason on every stale row.
func assertAICertPageCoversEverything(t *testing.T, page string, rows []aicert.ReadinessRow, corpus []aicert.Scenario) {
	t.Helper()
	for _, row := range rows {
		if !strings.Contains(page, "#### `"+row.SiteKey()+"`") {
			t.Errorf("the page has no section for shipped site %s", row.SiteKey())
		}
		if row.Status() != aicert.StatusStale {
			continue
		}
		if reason := row.Standing.Reason(); reason == "" || !strings.Contains(page, reason) {
			t.Errorf("the stale row for %s on %s carries no reason on the page", row.SiteKey(), row.Binding())
		}
	}
	for _, sc := range corpus {
		if !strings.Contains(page, "`"+sc.Name+"`") {
			t.Errorf("scenario %s is in the corpus but not on the page", sc.Name)
		}
		link := corpusLink(sc)
		if !strings.Contains(page, "("+link+")") {
			t.Errorf("scenario %s is listed without a link to %s", sc.Name, link)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "..", "..", strings.TrimPrefix(link, "../../"))); err != nil {
			t.Errorf("scenario %s links to %s, which does not resolve from docs/reference/: %v", sc.Name, link, err)
		}
	}
}

// renderAICertPage builds the whole page. Each section is its own writer, and
// each takes only what it renders, so a section can be read without holding the
// rest of the page in mind.
func renderAICertPage(rows []aicert.ReadinessRow, unclaimed []aicert.Record, corpus []aicert.Scenario, records []aicert.Record) []byte {
	var page strings.Builder
	writeAICertHead(&page)
	writeAICertTotals(&page, rows, corpus, records)
	writeAICertBindings(&page, rows)
	writeAICertStale(&page, rows)
	writeAICertSites(&page, rows, corpus)
	writeAICertUnclaimed(&page, unclaimed)
	return []byte(page.String())
}

func writeAICertHead(page *strings.Builder) {
	page.WriteString("# AI certification\n\n")
	page.WriteString("<!-- Generated from the invocation-site census, the scenario corpus and the committed records; do not edit by hand. -->\n\n")
	page.WriteString("Generated by `" + aiCertRegenerate + "`; do not edit by hand. It reads the same\n")
	page.WriteString("three trees `make e2e-ai-report` reads — the sites this build registers, the\n")
	page.WriteString("scenarios under [`backend/internal/compose/aicert/corpus/`](" + corpusLinkPrefix + "corpus/README.md),\n")
	page.WriteString("and the records under [`backend/internal/compose/aicert/records/`](" + corpusLinkPrefix + "records/README.md)\n")
	page.WriteString("— and renders them for a reader rather than a terminal.\n\n")
	page.WriteString("**A band is a claim about one deployment, not about the product.** Every record\n")
	page.WriteString("names a (provider, model, env) binding, and green-lights no other: the same\n")
	page.WriteString("model reached through a broker instead of an EU-hosted endpoint is a different\n")
	page.WriteString("row and needs its own run. Nothing here is a merge gate — the certification\n")
	page.WriteString("lane is paid, manual and BYOK-gated, so a prompt edit turns a record `stale`\n")
	page.WriteString("rather than failing a build.\n\n")
	page.WriteString("**RUNS/PASSED is how often a site did what its scenarios asked.** The four\n")
	page.WriteString("outcome counts beside it are what the site's own validator REPORTED, never a\n")
	page.WriteString("pass/fail column: a reply can be `accepted` and still fail the run, when the\n")
	page.WriteString("scenario asked for an abstention.\n\n")
	page.WriteString("How to add a case: [write-a-certification-case.md](../how-to/write-a-certification-case.md).\n")
	page.WriteString("How to certify a model: [certify-an-ai-model.md](../how-to/certify-an-ai-model.md).\n\n")
	page.WriteString("## The four states\n\n")
	page.WriteString("| State | What it claims |\n|---|---|\n")
	page.WriteString("| `current` | Every scenario this record measured still matches what this build sends, and there are none it has not seen. |\n")
	page.WriteString("| `partial` | Right about every case it measured; the corpus has since grown cases it never saw. The `Scenarios` column says how many of each. |\n")
	page.WriteString("| `stale` | A scenario it measured — or the prompt this build now builds from one — has changed since. The band is no longer a claim about what ships. |\n")
	page.WriteString("| `absent` | Never certified on any binding. The honest state, not a failure: the columns are dashes because no run has measured them. |\n\n")
}

// writeAICertTotals opens with the numbers a reader wants before any table:
// how much of what ships is certified at all, and how much corpus and how many
// records are behind that answer.
func writeAICertTotals(page *strings.Builder, rows []aicert.ReadinessRow, corpus []aicert.Scenario, records []aicert.Record) {
	best := bestStatusPerSite(rows)
	counted := map[string]int{}
	for _, status := range best {
		counted[status]++
	}
	bindings := map[string]bool{}
	for _, rec := range records {
		bindings[bindingKey(rec)] = true
	}
	page.WriteString("## Readiness\n\n| | |\n|---|---:|\n")
	fmt.Fprintf(page, "| Shipped invocation sites | %d |\n", len(best))
	fmt.Fprintf(page, "| … best state `current` | %d |\n", counted[aicert.StatusCurrent])
	fmt.Fprintf(page, "| … best state `partial` | %d |\n", counted[aicert.StatusPartial])
	fmt.Fprintf(page, "| … best state `stale` | %d |\n", counted[aicert.StatusStale])
	fmt.Fprintf(page, "| … `absent` on every binding | %d |\n", counted[aicert.StatusAbsent])
	fmt.Fprintf(page, "| Scenarios in the corpus | %d |\n", len(corpus))
	fmt.Fprintf(page, "| Committed records | %d |\n", len(records))
	fmt.Fprintf(page, "| Bindings measured | %d |\n\n", len(bindings))
	page.WriteString("A site's *best* state is the strongest any of its bindings reached. A site\n")
	page.WriteString("`current` on one model and `stale` on three is counted once, as `current` —\n")
	page.WriteString("the per-binding truth is in the tables below.\n\n")
}

// bestStatusPerSite folds a site's rows to the strongest state any of them
// reached, because the question "is this site certified" is asked of the site
// and answered by whichever binding answers it best.
func bestStatusPerSite(rows []aicert.ReadinessRow) map[string]string {
	rank := map[string]int{
		aicert.StatusAbsent: 0, aicert.StatusStale: 1, aicert.StatusPartial: 2, aicert.StatusCurrent: 3,
	}
	best := map[string]string{}
	for _, row := range rows {
		key, status := row.SiteKey(), row.Status()
		if held, seen := best[key]; !seen || rank[status] > rank[held] {
			best[key] = status
		}
	}
	return best
}

// bindingKey is the (provider, model, env) a record was measured on — the whole
// of what its band green-lights.
func bindingKey(rec aicert.Record) string {
	return rec.Provider + "\x00" + rec.ServedModel + "\x00" + rec.EnvClass
}

// bindingTotals is one (provider, model, env) folded over every site row it
// produced: what it was measured on, how much of the shipped surface it
// touched, and how that surface stands today.
type bindingTotals struct {
	provider, model, env string
	sites                map[string]bool
	byStatus             map[string]int
	byVerdict            map[string]int
	runs, passed         int
}

// writeAICertBindings is the certification table per provider and model — one
// line per deployment somebody has actually paid to measure.
func writeAICertBindings(page *strings.Builder, rows []aicert.ReadinessRow) {
	totals := foldBindings(rows)
	page.WriteString("## Certification by provider and model\n\n")
	page.WriteString("One line per (provider, model, env) binding, folded over every site it\n")
	page.WriteString("measured. SITES is how many shipped invocation sites this binding has ever\n")
	page.WriteString("been run against — not how many exist — and the three state columns split\n")
	page.WriteString("those sites by whether the measurement still describes what this build sends.\n")
	page.WriteString("The band columns count the same sites by the verdict each reached.\n\n")
	page.WriteString("| Provider | Model | Env | Sites | `current` | `partial` | `stale` | Runs | Passed | Reliability | `certified` | `supported_degraded` | `not_supported` |\n")
	page.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, b := range totals {
		fmt.Fprintf(page, "| `%s` | `%s` | `%s` | %d | %d | %d | %d | %d | %d | %s | %d | %d | %d |\n",
			b.provider, b.model, b.env, len(b.sites),
			b.byStatus[aicert.StatusCurrent], b.byStatus[aicert.StatusPartial], b.byStatus[aicert.StatusStale],
			b.runs, b.passed, reliability(b.passed, b.runs),
			b.byVerdict[aicert.VerdictCertified], b.byVerdict[aicert.VerdictSupportedDegraded],
			b.byVerdict[aicert.VerdictNotSupported])
	}
	page.WriteString("\n")
}

// foldBindings groups the certified rows by binding, ordered so the page is a
// function of the trees and not of a map walk.
func foldBindings(rows []aicert.ReadinessRow) []bindingTotals {
	byKey := map[string]*bindingTotals{}
	for _, row := range rows {
		if !row.Certified {
			continue
		}
		key := bindingKey(row.Record)
		b := byKey[key]
		if b == nil {
			b = &bindingTotals{
				provider: row.Record.Provider, model: row.Record.ServedModel, env: row.Record.EnvClass,
				sites: map[string]bool{}, byStatus: map[string]int{}, byVerdict: map[string]int{},
			}
			byKey[key] = b
		}
		b.sites[row.SiteKey()] = true
		b.byStatus[row.Status()]++
		b.byVerdict[row.Tally.Verdict]++
		b.runs += row.Tally.Runs
		b.passed += row.Tally.Passed
	}
	out := make([]bindingTotals, 0, len(byKey))
	for _, b := range byKey {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].provider != out[j].provider {
			return out[i].provider < out[j].provider
		}
		if out[i].model != out[j].model {
			return out[i].model < out[j].model
		}
		return out[i].env < out[j].env
	})
	return out
}

// writeAICertStale is the question this page exists to answer for a release
// decision: what is no longer a claim, and what it would cost to make it one
// again. A count alone cannot answer the second half.
func writeAICertStale(page *strings.Builder, rows []aicert.ReadinessRow) {
	page.WriteString("## Stale records, and why\n\n")
	var stale []aicert.ReadinessRow
	for _, row := range rows {
		if row.Status() == aicert.StatusStale {
			stale = append(stale, row)
		}
	}
	if len(stale) == 0 {
		page.WriteString("None: every committed record still describes the scenarios, and the prompts,\n")
		page.WriteString("that this build sends.\n\n")
		return
	}
	page.WriteString("A record goes stale when a scenario it MEASURED changes, or when the prompt\n")
	page.WriteString("this build now builds from that scenario does — the certification stamp covers\n")
	page.WriteString("both, so a prompt edit in the product invalidates a record that never mentioned\n")
	page.WriteString("it. A scenario the record never saw does not make it stale; that is `partial`.\n\n")
	page.WriteString("A record that *predates per-scenario stamps* carries one stamp for the whole\n")
	page.WriteString("task and nothing finer, so it can only say that SOMETHING it measured has\n")
	page.WriteString("moved — never which case. Re-certifying it costs the whole task; a record with\n")
	page.WriteString("per-scenario stamps names the cases, and costs only those.\n\n")
	page.WriteString("Re-certify with `make e2e-ai TASK=<task> MODEL=<provider:model>` (paid: real\n")
	page.WriteString("model, real network).\n\n")
	page.WriteString("| Site | Binding | Why it is stale |\n|---|---|---|\n")
	for _, row := range stale {
		fmt.Fprintf(page, "| `%s` | `%s` | %s |\n", row.SiteKey(), row.Binding(), row.Standing.Reason())
	}
	page.WriteString("\n")
}

// writeAICertSites is the body: every shipped site, the scenarios it is scored
// against today, and every record measured on it. The scenario list is the
// half a reader cannot get from the terminal report at all — a band says how
// well a model did, and only the corpus says at WHAT.
func writeAICertSites(page *strings.Builder, rows []aicert.ReadinessRow, corpus []aicert.Scenario) {
	page.WriteString("## Sites, their scenarios and their records\n\n")
	page.WriteString("One section per task, one subsection per invocation site that task ships. A\n")
	page.WriteString("task can ship several — `cold_start` ships four — and the site is the unit\n")
	page.WriteString("here, because it is what the contract ships and what a scenario names.\n\n")
	page.WriteString("In each record table, `Scenarios` is how many of the site's CURRENT scenarios\n")
	page.WriteString("that record still describes, over how many the site ships today. A record\n")
	page.WriteString("written before per-scenario stamps existed can only be judged whole, and shows\n")
	page.WriteString("`-`.\n\n")
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
	scenarios := scenariosBySite(corpus)
	for _, task := range tasks {
		fmt.Fprintf(page, "### `%s`\n\n", task)
		writeAICertTaskContext(page, byTask[task])
		for _, siteKey := range siteKeysOf(byTask[task]) {
			writeAICertSite(page, siteKey, byTask[task], scenarios[siteKey])
		}
	}
}

// writeAICertTaskContext states, once per task, what its contract has
// production prepend that no certification run was served. The cert lane has no
// database to assemble company context from, and for most tasks that costs
// nothing — their contract prepends none. Where it does, the certified prompt
// is short exactly what production always supplies, and no column can say so.
func writeAICertTaskContext(page *strings.Builder, rows []aicert.ReadinessRow) {
	for _, row := range rows {
		if !row.Certified || row.Record.ContextApplied || len(row.Record.ContextScopes) == 0 {
			continue
		}
		fmt.Fprintf(page, "Certified without the company context production prepends (`%s`): "+
			"this lane runs with no database to assemble it from.\n\n",
			strings.Join(row.Record.ContextScopes, "`, `"))
		return
	}
}

// siteKeysOf lists a task's sites once each, in the order the census yielded
// them, so the page's shape follows the census rather than a map.
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

// scenariosBySite indexes the corpus the way the page reads it, each site's
// cases ordered by name so the page is stable across regenerations.
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

func writeAICertSite(page *strings.Builder, siteKey string, rows []aicert.ReadinessRow, scenarios []aicert.Scenario) {
	var mine []aicert.ReadinessRow
	for _, row := range rows {
		if row.SiteKey() == siteKey {
			mine = append(mine, row)
		}
	}
	fmt.Fprintf(page, "#### `%s`\n\n", siteKey)
	fmt.Fprintf(page, "Scope a run of it can claim: `%s`.\n\n", mine[0].Scope)
	writeAICertScenarios(page, scenarios)
	writeAICertSiteRecords(page, mine)
}

// writeAICertScenarios lists what this site is actually scored against, with a
// link to each case. A scenario's file name and its `name:` are different
// strings — nothing else in the tree maps one to the other — so the link is the
// only way a reader gets from this page to the case.
func writeAICertScenarios(page *strings.Builder, scenarios []aicert.Scenario) {
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
			sc.Name, sc.Expect.Outcome, filepath.Base(sc.Path), corpusLink(sc))
	}
	page.WriteString("\n")
}

// corpusLink turns a scenario's loader path into one a reader of
// docs/reference/ can follow.
func corpusLink(sc aicert.Scenario) string {
	return corpusLinkPrefix + filepath.ToSlash(sc.Path)
}

func writeAICertSiteRecords(page *strings.Builder, rows []aicert.ReadinessRow) {
	if len(rows) == 1 && !rows[0].Certified {
		page.WriteString("No record: this site has never been certified on any binding.\n\n")
		return
	}
	fmt.Fprintf(page, "Records (%d):\n\n", len(rows))
	page.WriteString("| Binding | State | Scenarios | Band | Runs | Passed | Reliability | `accepted` | `wrong_answer` | `invalid` | `abstained` |\n")
	page.WriteString("|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range rows {
		t := row.Tally
		fmt.Fprintf(page, "| `%s` | `%s` | %s | `%s` | %d | %d | %s | %d | %d | %d | %d |\n",
			row.Binding(), row.Status(), row.Coverage(), t.Verdict,
			t.Runs, t.Passed, reliability(t.Passed, t.Runs),
			t.ReportedAccepted, t.ReportedWrongAnswer, t.ReportedInvalid, t.ReportedAbstained)
	}
	page.WriteString("\n")
}

// reliability renders a pass rate, or a dash when there is nothing to divide.
// Zero runs is not a reliability of zero.
func reliability(passed, runs int) string {
	if runs == 0 {
		return aicert.Unmeasured
	}
	return strconv.FormatFloat(float64(passed)/float64(runs), 'f', 2, 64)
}

// writeAICertUnclaimed names each committed record no row above could carry. A
// record nobody enumerates reads as no record at all, which is the failure this
// whole page exists to remove.
func writeAICertUnclaimed(page *strings.Builder, unclaimed []aicert.Record) {
	if len(unclaimed) == 0 {
		return
	}
	page.WriteString("## Records no shipped site claims\n\n")
	page.WriteString("| Record | Why no row carries it |\n|---|---|\n")
	for _, rec := range unclaimed {
		fmt.Fprintf(page, "| `%s/%s_%s_%s` | %s |\n", rec.Task, rec.Provider, rec.ServedModel, rec.EnvClass,
			"it names no scenario of any site its task ships, so no row above can attribute it")
	}
	page.WriteString("\n")
}

// syncAICertPage compares the rendered page against its committed copy, or
// rewrites it under -update-ai-cert.
//
// The failure names the resolved path and the regeneration command: this
// package reaches the page by walking up out of the Go module, so a package
// move surfaces here as a missing file and the person doing the move needs to
// be told what to fix rather than handed a bare "no such file".
func syncAICertPage(t *testing.T, want []byte) {
	t.Helper()
	if *updateAICert {
		if err := os.WriteFile(aiCertPage, want, 0o644); err != nil {
			t.Fatalf("rewriting %s: %v", aiCertPage, err)
		}
		return
	}
	got, err := os.ReadFile(aiCertPage)
	if err != nil {
		absolute, resolveErr := filepath.Abs(aiCertPage)
		if resolveErr != nil {
			absolute = aiCertPage
		}
		t.Fatalf("reading the committed page %s (resolved to %s): %v\n"+
			"This test renders it from the corpus, the records and the census, and reaches it by "+
			"walking up out of internal/compose/aicert. If this package has moved, fix the walk-up in "+
			"aiCertPage. Otherwise regenerate with: %s", aiCertPage, absolute, err, aiCertRegenerate)
	}
	if bytes.Equal(got, want) {
		return
	}
	t.Errorf("%s is stale — it no longer matches the corpus, the records and the census it is rendered from.\n"+
		"Regenerate it with: %s\nand commit the result together with the change that moved them.\n%s",
		aiCertPage, aiCertRegenerate, firstAICertDifference(string(got), string(want)))
}

// firstAICertDifference reports the first line where two renderings diverge. A
// full dump of the page is unreadable in test output; the first divergent line
// is what a reader needs to see which row moved.
func firstAICertDifference(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			return "first difference at line " + strconv.Itoa(i+1) + ":\n  committed: " +
				truncateAICertLine(gotLines[i]) + "\n  rendered:  " + truncateAICertLine(wantLines[i])
		}
	}
	return fmt.Sprintf("the committed page has %d lines, the rendered one %d", len(gotLines), len(wantLines))
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
