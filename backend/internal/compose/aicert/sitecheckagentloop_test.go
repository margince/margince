// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// agentLoopSite is the morning brief's site, whose check replays a run through
// the production runner.
var agentLoopSite = aitasks.Site{Task: ai.TaskAgentLoop, Variant: "morning_brief", Kind: ai.SiteKindAgentLoop}

// agentLoopWalk walks the agent_loop check over the source on disk, with the
// files of edits served from their given text instead. A key is a path under
// backend/internal, as "compose/certcase_agentloop.go".
func agentLoopWalk(t *testing.T, edits map[string]func(string) string) *checkWalk {
	t.Helper()
	return siteWalk(t, agentLoopSite, "agentLoopCase", edits)
}

// siteWalk walks the check of one site's case type in package compose.
func siteWalk(t *testing.T, site aitasks.Site, typeName string, edits map[string]func(string) string) *checkWalk {
	t.Helper()
	index := newSourceIndex(t)
	index.declared = declaredDependencies(t, site)
	index.overlay = map[string]string{}
	for rel, edit := range edits {
		path := filepath.Join(backendRoot, "internal", filepath.FromSlash(rel))
		body, err := readSource(path)
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		changed := edit(body)
		if changed == body {
			t.Fatalf("the edit of %s changed nothing, so a digest that does not move proves nothing", rel)
		}
		index.overlay[path] = changed
	}
	pkg := index.load(t, modulePath+"/internal/compose")
	return index.walkOf(t, pkg, typeName)
}

func replaceOnce(old, repl string) func(string) string {
	return func(body string) string { return strings.Replace(body, old, repl, 1) }
}

func TestTheAgentLoopCheckDoesNotReachTheStoresBehindTheToolRegistry(t *testing.T) {
	walk := agentLoopWalk(t, nil)
	if got := len(walk.seen); got > 1500 {
		t.Errorf("the check reaches %d declarations; a replay of one run needs a few hundred, and a count this high means the walk is in the stores again", got)
	}
	for _, store := range []string{"modules/contacts.Store", "modules/activities.Store", "modules/capture.Sink", "modules/agents.Registry"} {
		for _, id := range walk.seen {
			if strings.HasSuffix(id, "/internal/"+store) {
				t.Errorf("the check reaches %s, which no grading reads", store)
			}
		}
	}
}

// What cannot change how a run is graded moves no digest, and what can does. The
// first two edits made the records stale for nothing. The rest are real grading
// dependencies, so a walk that skipped too much fails here.
func TestOnlyAGradingChangeMovesTheAgentLoopCheck(t *testing.T) {
	base := walkDigest(t, agentLoopWalk(t, nil))
	for name, edits := range map[string]map[string]func(string) string{
		"a routing field": {"modules/ai/routing.go": replaceOnce("type RoutingConfig struct {", "type RoutingConfig struct {\n\tTaskLadders map[string]string")},
		"an operator definition": {"shared/kernel/systemtext/operatorcopy.go": replaceOnce(
			"var operatorDefinitions = map[string]Definition{", "var operatorDefinitions = map[string]Definition{\n\t\"zz.added\": {},")},
	} {
		if got := walkDigest(t, agentLoopWalk(t, edits)); got != base {
			t.Errorf("%s moved the agent_loop check, and no grading reads it", name)
		}
	}
	for name, edits := range map[string]map[string]func(string) string{
		"the grading of a run": {"compose/certcase_agentloop.go": replaceOnce(
			"the run ended without writing with %s", "the run ended with no write by %s")},
		"the replay that precedes it": {"compose/certcase_agentloop.go": replaceOnce(
			"the loop could not replay the run: ", "the loop could not replay: ")},
		"the arguments a write is graded on": {"modules/agents/reserved.go": replaceOnce(
			"	return split.Args, nil", "	return nil, nil")},
		"the runner's allowlist": {"modules/agents/runner/allowlist.go": replaceOnce(
			"return fmt.Errorf(\"%w: %s\", errOutsideAgentSpec, tool)", "return fmt.Errorf(\"%w: %s!\", errOutsideAgentSpec, tool)")},
	} {
		if got := walkDigest(t, agentLoopWalk(t, edits)); got == base {
			t.Errorf("%s did not move the agent_loop check, so its records would read current after it changed", name)
		}
	}
}

