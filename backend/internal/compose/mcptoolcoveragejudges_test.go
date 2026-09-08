// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The judge half of docs/reference/mcp-tool-coverage.md: who decided the pass
// rates that page reports, and how accurate that decider is.
//
// It is a separate FILE and the same PAGE. A reader needs the pass rate and the
// accuracy of whoever produced it together — a number without its error bar
// invites trust nobody measured — but the two are different subjects, and the
// coverage file was over its size ceiling holding both.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The judge eval records: one per model that has been put on trial as the
// lane's judge. They sit beside the lane's own verdicts because they answer the
// other half of the same question — the verdicts say how a model DROVE the
// tools, these say how well a model JUDGED whether the driving worked, and a
// page reporting one without the other invites a reader to trust a pass rate
// without knowing who decided it was a pass.
const judgeEvalRecordDir = "aicert/records/judge_eval"

// A SUITE CASE THAT NO JUDGE MODEL CAN PASS, and therefore no model's failure.
//
// The case pins E2E_LLM_JUDGE_MODEL to a name no corpus was recorded under and
// asserts the replay is refused. An eval run drives the suite with a LIVE judge,
// where there is no recorded verdict to mismatch, so the case fails identically
// for every model under test. Counting it would charge all three for a property
// of how they were measured.
//
// Named rather than filtered by a pattern: an exemption that matched broadly
// would silently start excusing real failures, which is the one direction a
// census must not fail in.
var judgeEvalHarnessArtifacts = gatekit.Waive(map[string]string{
	"judge/a verdict from another model is a stop": "the case pins a model with no recorded corpus and asserts the replay is refused; an eval drives the suite live, where there is no replay to mismatch, so it fails for every model alike and measures the trial rather than the judge",
})

type judgeEvalRecord struct {
	Model                string   `json:"model"`
	GroundTruth          string   `json:"ground_truth"`
	SuiteExit            int      `json:"suite_exit"`
	CasesPassed          int      `json:"cases_passed"`
	CasesFailed          int      `json:"cases_failed"`
	JudgedFixturesPassed int      `json:"judged_fixtures_passed"`
	JudgedFixturesFailed int      `json:"judged_fixtures_failed"`
	Failures             []string `json:"failures"`
}

type judgeEvalRow struct {
	Model string `json:"model"`
	// Scored is what this model was actually charged with: every suite case
	// except the artifacts above, which no model can pass.
	Scored int `json:"scored"`
	Passed int `json:"passed"`
	// Failed is the model's own errors. Artifacts are reported separately so a
	// reader can see the arithmetic rather than trust a filtered total.
	Failed    int      `json:"failed"`
	Artifacts int      `json:"harness_artifacts"`
	Accuracy  string   `json:"accuracy"`
	Errors    []string `json:"errors"`
}

// readJudgeEvals folds each committed judge-trial record into a scored row.
//
// A missing directory is not an error, for the reason the verdict reader gives:
// it means nobody has put a judge on trial on this checkout, which the page says
// outright rather than publishing an empty comparison as a finished one.
func readJudgeEvals(t *testing.T, dir string) ([]judgeEvalRow, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []judgeEvalRow
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var record judgeEvalRecord
		if decodeErr := json.Unmarshal(body, &record); decodeErr != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), decodeErr)
		}
		row := judgeEvalRow{Model: record.Model, Errors: []string{}}
		for _, failure := range record.Failures {
			if judgeEvalHarnessArtifacts.Waived(t, failure) {
				row.Artifacts++
				continue
			}
			row.Errors = append(row.Errors, failure)
		}
		row.Failed = len(row.Errors)
		// The denominator excludes the artifacts, because a case no model can
		// pass measures nothing about any of them.
		row.Scored = record.CasesPassed + record.CasesFailed - row.Artifacts
		row.Passed = row.Scored - row.Failed
		if row.Scored > 0 {
			row.Accuracy = fmt.Sprintf("%.1f%%", 100*float64(row.Passed)/float64(row.Scored))
		}
		sort.Strings(row.Errors)
		out = append(out, row)
	}
	// Most accurate first, then by name, so the page does not reorder itself
	// between runs over a directory listing.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Failed != out[j].Failed {
			return out[i].Failed < out[j].Failed
		}
		return out[i].Model < out[j].Model
	})
	return out, nil
}

// writeCoverageJudges is who decided the pass rates above, and how accurate
// they are. It sits after the results because that is the order a reader needs
// them in: the number first, then how much to trust it.
func writeCoverageJudges(p *strings.Builder, r mcpToolCoverage) {
	p.WriteString("## Who judged it, and how well\n\n")
	if len(r.Judges) == 0 {
		p.WriteString("_No judge has been put on trial._\n\n")
		return
	}
	p.WriteString("The semantic half of each criterion is decided by a model rather than a regex, " +
		"so the judge is part of the apparatus and its accuracy belongs on the same page as the " +
		"results it produced. A judge wrong in the quiet direction — passing an answer the " +
		"criterion fails — turns a missed defect into a green run.\n\n")
	p.WriteString("Scored against **human-authored fixtures** under `e2e/llm/testdata/<case>/`, " +
		"labelled by `scripts/test-e2e-llm-check.sh`. The recorded verdicts under " +
		"`e2e/llm/testdata/judge/` are deliberately not the reference: they were written by one " +
		"model, so scoring against them measures resemblance to that model and hands it a free " +
		"hundred per cent.\n\n")
	p.WriteString("| Judge | Accuracy | Scored | Passed | Its own errors |\n|---|---:|---:|---:|---:|\n")
	for _, judge := range r.Judges {
		fmt.Fprintf(p, "| `%s` | %s | %d | %d | %d |\n",
			judge.Model, judge.Accuracy, judge.Scored, judge.Passed, judge.Failed)
	}
	p.WriteString("\n")
	for _, judge := range r.Judges {
		if len(judge.Errors) == 0 {
			continue
		}
		fmt.Fprintf(p, "> `%s` misread: %s\n\n", judge.Model, strings.Join(judge.Errors, ", "))
	}
	if len(r.Excluded) > 0 {
		p.WriteString("Excluded from every judge's score, because it fails for all of them alike " +
			"and measures how the trial is driven rather than the model:\n\n")
		for _, name := range sortedKeys(r.Excluded) {
			p.WriteString("- `" + name + "` — " + r.Excluded[name] + "\n")
		}
		p.WriteString("\n")
	}
}
