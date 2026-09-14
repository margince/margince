// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Code from a pull request's own branch never runs with a token GitHub has not
// downgraded.
//
// On `pull_request`, a fork's GITHUB_TOKEN is read-only whatever the workflow's
// `permissions:` block asks for. GitHub imposes that downgrade, and it is the
// control that makes running a contributor's branch safe at all. The triggers
// listed below get no such downgrade: they fire in the BASE repository's
// context, with a full-capability token and the repository's secrets, on events
// a contributor provokes by opening a pull request and waiting for a maintainer
// to review or comment on it.
//
// Combine one of them with a checkout of the pull request's head and the
// workflow runs the contributor's version of every file it executes — including
// the CI plumbing, which is not what a reviewer reads a diff for — with that
// token in the step environment. `persist-credentials: false` does not cover
// it: that keeps the token out of the git config, not out of `env:`.
//
// So a job that checks out a pull request's head, in a workflow any privileged
// trigger can start, has to say in its own condition that the branch belongs to
// this repository.
//
// WHAT THIS DOES NOT DO, deliberately: evaluate the condition. Deciding whether
// a GitHub expression is satisfiable means implementing GitHub's expression
// language against a synthetic payload, which is a second copy of the thing
// under test — the line aggregategatereach_test.go draws for the same reason.
// This asserts the comparison is PRESENT, so an unfamiliar spelling of the
// guard fails here rather than passing. For a guard that is the safe direction:
// the reviewer is sent to read a condition, not handed a green.

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// privilegedTriggers are the events that run in the base repository's context
// with a token GitHub has not downgraded for a fork.
//
// `pull_request` is deliberately absent and is the reason the list is short:
// there the downgrade IS the control, which is why running a contributor's
// branch under it is ordinary rather than a defect.
var privilegedTriggers = []string{
	"pull_request_target",
	"pull_request_review",
	"pull_request_review_comment",
	"issue_comment",
	"workflow_run",
}

// sameRepoGuard is the comparison confining a job to branches of this
// repository. Spelled once, because it is both what the gate looks for and what
// its failure tells an author to write.
const sameRepoGuard = "github.event.pull_request.head.repo.full_name == github.repository"

// triggeredWorkflow is the workflow decoded to the fields THIS gate reads and
// no further, as every workflow-reading gate here decodes its own shape.
//
// `on` is a YAML 1.1 boolean, which is why some parsers file the key under
// `true`; yaml.v3 keeps it a string.
type triggeredWorkflow struct {
	On   map[string]yaml.Node `yaml:"on"`
	Jobs map[string]struct {
		If    string `yaml:"if"`
		Steps []struct {
			If   string `yaml:"if"`
			Uses string `yaml:"uses"`
			With struct {
				Ref string `yaml:"ref"`
			} `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// namesAHeadRef reports whether a checkout `ref:` resolves to a commit the pull
// request's author controls.
//
// Keyed on `.head` rather than on one expression, because the contributor's tip
// has several spellings — `github.event.pull_request.head.sha`, the `.head.ref`
// branch name, `github.head_ref`, and `github.event.workflow_run.head_sha` on
// the trigger that carries a run's own origin. Matching the whole family
// over-reports onto a ref that is somebody's head but not a fork's, and that is
// the safe direction: the cost is a condition a reviewer reads, where the cost
// of missing one is the checkout this gate exists to find.
func namesAHeadRef(ref string) bool {
	return strings.Contains(ref, ".head")
}

// privilegedTriggersOf is the events in this workflow that carry an
// undowngraded token, named so a failure can say which one it is about.
func privilegedTriggersOf(wf triggeredWorkflow) []string {
	var found []string
	for _, trigger := range privilegedTriggers {
		if _, declared := wf.On[trigger]; declared {
			found = append(found, trigger)
		}
	}
	return found
}

func readTriggeredWorkflow(t *testing.T, path string) triggeredWorkflow {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob above
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var wf triggeredWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return wf
}

func TestNoPrivilegedTriggerRunsCodeFromAPullRequestsOwnBranch(t *testing.T) {
	t.Parallel()

	privileged, checkouts := 0, 0
	for _, path := range workflowFiles(t) {
		wf := readTriggeredWorkflow(t, path)
		triggers := privilegedTriggersOf(wf)
		if len(triggers) == 0 {
			continue
		}
		privileged++
		for _, name := range slices.Sorted(maps.Keys(wf.Jobs)) {
			job := wf.Jobs[name]
			guarded := strings.Contains(normalizeCondition(job.If), sameRepoGuard)
			for _, step := range job.Steps {
				if !namesAHeadRef(step.With.Ref) {
					continue
				}
				checkouts++
				// The step's own condition counts too: a checkout that never
				// runs puts no contributor-authored file on the runner, which is
				// the property under test rather than where it is written.
				if guarded || strings.Contains(normalizeCondition(step.If), sameRepoGuard) {
					continue
				}
				t.Errorf("%s: job %q checks out %s while %s can start this workflow, so a fork's "+
					"branch runs its own copy of whatever this job executes — with a token GitHub "+
					"has NOT downgraded, because that downgrade only applies to `pull_request`. "+
					"Add the same-repository comparison to the job's `if:` (or to the checkout "+
					"step's):\n\t%s",
					filepath.Base(path), name, step.With.Ref,
					strings.Join(triggers, ", "), sameRepoGuard)
			}
		}
	}

	// A scan that matched nothing reports exactly like a tree with nothing to
	// find, so both halves of what this gate looks for are counted.
	if privileged == 0 {
		t.Errorf("no workflow under %s declares any of %s. Either the last privileged trigger is "+
			"gone — delete this gate with it — or the decode stopped seeing `on:`, which reads "+
			"exactly like a clean tree", workflowDir, strings.Join(privilegedTriggers, ", "))
	}
	if checkouts == 0 {
		t.Errorf("no job in a privileged workflow checks out a pull request's head. Either that " +
			"pattern is gone — delete this gate with it — or namesAHeadRef has stopped matching " +
			"the refs it is meant to, and this gate judged nothing")
	}
}

// What the scan above cannot see is a head checkout spelled another way, so the
// matcher is asserted in both directions on the refs it must and must not
// answer to.
func TestTheHeadRefScanMatchesTheContributorsTipAndNotItsNeighbours(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		ref      string
		fromHead bool
	}{
		{"${{ github.event.pull_request.head.sha }}", true},
		{"${{ github.event.pull_request.head.ref }}", true},
		{"${{ github.head_ref }}", true},
		{"${{ github.event.workflow_run.head_sha }}", true},
		// The merge ref is GitHub's own commit, not the contributor's tip.
		{"${{ github.event.pull_request.merge_commit_sha }}", false},
		{"${{ github.event.pull_request.base.sha }}", false},
		{"${{ github.sha }}", false},
		{"main", false},
		// No `ref:` at all: the checkout resolves the event's default, which for
		// every privileged trigger here is the base repository.
		{"", false},
	} {
		if got := namesAHeadRef(tc.ref); got != tc.fromHead {
			t.Errorf("%q: matched=%v, want %v", tc.ref, got, tc.fromHead)
		}
	}
}
