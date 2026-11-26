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
		out, problems, code := answer(*newlyStaleAgainst, *requireCurrent, *totalsOnly, *presetDir, readiness, stamps, perScenario, records, corpus, census)
		emit(out)
		for _, line := range problems {
			fmt.Fprintln(os.Stderr, line)
		}
		os.Exit(code)
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

// answer is what one of the CI flags asked: the text for stdout, the lines for
// stderr and the exit code. It is 0, except for -require-current naming a rung
// that is not current.
func answer(base, preset string, onlyTotals bool, presetDir string, readiness aicert.Census, stamps map[string]string,
	perScenario map[string]map[string]string, records []aicert.Record, corpus []aicert.Scenario, census *aitasks.Registry,
) (out string, problems []string, code int) {
	rows, _ := aicert.Readiness(readiness, stamps, perScenario, records)
	switch {
	case onlyTotals:
		return totals(rows) + "\n", nil, 0
	case base != "":
		comment, err := newlyStaleFromFile(base, rows)
		if err != nil {
			return "", []string{"reportcmd: " + err.Error()}, 1
		}
		return comment, nil, 0
	default:
		reports, err := presetReports(context.Background(), presetDir, corpus, census, records)
		if err != nil {
			return "", []string{"reportcmd: " + err.Error()}, 1
		}
		problems, code = requireCurrent(reports, preset)
		return "", problems, code
	}
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
