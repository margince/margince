// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every workflow that reports a check on a pull request also runs on the merge
// queue, unless it is waived as one no ruleset requires.
//
// The queue asks for each required check again, on its own commit. A workflow
// with no `merge_group` trigger never answers, so every entry waits for the
// timeout and nothing merges.

// The ruleset that names the required checks is in GitHub, not in this tree.
// So a new pull request workflow counts as required until a waiver says it is
// not. Not caught: a waived workflow that a ruleset requires after all.

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// pullRequestTriggers are the events that put a check on a pull request's head.
var pullRequestTriggers = []string{"pull_request", "pull_request_target"}

const mergeQueueTrigger = "merge_group"

// advisoryPullRequestWorkflows run on pull requests, report a check that always
// succeeds, and are not required, so the queue never waits for them.
var advisoryPullRequestWorkflows = gatekit.Waive(map[string]string{
	"closing-declaration.yml": "advisory: it warns in a comment and its job always succeeds, " +
		"so no ruleset requires it and the queue never asks for it",
	"docs-describing.yml": "advisory: it warns in a comment and its job always succeeds, " +
		"so no ruleset requires it and the queue never asks for it",
	"review-coverage.yml": "advisory: it reports review coverage in a sticky comment and " +
		"never blocks, so no ruleset requires it and the queue never asks for it",
})

// queueWorkflow is the part of a workflow these gates read: its triggers, its
// concurrency and each job's condition.
type queueWorkflow struct {
	On          yaml.Node `yaml:"on"`
	Concurrency yaml.Node `yaml:"concurrency"`
	Jobs        map[string]struct {
		If string `yaml:"if"`
	} `yaml:"jobs"`
}

// triggers lists the events a workflow's `on:` names. GitHub accepts three
// spellings: one event, a list of events, or a map of them.
func (wf queueWorkflow) triggers() []string {
	switch wf.On.Kind {
	case yaml.ScalarNode:
		return []string{wf.On.Value}
	case yaml.SequenceNode:
		events := make([]string, 0, len(wf.On.Content))
		for _, item := range wf.On.Content {
			events = append(events, item.Value)
		}
		return events
	case yaml.MappingNode:
		events := make([]string, 0, len(wf.On.Content)/2)
		for i := 0; i < len(wf.On.Content); i += 2 {
			events = append(events, wf.On.Content[i].Value)
		}
		return events
	default:
		return nil
	}
}

func readQueueWorkflow(t *testing.T, path string) queueWorkflow {
	t.Helper()
	raw, err := gatekit.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var wf queueWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(wf.triggers()) == 0 {
		t.Fatalf("%s names no trigger that this gate can read, so it was about to judge nothing",
			filepath.Base(path))
	}
	return wf
}

// mergeQueueWorkflows returns each workflow the queue starts, by file name.
func mergeQueueWorkflows(t *testing.T) map[string]queueWorkflow {
	t.Helper()
	found := map[string]queueWorkflow{}
	for _, path := range workflowFiles(t) {
		wf := readQueueWorkflow(t, path)
		if slices.Contains(wf.triggers(), mergeQueueTrigger) {
			found[filepath.Base(path)] = wf
		}
	}
	if len(found) == 0 {
		t.Fatalf("no workflow under %s runs on %s, so the queue would wait for every required "+
			"check until it timed out", workflowDir, mergeQueueTrigger)
	}
	return found
}

