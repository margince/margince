// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// verdict files one model-a verdict under a planted tree's door, stamped or not.
func verdict(t *testing.T, root string, door Door, name string, stamp *RecordedStamp, search string) {
	t.Helper()
	dir := filepath.Join(root, VerdictTrees[door], "model-a")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"scenario": name, "passed": 2, "runs": 2, "pass_at": 2,
		"search": search, "stamp": stamp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTheReportReadsEachVerdictAgainstItsOwnDoor(t *testing.T) {
	root := tree(t)
	mcp, rest := stampsAt(t, root, DoorMCP), stampsAt(t, root, DoorREST)
	verdict(t, root, DoorMCP, "case1_log", &RecordedStamp{Stamp: mcp["case1_log"]}, "semantic")
	verdict(t, root, DoorMCP, "case2_read", nil, "semantic")
	verdict(t, root, DoorREST, "case1_log", &RecordedStamp{Stamp: rest["case1_log"]}, "semantic")
	verdict(t, root, DoorREST, "case9_gone", &RecordedStamp{}, "semantic")
	lines, err := Report(root)
	if err != nil {
		t.Fatal(err)
	}
	got := Summarise(lines)
	for _, want := range []string{
		"mcp_e2e/model-a: 1 current, 1 stale",
		"mcp_e2e_rest/model-a: 1 current, 1 stale",
		"stale  mcp_e2e/model-a case2_read: no stamp",
		"stale  mcp_e2e_rest/model-a case9_gone: no scenario",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report has no line %q:\n%s", want, got)
		}
	}
}

// Grading saved transcripts again keeps the run's commit beside the grader's.
// The verdict reads current only while the digests it measured still match.
func TestARegradedVerdictNamesBothCommitsAndIsCurrentOnlyOnItsDigests(t *testing.T) {
	root := tree(t)
	mcp := stampsAt(t, root, DoorMCP)
	moved := mcp["case2_read"]
	moved.Tools = "copy-since-changed"
	for name, stamp := range map[string]Stamp{"case1_log": mcp["case1_log"], "case2_read": moved} {
		verdict(t, root, DoorMCP, name, &RecordedStamp{Stamp: stamp, Commit: "run1", GradedCommit: "grade2"}, "semantic")
	}
	lines, err := Report(root)
	if err != nil {
		t.Fatal(err)
	}
	got := Summarise(lines)
	for _, want := range []string{
		"mcp_e2e/model-a: 1 current, 1 stale",
		"stale  mcp_e2e/model-a case2_read: tool copy",
		"re-graded  mcp_e2e/model-a case1_log: run at run1, graded at grade2",
		"re-graded  mcp_e2e/model-a case2_read: run at run1, graded at grade2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report has no line %q:\n%s", want, got)
		}
	}
	for _, live := range []*RecordedStamp{nil, {Commit: "run1"}, {Commit: "run1", GradedCommit: "run1"}} {
		if live.Regraded() {
			t.Errorf("a verdict graded as it ran reads as re-graded: %+v", live)
		}
	}
}

func TestATreeTheReaderCannotTrustIsRefused(t *testing.T) {
	root := t.TempDir()
	verdict(t, root, DoorMCP, "case1_log", nil, "semantic")
	verdict(t, root, DoorMCP, "case2_read", nil, "lexical")
	if _, err := ReadTree(filepath.Join(root, VerdictTrees[DoorMCP])); err == nil ||
		!strings.Contains(err.Error(), "mixes lexical") {
		t.Errorf("a folder mixing lexical and semantic verdicts read as %v", err)
	}
	nested := filepath.Join(root, VerdictTrees[DoorREST], "vendor", "model")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTree(filepath.Join(root, VerdictTrees[DoorREST])); err == nil {
		t.Error("a nested model folder read without an error")
	}
	if verdicts, err := ReadTree(filepath.Join(root, "no-such-tree")); err != nil || verdicts != nil {
		t.Errorf("a missing tree is no verdict yet: %v %v", verdicts, err)
	}
}

func TestTheStampsTheLaneFilesAreWrittenWhole(t *testing.T) {
	root := tree(t)
	path := filepath.Join(t.TempDir(), "stamps.json")
	if err := WriteStamps(root, DoorREST, path); err != nil {
		t.Fatal(err)
	}
	var written map[string]Stamp
	body, err := os.ReadFile(path) // #nosec G304 -- t.TempDir() + a literal filename, test-only
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &written); err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 || !reflect.DeepEqual(written["case1_log"], stampsAt(t, root, DoorREST)["case1_log"]) {
		t.Errorf("wrote %v", written)
	}
}

func TestAStampItCannotMakeSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		name, file, from, to string
		door                 Door
	}{
		{"no instructions", mcpInfoFile, `"instructions":"Be careful.",`, ``, DoorMCP},
		{"no tools", mcpInfoFile, `"tools":{"tools":[`, `"tools":{"none":[`, DoorMCP},
		{"an unnamed tool", mcpInfoFile, `{"name":"read_record",`, `{`, DoorMCP},
		{"no judge", candidatesFile, `"judge-1"`, `""`, DoorMCP},
		{"no skill", restSkill[0], "# ", "# ", DoorREST},
	} {
		root := tree(t)
		edit(t, root, tc.file, tc.from, tc.to)
		if tc.name == "no skill" {
			if err := os.Remove(filepath.Join(root, restSkill[0])); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := CurrentStamps(root, tc.door); err == nil {
			t.Errorf("%s: stamped without an error", tc.name)
		}
	}
	if _, err := CurrentStamps(tree(t), Door("smoke-signal")); err == nil {
		t.Error("a door nobody serves stamped without an error")
	}
	if _, err := NewlyStale(t.TempDir(), tree(t)); err == nil {
		t.Error("a base tree with nothing to stamp gave a list")
	}
}

func TestOnlyAPassingRunDrivesATool(t *testing.T) {
	v := Verdict{Passed: 1, Runs: 2, ToolsByRun: []RunTools{
		{Passed: true, Tools: []string{"find_records"}},
		{Passed: false, Tools: []string{"find_records", "read_tags"}},
	}}
	if got := v.PassingTools(); !slices.Equal(got, []string{"find_records"}) {
		t.Errorf("PassingTools() = %v, want [find_records]: read_tags only worked in a failed run", got)
	}
}

func TestAnOlderVerdictsToolsCountOnlyWhenEveryRunPassed(t *testing.T) {
	tools := []string{"find_records"}
	if got := (Verdict{Passed: 2, Runs: 2, ToolsCalled: tools}).PassingTools(); !slices.Equal(got, tools) {
		t.Errorf("every run passed: PassingTools() = %v, want %v", got, tools)
	}
	if got := (Verdict{Passed: 2, Runs: 3, ToolsCalled: tools}).PassingTools(); got != nil {
		t.Errorf("a run failed: PassingTools() = %v, want none, since the union may hold its tools", got)
	}
}
