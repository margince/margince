// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gen-perfdoc renders docs/reference/performance-budgets.md and the
// plain-language docs/reference/benchmark.md from the records the benchmark
// lane leaves behind.
//
// docs/reference/rbac-matrix.md is the model for the SHAPE — a published page a
// reader who cannot run the lane can still consult — and deliberately not the
// model for the SEMANTICS. That page is derived from the seeded policy and so
// is drift-gated: render it twice, get the same bytes. A latency is measured,
// not derived, so rendering twice never gives the same bytes and a drift gate
// would fail every run for everybody. What this holds to instead is the rule
// aicert's records already follow: a measurement says what it ran on, and a
// budget nobody measured is printed as unmeasured rather than omitted.
//
// That last part is the point. #697 was filed because we publish budgets and
// measure most of them nowhere; a page that listed only what happens to have a
// harness would hide exactly the gap it was written to close.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// publishedBudget is one row of the performance-budget table this product
// publishes. Every budget gets a row whether or not anything measures it.
type publishedBudget struct {
	id        string
	operation string
	budget    string
	// measuredBy names the make targets whose records fill this row in, and is
	// empty when nothing measures it yet; the page prints that gap as one.
	measuredBy []string
}

// The published set, in the order acceptance-standards.md lists it, then the
// budgets a bench declares beside its own cases (LOAD-1, bench-daily). Hand-kept
// against the spec on purpose: it is the CLAIM side of the page, and deriving
// it from the records would let a budget disappear from the page simply by
// nobody measuring it — which is the failure this whole page exists to prevent.
var published = []publishedBudget{
	{"PERF-1", "Record open (contact/company/deal)", "< 100 ms server", []string{"bench-record", dailyTarget}},
	{"PERF-2", "List/table view (50 rows, filtered)", "< 150 ms server", []string{dailyTarget}},
	{"PERF-3", "Search (full-text)", "< 200 ms", []string{"bench-perf"}},
	{"PERF-4", "Save/mutation", "< 150 ms server", []string{"bench-record"}},
	{"PERF-5", "AI baseline action (summary/draft)", "first token < 1.5 s", nil},
	{"PERF-6", "Cold start (single binary)", "< 2 s", nil},
	{"PERF-7", "Context-graph assembly", "< 300 ms at mid-market", []string{"bench-perf", dailyTarget}},
	{"PERF-8", "Worklist and Home, as the screen calls them", "< 1 s", []string{dailyTarget}},
	{"PERF-9", "Analytics screen", "< 300 ms", []string{dailyTarget}},
	{"PERF-10", "Search as the screen calls it", "< 1 s", []string{dailyTarget}},
	{"LOAD-1", "A cheap request while the team starts at once", "< 150 ms", []string{dailyTarget}},
	{"CAP-PARAM-1", "Capture to timeline", "60 s p95", []string{"bench-capture"}},
	{"MOBILE-AC-2", "Record open, perceived, Fast-3G", "< 300 ms perceived", []string{"bench-mobile"}},
}

// dailyTarget is the bench whose rows carry a seat and a stored verdict.
const dailyTarget = "bench-daily"

type machine struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPU       string `json:"cpu"`
	Cores     int    `json:"cores"`
	MemoryGiB int    `json:"memory_gib"`
	Toolchain string `json:"toolchain"`
	Postgres  string `json:"postgres,omitempty"`
	Network   string `json:"network,omitempty"`
	Viewport  string `json:"viewport,omitempty"`
}

type measurement struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
	P99Ms    float64 `json:"p99_ms"`
	BudgetMs float64 `json:"budget_ms"`
	Samples  int     `json:"samples"`
	Caveat   string  `json:"caveat,omitempty"`
	// The fields below mirror BudgetMeasurement in
	// internal/compose/integration/perfrecord.go; older records leave them empty.
	Seat          string  `json:"seat,omitempty"`
	Flow          string  `json:"flow,omitempty"`
	Verdict       string  `json:"verdict,omitempty"`
	KnownIssue    int     `json:"known_issue,omitempty"`
	Status5xx     int     `json:"status_5xx,omitempty"`
	Status422     int     `json:"status_422,omitempty"`
	PoolWaitMs    float64 `json:"pool_wait_ms,omitempty"`
	PoolWaitMaxMs float64 `json:"pool_wait_max_ms,omitempty"`
	Acquires      int64   `json:"acquires,omitempty"`
	PoolSize      int32   `json:"pool_size,omitempty"`
	Note          string  `json:"note,omitempty"`
}

// corpus mirrors CorpusFacts in perfrecord.go: what a seeded bench ran over.
type corpus struct {
	Scale      float64 `json:"scale"`
	Contacts   int     `json:"contacts"`
	Companies  int     `json:"companies"`
	Deals      int     `json:"deals"`
	Leads      int     `json:"leads"`
	Projects   int     `json:"projects"`
	Activities int     `json:"activities"`
	Reps       int     `json:"reps"`
	Managers   int     `json:"managers"`
}

type record struct {
	Target     string        `json:"target"`
	MeasuredOn string        `json:"measured_on"`
	Machine    machine       `json:"machine"`
	Budgets    []measurement `json:"budgets"`
	Corpus     *corpus       `json:"corpus,omitempty"`
}

const (
	recordDir = "../docs/reference/perfbench"
	pagePath  = "../docs/reference/performance-budgets.md"
	plainPath = "../docs/reference/benchmark.md"
)

func main() {
	records, err := loadRecords(recordDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-perfdoc: reading records: %v\n", err)
		os.Exit(1)
	}
	pages := []struct{ path, body string }{
		{pagePath, render(records)},
		{plainPath, renderPlain(records)},
	}
	for _, page := range pages {
		if err := os.WriteFile(page.path, []byte(page.body), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "gen-perfdoc: writing %s: %v\n", page.path, err)
			os.Exit(1)
		}
		fmt.Printf("gen-perfdoc: %s rendered from %d record(s)\n", page.path, len(records))
	}
}

// loadRecords reads every record in the directory, keyed by target. A missing
// directory is not an error: it is the state of a checkout where nobody has run
// a benchmark yet, and the page it produces — every row unmeasured — is a true
// statement about that checkout.
func loadRecords(dir string) (map[string]record, error) {
	records := map[string]record{}
	// Rooted at the record directory rather than joining names onto it. An
	// fs.FS cannot be walked out of, so a name that tried to reach outside it
	// resolves to nothing instead of being opened — which is the property
	// gosec's G304 actually asks about. Rooting the reads answers the question;
	// a waiver would only have declined to.
	fsys := os.DirFS(dir)
	entries, err := fs.ReadDir(fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		body, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		var r record
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		for _, m := range r.Budgets {
			if err := validStoredVerdict(m); err != nil {
				return nil, fmt.Errorf("%s: %w", entry.Name(), err)
			}
		}
		records[r.Target] = r
	}
	return records, nil
}