// walkDigest is the digest of a walk, as checkDigest hashes it.
func walkDigest(t *testing.T, walk *checkWalk) string {
	t.Helper()
	return walk.index.hashWalk(t, walk)
}

// The registry is hashed as the specs it publishes, so a tool's spec moves the
// check, and each agent's own tools move only its own.
func TestAnAgentLoopCheckHashesTheToolSpecsItReplaysThrough(t *testing.T) {
	brief := declaredDependencies(t, agentLoopSite)
	sweep := declaredDependencies(t, aitasks.Site{Task: ai.TaskAgentLoop, Variant: "overnight_at_risk_sweep", Kind: ai.SiteKindAgentLoop})
	if len(brief) != 1 || len(sweep) != 1 {
		t.Fatalf("declared dependencies = %v, %v; want one each", brief, sweep)
	}
	for key, digest := range brief {
		if sweep[key] == digest {
			t.Errorf("two agents with different tools hash the same specs for %s", key)
		}
		index := newSourceIndex(t)
		index.declared = map[string]string{key: digest + "x"}
		pkg := index.load(t, modulePath+"/internal/compose")
		moved := index.digestOf(t, pkg, "agentLoopCase")
		index.declared = brief
		if index.digestOf(t, pkg, "agentLoopCase") == moved {
			t.Error("a changed tool spec did not move the check")
		}
	}
	if got := declaredDependencies(t, aitasks.Site{Task: ai.TaskCaptureClassify, Variant: "classify", Kind: ai.SiteKindOneShot}); got != nil {
		t.Errorf("a one-shot site declares %v, want nothing", got)
	}
}

// The full-path cases record what the production pass stored, so the floors and
// the re-ask the pass applies decide the grade. A change there moves the check;
// the database ledger's transactions, which a certification run never builds,
// move nothing.
func TestAFloorOrADatabaseChangeMovesTheFullPathChecksAsItShould(t *testing.T) {
	classify := aitasks.Site{Task: ai.TaskCaptureClassify, Variant: "classify", Kind: ai.SiteKindOneShot}
	counterparty := aitasks.Site{Task: ai.TaskCaptureCounterpartyVerdict, Variant: "verdict", Kind: ai.SiteKindOneShot}
	for _, tc := range []struct {
		name     string
		site     aitasks.Site
		typeName string
		edits    map[string]func(string) string
		moves    bool
	}{
		{
			"the classify floor", classify, "captureClassifyCase",
			map[string]func(string) string{"compose/captureclassify.go": replaceOnce("classifyConfidenceFloor = 0.7", "classifyConfidenceFloor = 0.9")},
			true,
		},
		{
			"the verdict floor", counterparty, "counterpartyVerdictCase",
			map[string]func(string) string{"compose/captureverdict.go": replaceOnce("verdictConfidenceFloor = 0.7", "verdictConfidenceFloor = 0.9")},
			true,
		},
		{
			"the database apply", counterparty, "counterpartyVerdictCase",
			map[string]func(string) string{"compose/captureverdictapply.go": replaceOnce("is not a sender kind", "is no sender kind")},
			false,
		},
	} {
		base := walkDigest(t, siteWalk(t, tc.site, tc.typeName, nil))
		moved := walkDigest(t, siteWalk(t, tc.site, tc.typeName, tc.edits)) != base
		if moved != tc.moves {
			t.Errorf("%s: moved = %v, want %v", tc.name, moved, tc.moves)
		}
	}
}
