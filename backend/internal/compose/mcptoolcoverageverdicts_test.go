// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The committed verdicts of the use-case lane, as the coverage page reads them:
// how each was measured, and whether it still describes the product.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/compose/mcpe2e"
)

// verdictKey is what a committed verdict is filed under. The model is half of
// it: a pass rate belongs to the model that produced it, and two models that
// ran the same case have two answers, not one.
type verdictKey struct {
	model    string
	scenario string
}

type e2eVerdict struct {
	Scenario string `json:"scenario"`
	Passed   int    `json:"passed"`
	Runs     int    `json:"runs"`
	PassAt   int    `json:"pass_at"`
	Driver   string `json:"driver"`
	Search   string `json:"search"`
	// SelfJudged says a judge of the candidate's own family decided its
	// judged criteria; Stamp is what the verdict measured, nil before stamps.
	SelfJudged bool                  `json:"self_judged"`
	Stamp      *mcpe2e.RecordedStamp `json:"stamp"`
	// Model is the folder the verdict was filed under, not a field in it: the
	// lane names the directory for the model it pinned.
	Model string `json:"-"`
}

// readE2ELLMVerdicts reads the committed verdicts. A missing directory is not
// an error: it means the paid lane has not been run on this checkout, which the
// page says outright.
func readE2ELLMVerdicts(dir string) (map[verdictKey]e2eVerdict, error) {
	out := map[verdictKey]e2eVerdict{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, model := range entries {
		if !model.IsDir() {
			continue
		}
		searches := map[string]bool{}
		files, readErr := os.ReadDir(filepath.Join(dir, model.Name()))
		if readErr != nil {
			return nil, readErr
		}
		for _, entry := range files {
			if entry.IsDir() {
				return nil, fmt.Errorf("%s/%s: a verdict folder may not nest — the model id carried a '/'; "+
					"file it under E2E_LLM_FOLDER", model.Name(), entry.Name())
			}
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			body, fileErr := os.ReadFile(filepath.Join(dir, model.Name(), entry.Name()))
			if fileErr != nil {
				return nil, fileErr
			}
			var v e2eVerdict
			if decodeErr := json.Unmarshal(body, &v); decodeErr != nil {
				return nil, fmt.Errorf("%s/%s: %w", model.Name(), entry.Name(), decodeErr)
			}
			v.Model = model.Name()
			// Keyed by MODEL AND SCENARIO. Keying on the scenario alone let the
			// last model directory read win, so a page could show one model's
			// pass rate beside another's on the next row and say nothing about
			// it. Today one model has run; the bug would appear silently on the
			// day a second one does, which is the worst time to find it.
			out[verdictKey{model: model.Name(), scenario: v.Scenario}] = v
			searches[v.Search] = true
		}
		// One column is one setup: a word-overlap verdict beside ones made by
		// meaning would publish two experiments under one model's name.
		if searches["lexical"] && len(searches) > 1 {
			return nil, fmt.Errorf("%s mixes lexical verdicts with others; a lexical run files under "+
				"E2E_LLM_VERDICTS, never in the records tree", model.Name())
		}
	}
	return out, nil
}

// runsFor collects each model's result on one scenario, ordered by model name.
//
// It replaced a chooser that returned ONE verdict per scenario, picking the
// lowest model name. That was correct while one model had run and silently
// wrong the moment a second did: the page would have shown one model's pass
// rate under a heading that named the case, not the model.
//
// A verdict whose stamp no longer matches the scenario's current one carries
// why, so the page says it waits for a re-check rather than reading as current.
func runsFor(verdicts map[verdictKey]e2eVerdict, scenario string, current map[string]mcpe2e.Stamp) []caseModelRun {
	var out []caseModelRun
	for key, v := range verdicts {
		if key.scenario != scenario {
			continue
		}
		out = append(out, caseModelRun{
			Model:  v.Model,
			Runs:   v.Runs,
			Passed: v.Passed,
			PassAt: v.PassAt,
			Held:   v.PassAt > 0 && v.Passed >= v.PassAt,
			Driver: v.Driver,
			Search: v.Search,

			SelfJudged: v.SelfJudged,
			Recheck:    mcpe2e.Recheck(v.Stamp, current[scenario], true),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

// currentStamps is what every scenario would be stamped with on this tree, read
// only when a verdict needs comparing: a planted scenario folder has none of
// the committed files a stamp is made from.
func currentStamps(scenarioDir string, verdicts map[verdictKey]e2eVerdict) (map[string]mcpe2e.Stamp, error) {
	if len(verdicts) == 0 {
		return map[string]mcpe2e.Stamp{}, nil
	}
	return mcpe2e.CurrentStamps(filepath.Join(scenarioDir, "..", "..", ".."))
}
