// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// The certification card names every AI job in plain language, and those names
// are the whole point of it: a reader who has to know what
// `capture_confidentiality_verdict` means did not need the card.
//
// Nothing about a task contract can produce such a name, so they are authored
// in the locale catalogue — which means a job added to ai-tasks.yaml would
// otherwise ship with no wording and reach a reader as an identifier, or vanish
// from the card entirely. This derives the required key set from the shipped-site
// census, so that failure is a red test instead.
//
// Held by: the same census the readiness report reads (compose.NewTaskCensus).
// Relative to the MODULE ROOT: gates' TestMain chdirs there, which is why this
// is one level up rather than two.
const enCatalogue = "../frontend/src/i18n/en.ts"

// certJudgeTask is the certification lane's own grader. It ships, so the census
// carries it, but it is the instrument rather than a job the product runs for a
// user — and TestTheGraderIsTheOnlyJobWithoutAName below holds that it stays
// the only exclusion.
const certJudgeTask = ai.TaskCertJudge

func catalogueKeys(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(enCatalogue)
	if err != nil {
		t.Fatalf("reading the locale catalogue: %v", err)
	}
	// Match the STATEMENT, not the line: a key whose translation wraps onto the
	// next line is one statement, and a line-wise scan would miss its
	// neighbours. Under-recognition is the one way this gate must not break —
	// it would read a smaller catalogue and report PASS.
	keyed := regexp.MustCompile(`"(aiCert\.(?:job|site)\.[a-z0-9_.]+)"\s*:`)
	found := map[string]bool{}
	for _, m := range keyed.FindAllStringSubmatch(string(raw), -1) {
		found[m[1]] = true
	}
	if len(found) == 0 {
		t.Fatalf("no aiCert.job/site keys found in %s — the pattern has stopped matching the catalogue, "+
			"which would let this gate pass while every name was missing", enCatalogue)
	}
	return found
}

// Every shipped job and every one of its invocation sites has a human name.
func TestEveryShippedJobIsNamedForAReader(t *testing.T) {
	t.Parallel()

	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the invocation-site census: %v", err)
	}
	if len(census.All()) == 0 {
		t.Fatal("the site census is empty, so every assertion below would pass vacuously")
	}
	keys := catalogueKeys(t)

	var missing []string
	for _, site := range census.All() {
		if site.Task == certJudgeTask {
			continue
		}
		for _, want := range []string{
			fmt.Sprintf("aiCert.job.%s", site.Task),
			fmt.Sprintf("aiCert.site.%s.%s", site.Task, site.Variant),
		} {
			if !keys[want] {
				missing = append(missing, want)
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d certification name(s) missing from %s — a job with no name is dropped from the card "+
			"or printed as an identifier, and both defeat what the card is for:\n  %s",
			len(missing), enCatalogue, strings.Join(dedupe(missing), "\n  "))
	}
}

// The skip list is one entry and must stay one.
//
// A gate whose exclusions can grow silently stops covering the thing it names:
// it reads a smaller tree, reports PASS, and no assertion fires. So the
// exclusion is asserted rather than merely applied — cert_judge is excluded
// BECAUSE it is the grader, and any second exclusion has to be argued here.
func TestTheGraderIsTheOnlyJobWithoutAName(t *testing.T) {
	t.Parallel()

	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the invocation-site census: %v", err)
	}
	keys := catalogueKeys(t)

	for _, site := range census.All() {
		named := keys[fmt.Sprintf("aiCert.job.%s", site.Task)]
		if site.Task == certJudgeTask {
			if named {
				t.Errorf("the grader %s carries a reader-facing name; it is the certification instrument, "+
					"not a job the product runs for a user", site.Task)
			}
			continue
		}
		if !named {
			t.Errorf("shipped job %s has no name, and only the grader may be excluded — either name it "+
				"or argue the exclusion in this test", site.Task)
		}
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
