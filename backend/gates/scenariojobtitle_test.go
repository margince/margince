// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Each assistant job's `title:` line is its name on the result pages a client
// reads, so it says the job in a user's words.
const scenarioDir = "../e2e/llm/scenarios"

// jobTitleMaxRunes keeps a job's name to one short cell of the result table.
const jobTitleMaxRunes = 48

// jobTitleCodeWord marks a builder's word: a tool name, inline code, or a
// scenario number.
var jobTitleCodeWord = regexp.MustCompile("[_`]|(?i)\\bcase ?\\d+")

var scenarioTitleLine = regexp.MustCompile(`(?m)^title:[ \t]*(.*?)[ \t]*$`)

func TestEveryScenarioNamesItsJobInPlainWords(t *testing.T) {
	t.Parallel()
	entries, err := gatekit.ReadDir(scenarioDir)
	if err != nil {
		t.Fatalf("reading %s: %v", scenarioDir, err)
	}
	read := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		body, err := gatekit.ReadFile(filepath.Join(scenarioDir, entry.Name())) //#nosec G304 -- a file this gate listed
		if err != nil {
			t.Fatal(err)
		}
		read++
		title := ""
		if m := scenarioTitleLine.FindSubmatch(body); m != nil {
			title = string(m[1])
		}
		if problem := jobTitleProblem(title); problem != "" {
			t.Errorf("%s: its `title:` %s", entry.Name(), problem)
		}
	}
	if read == 0 {
		t.Fatal("read no scenarios; this census would pass having read nothing")
	}
}

func TestAJobTitleWithACodeWordOrNoneFails(t *testing.T) {
	t.Parallel()
	for _, title := range []string{
		"", "Mark notices read with change_attention", "Use `read_details` here",
		"Near neighbour of case104", strings.Repeat("a long title ", 5),
	} {
		if jobTitleProblem(title) == "" {
			t.Errorf("%q passed as a plain job title", title)
		}
	}
	if problem := jobTitleProblem("Tidy up the deal room"); problem != "" {
		t.Errorf("a plain title failed: %s", problem)
	}
}

func jobTitleProblem(title string) string {
	switch {
	case title == "":
		return "is missing; name the job as a user would ask for it"
	case utf8.RuneCountInString(title) > jobTitleMaxRunes:
		return fmt.Sprintf("is longer than %d characters; say the job, not how it is checked", jobTitleMaxRunes)
	case jobTitleCodeWord.MatchString(title):
		return fmt.Sprintf("%q holds a code word; say it in a user's words", title)
	}
	return ""
}
