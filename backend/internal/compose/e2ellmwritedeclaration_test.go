// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The use-case lane rebuilds the database before any run that follows a run,
// because the surface it grants is the whole server and so ANY run can write
// whatever it likes — the reset cannot key on what a scenario declares.
//
// `writes:` is therefore the case author's statement of INTENT, not the bound
// the runner enforces. It is still worth holding: a case whose declaration
// disagrees with the tools it names is a case whose author and reader disagree
// about what it does, and that is the disagreement this test refuses.
const (
	e2eLLMRunnerScript  = "../../../scripts/e2e-llm.sh"
	e2eLLMWritesReadBy  = "--field writes"
	e2eLLMWritesDeclare = "writes: true"
)

var e2eWritesDeclaration = regexp.MustCompile(`(?m)^writes:\s*(\S+)\s*$`)

// TestEveryWritingScenarioDeclaresThatItWrites holds the lane's reset decision
// to the tools the scenario itself names.
//
// The runner used to carry the answer as a glob list — `case1_*|case2_*|case3_*`
// — and that list could only fail one way. A writing case nobody added to it ran
// its second and third attempts against the world its first attempt had already
// changed: no error, no empty transcript, just a run answering a question the
// scenario is not asking. `case3_*` does not even match `case30_…`, so the list
// was one filename away from being wrong at all times.
//
// So the fact is declared where its author is, and derived here from what the
// case reaches for. A tool that is not read-only writes, and a case that may
// call one may write on any run — MAY, not MUST, because permission is enough:
// the reset has to be right for the run that took the permission.
//
// Over-declaration is deliberately allowed. A case that saves a run through a
// read-scoped tool still leaves something behind for its next attempt to trip
// over, and the cost of one extra rebuild is time; the cost of one missing
// reset is a verdict nobody can read.
func TestEveryWritingScenarioDeclaresThatItWrites(t *testing.T) {
	writers := map[string]bool{}
	for _, spec := range servedSurface(t).Specs() {
		if !spec.ReadOnly() {
			writers[spec.Name] = true
		}
	}
	// Under-recognition is the one way this test must not break: an empty
	// writer set clears every scenario and reports a lane that resets nothing
	// as correct.
	if len(writers) == 0 {
		t.Fatal("the served surface reported no writing tool, so every scenario would pass " +
			"this vacuously — the read-only derivation is what fails, not the lane")
	}

	entries, err := os.ReadDir(e2eLLMScenarioDir)
	if err != nil {
		t.Fatalf("reading the use-case lane at %s: %v", e2eLLMScenarioDir, err)
	}
	read := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(e2eLLMScenarioDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("reading %s: %v", entry.Name(), readErr)
		}
		read++
		text := string(body)
		reached := append(toolsInBlock(e2eMustCallBlock, text), toolsInBlock(e2eMayCallBlock, text)...)
		var writes []string
		for _, tool := range reached {
			if writers[tool] {
				writes = append(writes, tool)
			}
		}
		if len(writes) == 0 {
			continue
		}
		if declared := e2eWritesDeclaration.FindStringSubmatch(text); len(declared) == 2 && declared[1] == "true" {
			continue
		}
		sort.Strings(writes)
		t.Errorf("e2e/llm/scenarios/%s can write (%s) and does not declare %q, so its second and "+
			"third runs would drive the world its first run left behind",
			entry.Name(), strings.Join(writes, ", "), e2eLLMWritesDeclare)
	}
	if read == 0 {
		t.Fatalf("read no scenario from %s — a scan that finds nothing has nothing to hold",
			e2eLLMScenarioDir)
	}

	// The declaration is only worth anything while something acts on it. Without
	// this, deleting the runner's read leaves twenty-one files stating a fact
	// nobody consults and this test still green.
	runner, err := os.ReadFile(e2eLLMRunnerScript)
	if err != nil {
		t.Fatalf("reading the lane runner at %s: %v", e2eLLMRunnerScript, err)
	}
	if !strings.Contains(string(runner), e2eLLMWritesReadBy) {
		t.Errorf("%s no longer reads the scenarios' %q with %q, so the declarations this test "+
			"holds decide nothing", e2eLLMRunnerScript, e2eLLMWritesDeclare, e2eLLMWritesReadBy)
	}
}
