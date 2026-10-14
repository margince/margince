// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A shipped preset binds a model to every task, and the certification records
// grade each of those bindings. Where a record says the bound model is not
// certified for a task, the operator choosing that preset is owed the fact.
//
// The preset points at the certification reference rather than naming the
// tasks, which is the whole shape of this gate. The bands move: a prompt fix
// re-certifies a task, and a corpus addition degrades another. A list written
// into a comment would read as true long after it stopped being.
//
// The corpus is derived from the records, so a preset added tomorrow is covered
// the day its certification lands.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// certificationReference is what a preset must point at: the committed page
// carrying the current per-preset, per-task table.
const certificationReference = "docs/reference/ai-certification.md"

// aiCertificationPayload is the committed derivation, read for its preset rollup
// alone. The page beside it prints the same figures from this file.
const aiCertificationPayload = "../docs/reference/ai-certification.json"

func TestAPresetBindingAnUncertifiedTaskPointsAtTheRecords(t *testing.T) {
	t.Parallel()
	presets := presetsWithAnUncertifiedTask(t)
	// A census that reads nothing agrees with everything. Every shipped preset
	// binds some task the records do not certify today. So an empty answer
	// means the payload's shape moved, not that the tree got cleaner.
	if len(presets) == 0 {
		t.Fatal("no preset reads as binding an uncertified task — this census is reading a shape that is gone")
	}
	for _, preset := range presets {
		path := filepath.Join("..", "config", "presets", preset)
		body, err := os.ReadFile(path) //#nosec G304 -- a path this gate derived from the committed payload
		if err != nil {
			t.Errorf("reading %s: %v", preset, err)
			continue
		}
		if !strings.Contains(string(body), certificationReference) {
			t.Errorf("%s binds a model the records do not certify for some task, and points nowhere: "+
				"name %s in a comment, so an operator choosing this preset can read which tasks those are",
				preset, certificationReference)
		}
	}
}

// The unmeasured case, held rather than argued. Ten task bindings across the
// shipped presets carry no band today. A census skipping them reads a smaller
// tree while reporting PASS.
func TestTheDisclosureCensusCountsAnUnmeasuredTask(t *testing.T) {
	t.Parallel()
	if certificationBandIsCovered("") {
		t.Error("an unmeasured task reads as covered, so a preset whose only gap is unmeasured drops out of the census")
	}
	if certificationBandIsCovered("not_supported") || certificationBandIsCovered("supported_degraded") {
		t.Error("a measured weakness reads as covered")
	}
	if !certificationBandIsCovered("certified") {
		t.Error("a certified task reads as owing a disclosure")
	}
}

// presetsWithAnUncertifiedTask reads the committed payload and answers the
// preset files carrying at least one task whose bound model is not certified.
func presetsWithAnUncertifiedTask(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(aiCertificationPayload)
	if err != nil {
		t.Fatalf("reading the certification payload: %v", err)
	}
	var payload struct {
		Presets []struct {
			File  string `json:"file"`
			Tasks []struct {
				Task string `json:"task"`
				Band string `json:"band"`
			} `json:"tasks"`
		} `json:"presets"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parsing the certification payload: %v", err)
	}
	if len(payload.Presets) == 0 {
		t.Fatalf("%s names no presets — this gate is reading a shape that is gone", aiCertificationPayload)
	}
	var out []string
	for _, preset := range payload.Presets {
		if len(preset.Tasks) == 0 {
			t.Errorf("%s lists no tasks, so this gate cannot tell what it binds", preset.File)
			continue
		}
		for _, task := range preset.Tasks {
			// Anything other than certified. A degraded band is a measured
			// weakness. An empty one is a task no record has measured on this
			// binding, which the exporter spells by omitting the band. The
			// operator is owed both, and the page says which a task is in.
			//
			// Excluding the empty band is how this census would fail short.
			// A preset whose only uncertified task is unmeasured would drop
			// out, and stop being checked at all.
			if !certificationBandIsCovered(task.Band) {
				out = append(out, preset.File)
				break
			}
		}
	}
	return out
}

// certificationBandIsCovered answers whether a band needs no disclosure, which
// only a certified one does. The census and its own case read this, so the two
// answer an empty band the same way.
func certificationBandIsCovered(band string) bool { return band == "certified" }
