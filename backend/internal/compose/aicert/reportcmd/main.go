// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command reportcmd prints the AI certification readiness report — one row per
// shipped invocation site, saying whether a record covers it, whether that
// record still describes what this build sends, and on which
// (provider, model, env) it was measured. It reads three trees: the census of
// sites this build ships, the corpus those sites are scored against, and the
// committed records aicert.LoadRecords reads back
// (backend/internal/compose/aicert/records/ by default).
//
// It is a go-run-only developer tool, not a shipped process role:
// `make e2e-ai-report` invokes it directly with `go run`, so it never gets a
// cmd/<role> entry of its own. Printing the report always exits 0. The
// certification lane is paid and manual, and a report that failed a build would
// make every prompt edit wait on a paid run. Three flags answer one question for
// CI instead: -newly-stale, -totals and -require-current. Only the last fails,
// and only the release workflow runs it.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/agentloope2e/live"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func main() {
	dir := flag.String("dir", "internal/compose/aicert/records",
		"directory of certification records (relative to backend/, matching `make e2e-ai-report`'s cwd)")
	corpusDir := flag.String("corpus", "internal/compose/aicert/corpus",
		"directory of certification scenarios, read to tell a current record from a stale one")
	presetDir := flag.String("presets", "../config/presets",
		"directory of deploy presets, read to report each task's first rung and fallback per preset")
	repoRoot := flag.String("repo", "..", "the repository root (relative to backend/), read to tell a current agent loop verdict from a stale one")
	agentLoopDir := flag.String("agentloop", "internal/compose/aicert/records/agent_loop_e2e",
		"directory of the agent loop live lane's verdicts")
	newlyStaleAgainst := flag.String("newly-stale", "",
		"print the records current in this saved report of another tree and not current here, as a pull request comment")
	requireCurrent := flag.String("require-current", "",
		"exit 1 and list the rungs of this preset file (such as gemini_cloud.yaml) whose record is not current")
	totalsOnly := flag.Bool("totals", false, "print only how many records are current, partial, stale and absent")
	flag.Parse()

	// The census is what says which sites SHOULD have a record: an absent
	// record cannot name itself, and a report that enumerated only the records
	// it found would read as full coverage of whatever happened to exist.
	census, err := compose.NewTaskCensus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: building the invocation-site census: %v\n", err)
		os.Exit(1)
	}
	corpus, err := aicert.LoadCorpus(*corpusDir, census)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	records, err := aicert.LoadRecords(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	// The stamps are computed here, beside the trees they are read from: a stamp
	// drives each site's own request builder, which can fail exactly like reading
	// a malformed corpus does — and a report that could not tell a current record
	// from a stale one is not a report worth printing.
	stamps, perScenario, err := aicert.CurrentStamps(context.Background(), corpus, census)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}

	_, decisionRows, err := aicert.DecisionCertTable(corpus, census, records)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}

	contextOwed, err := aicert.ContextOwedTasks(context.Background(), corpus, census)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	readiness := aicert.Census{Sites: census.All(), Scopes: census.Scopes()}
	if *newlyStaleAgainst != "" || *requireCurrent != "" || *totalsOnly {
		os.Exit(answer(*newlyStaleAgainst, *requireCurrent, *totalsOnly, *presetDir, readiness, stamps, perScenario, records, corpus, census))
	}
	emit(renderReadiness(readiness, stamps, perScenario, records, contextOwed))
	emit(renderDecisions(decisionRows))

	reports, err := presetReports(context.Background(), *presetDir, corpus, census, records)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	emit(renderPresets(reports))

	judgeModel, err := agentloope2e.CommandJudge{Root: *repoRoot}.Model(context.Background())
	if err != nil {
		judgeModel = "" // a machine with no judge door still reports, on the pinned judge
	}
	verdicts, err := live.Lines(*repoRoot, *agentLoopDir, judgeModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	emit(renderAgentLoop(verdicts))
}

// answer prints what one of the CI flags asked and returns the exit code: 0,
// except for -require-current naming a rung that is not current.
func answer(base, preset string, onlyTotals bool, presetDir string, readiness aicert.Census, stamps map[string]string,
	perScenario map[string]map[string]string, records []aicert.Record, corpus []aicert.Scenario, census *aitasks.Registry,
) int {
	rows, _ := aicert.Readiness(readiness, stamps, perScenario, records)
	switch {
	case onlyTotals:
		emit(totals(rows) + "\n")
	case base != "":
		raw, err := os.ReadFile(base) // #nosec G304 -- the saved report the operator named
		if err != nil {
			fmt.Fprintf(os.Stderr, "reportcmd: reading the base report: %v\n", err)
			return 1
		}
		emit(renderNewlyStale(newlyStale(statusesOf(string(raw)), rows)))
	default:
		return requireCurrentPreset(preset, presetDir, corpus, census, records)
	}
	return 0
}

// requireCurrentPreset lists the rungs of one preset that are not current.
func requireCurrentPreset(file, dir string, corpus []aicert.Scenario, census *aitasks.Registry, records []aicert.Record) int {
	reports, err := presetReports(context.Background(), dir, corpus, census, records)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		return 1
	}
	for _, report := range reports {
		if report.File != file {
			continue
		}
		behind := notCurrentRungs(report)
		for _, line := range behind {
			fmt.Fprintln(os.Stderr, line)
		}
		if len(behind) > 0 {
			return 1
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "reportcmd: no preset file %q under %s\n", file, dir)
	return 1
}

// presetReports reads the rung states of the presets under dir over the committed records.
func presetReports(ctx context.Context, dir string, corpus []aicert.Scenario, census *aitasks.Registry, records []aicert.Record) ([]presetReport, error) {
	presets, err := loadPresets(dir)
	if err != nil {
		return nil, err
	}
	var reports []presetReport
	for file, routing := range presets {
		rungs, err := aicert.PresetRungs(ctx, routing, corpus, census, records)
		if err != nil {
			return nil, fmt.Errorf("preset %s: %w", file, err)
		}
		reports = append(reports, presetReport{File: file, Rungs: rungs})
	}
	return reports, nil
}

// emit writes one report section to stdout. A failed write stops the command,
// so a full disk or a closed pipe never leaves a short report behind a success
// exit.
func emit(section string) {
	if _, err := io.WriteString(os.Stdout, section); err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: writing the report: %v\n", err)
		os.Exit(1)
	}
}