func TestEveryPullRequestWorkflowAlsoRunsOnTheMergeQueue(t *testing.T) {
	t.Parallel()
	judged := 0
	for _, path := range workflowFiles(t) {
		name := filepath.Base(path)
		events := readQueueWorkflow(t, path).triggers()
		if !slices.ContainsFunc(pullRequestTriggers, func(e string) bool { return slices.Contains(events, e) }) {
			continue
		}
		judged++
		if slices.Contains(events, mergeQueueTrigger) {
			continue
		}
		if advisoryPullRequestWorkflows.Waived(t, name) {
			continue
		}
		t.Errorf("%s reports a check on pull requests but has no %s trigger. If a ruleset requires "+
			"that check, the queue waits for it on its own commit and ejects every entry at the "+
			"timeout. Add `%s:` to its triggers, or waive it here as advisory.",
			name, mergeQueueTrigger, mergeQueueTrigger)
	}
	if judged == 0 {
		t.Fatalf("no workflow under %s runs on %v; the decode stopped seeing triggers, so this gate "+
			"was about to pass without judging anything", workflowDir, pullRequestTriggers)
	}
	advisoryPullRequestWorkflows.AssertAllMatched(t)
}

// queueEntryKeys are the expressions that differ from one queue entry to the
// next. A concurrency group without one puts every queue run in one group, and
// GitHub keeps one pending run per group: it cancels the rest.
var queueEntryKeys = []string{"github.ref", "github.sha", "github.run_id", "github.event.merge_group"}

// TestNoMergeQueueRunCancelsAnother holds the concurrency of each workflow the
// queue starts. A cancelled required check fails the entry it was asked for.
func TestNoMergeQueueRunCancelsAnother(t *testing.T) {
	t.Parallel()
	workflows := mergeQueueWorkflows(t)
	for _, name := range slices.Sorted(maps.Keys(workflows)) {
		group, cancel := concurrencySettings(workflows[name].Concurrency)
		if group == "" {
			continue
		}
		if !slices.ContainsFunc(queueEntryKeys, func(key string) bool { return strings.Contains(group, key) }) {
			t.Errorf("%s: concurrency group %q is the same for every queue entry, so a new entry "+
				"cancels the pending run of the one before it. Add one of %v to the group.",
				name, group, queueEntryKeys)
		}
		if strings.TrimSpace(cancel) == "true" {
			t.Errorf("%s: cancel-in-progress is true on every event, so a push to the pull request "+
				"can cancel the queue's verdict. Limit it to pull_request, as ci.yml does.", name)
		}
	}
}

// concurrencySettings reads `concurrency:` in either spelling: a bare group
// name, or a map with `group` and `cancel-in-progress`.
func concurrencySettings(node yaml.Node) (group, cancel string) {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value, ""
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			switch node.Content[i].Value {
			case "group":
				group = node.Content[i+1].Value
			case "cancel-in-progress":
				cancel = node.Content[i+1].Value
			}
		}
	}
	return group, cancel
}

// pullRequestPayload is the event payload that is empty on the merge queue.
// There `github.event.pull_request.draft == false` reads false. A job condition
// that reads it without asking which event fired skips on the queue.
const (
	pullRequestPayload = "github.event.pull_request"
	eventNameGuard     = "github.event_name"
)

func TestNoMergeQueueJobConditionReadsAnEmptyPullRequest(t *testing.T) {
	t.Parallel()
	workflows := mergeQueueWorkflows(t)
	conditions := 0
	for _, name := range slices.Sorted(maps.Keys(workflows)) {
		jobs := workflows[name].Jobs
		for _, job := range slices.Sorted(maps.Keys(jobs)) {
			cond := normalizeCondition(jobs[job].If)
			if cond == "" {
				continue
			}
			conditions++
			if strings.Contains(cond, pullRequestPayload) && !strings.Contains(cond, eventNameGuard) {
				t.Errorf("%s: job %q reads %s in its condition without %s. That payload is empty on "+
					"%s, so the job skips there. Guard it, as ci.yml does: "+
					"`github.event_name != 'pull_request' || github.event.pull_request.draft == false`.",
					name, job, pullRequestPayload, eventNameGuard, mergeQueueTrigger)
			}
		}
	}
	if conditions == 0 {
		t.Fatalf("no job in a %s workflow carries an `if:`; the decode stopped seeing conditions, "+
			"so this gate was about to pass without judging anything", mergeQueueTrigger)
	}
}
