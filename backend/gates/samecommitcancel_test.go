// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// No pull request workflow that also runs without a new commit cancels a run
// in flight.
//
// Such a run and the one it cancels share a head commit, so the head keeps a
// `cancelled` check beside the one that replaced it. Renovate reads any
// conclusion other than success, neutral or skipped as pending. So it never
// automerges that branch, and the open pull request holds a slot for good.

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// newCommitTypes are the pull_request activity types that come with a commit
// the head has not had yet, or happen once in a pull request's life. They are
// also GitHub's default when a workflow lists no types.
var newCommitTypes = []string{"opened", "synchronize", "reopened", "ready_for_review"}

// sameCommitEvents fire on a pull request whose head has not moved, as often as
// someone edits, labels, reviews or comments.
var sameCommitEvents = []string{"pull_request_review", "pull_request_review_comment", "issue_comment"}

// cancelWorkflow is the part of a workflow this gate reads.
type cancelWorkflow struct {
	On          yaml.Node `yaml:"on"`
	Concurrency yaml.Node `yaml:"concurrency"`
	Jobs        map[string]struct {
		Concurrency yaml.Node `yaml:"concurrency"`
	} `yaml:"jobs"`
}

// sameCommitTriggers lists the triggers of wf that start a run on an unchanged
// pull request head.
func (wf cancelWorkflow) sameCommitTriggers() []string {
	var found []string
	// A bare event or a list of events carries no `types:`, so pull_request
	// there has only its default, new-commit types.
	if wf.On.Kind != yaml.MappingNode {
		for _, event := range (queueWorkflow{On: wf.On}).triggers() {
			if slices.Contains(sameCommitEvents, event) {
				found = append(found, event)
			}
		}
		return found
	}
	for i := 0; i+1 < len(wf.On.Content); i += 2 {
		event, settings := wf.On.Content[i].Value, wf.On.Content[i+1]
		switch {
		case slices.Contains(sameCommitEvents, event):
			found = append(found, event)
		case slices.Contains(pullRequestTriggers, event):
			for _, activity := range activityTypes(settings) {
				if !slices.Contains(newCommitTypes, activity) {
					found = append(found, event+"."+activity)
				}
			}
		}
	}
	return found
}

// activityTypes reads the `types:` list of one trigger, empty when it has none.
func activityTypes(settings *yaml.Node) []string {
	if settings.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(settings.Content); i += 2 {
		if settings.Content[i].Value != "types" {
			continue
		}
		var types []string
		for _, item := range settings.Content[i+1].Content {
			types = append(types, item.Value)
		}
		return types
	}
	return nil
}

// cancellingScopes names each concurrency block of wf that may cancel a run in
// flight. An expression counts, because it can evaluate to true.
func (wf cancelWorkflow) cancellingScopes() []string {
	var scopes []string
	if _, cancel := concurrencySettings(wf.Concurrency); cancel != "" && cancel != "false" {
		scopes = append(scopes, "the workflow")
	}
	for _, job := range slices.Sorted(maps.Keys(wf.Jobs)) {
		if _, cancel := concurrencySettings(wf.Jobs[job].Concurrency); cancel != "" && cancel != "false" {
			scopes = append(scopes, "job "+job)
		}
	}
	return scopes
}

// TestNoSameCommitPullRequestRunCancelsAnother judges every workflow that can
// start on a pull request head that has not moved.
func TestNoSameCommitPullRequestRunCancelsAnother(t *testing.T) {
	t.Parallel()
	judged := 0
	for _, path := range workflowFiles(t) {
		raw, err := gatekit.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		var wf cancelWorkflow
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		triggers := wf.sameCommitTriggers()
		if len(triggers) == 0 {
			continue
		}
		judged++
		for _, scope := range wf.cancellingScopes() {
			t.Errorf("%s: %s sets cancel-in-progress, and %v start a run on an unchanged head. "+
				"A cancelled run leaves a `cancelled` check there, and Renovate never merges a "+
				"head with one. Set `cancel-in-progress: false`: a run that is still waiting is "+
				"replaced before it starts and leaves no check.",
				filepath.Base(path), scope, triggers)
		}
	}
	// closing-declaration.yml runs on `edited`; a census of none means the
	// trigger decode stopped reading, not that the tree is clean.
	if judged == 0 {
		t.Fatalf("no workflow under %s runs on an unchanged pull request head; the trigger decode "+
			"stopped reading, so this gate was about to pass without judging anything", workflowDir)
	}
}
