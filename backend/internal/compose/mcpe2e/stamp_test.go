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

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// tree plants the committed files a stamp is read from, with one tool served
// plainly (log_activity) and one verb behind a router (change_deal fronts
// move_deal).
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, body string) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(mcpInfoFile, `{"instructions":"Be careful.","tools":{"tools":[`+
		`{"name":"log_activity","description":"Log one thing."},`+
		`{"name":"change_deal","description":"Change a deal."},`+
		`{"name":"read_record","description":"Read one record."}]}}`)
	write(routesFile, `{"routers":{"change_deal":{"field":"action","actions":{"move":"move_deal"}}}}`)
	write(candidatesFile, `{"judge":{"judge_routes":{"api":{"model":"judge-1"}}}}`)
	for _, name := range append(append(append([]string{}, harnessFiles...), restTools...), restSkill...) {
		write(name, "# "+name)
	}
	write(filepath.Join(ScenarioDir, "case1-log.yaml"), "name: case1_log\nprompt: |\n  Log it.\n"+
		"must_call:\n  - log_activity\nmust_call_with:\n  - move_deal.stage=won\njudge:\n  - it says so\n")
	write(filepath.Join(ScenarioDir, "case2-read.yaml"), "name: case2_read\nmust_call: [read_record]\n")
	return root
}

func stampsOf(t *testing.T, root string) map[string]Stamp {
	t.Helper()
	return stampsAt(t, root, DoorMCP)
}

func stampsAt(t *testing.T, root string, door Door) map[string]Stamp {
	t.Helper()
	stamps, err := CurrentStamps(root, door)
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}
	return stamps
}

func edit(t *testing.T, root, name, from, to string) {
	t.Helper()
	path := filepath.Join(root, name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), from) {
		t.Fatalf("%s carries no %q to edit", name, from)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), from, to, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAToolDescriptionChangeStalesOnlyTheScenariosThatNameIt(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, "Log one thing.", "Log one thing, once.")
	after := stampsOf(t, root)
	if got := before["case1_log"].Moved(after["case1_log"]); !reflect.DeepEqual(got, []string{"tool copy"}) {
		t.Errorf("case1_log names log_activity, moved = %v, want [tool copy]", got)
	}
	if got := before["case2_read"].Moved(after["case2_read"]); len(got) != 0 {
		t.Errorf("case2_read names no log_activity, moved = %v, want nothing", got)
	}
}

func TestAVerbIsStampedByTheRouterThatFrontsIt(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, "Change a deal.", "Change one deal.")
	if got := before["case1_log"].Moved(stampsOf(t, root)["case1_log"]); !reflect.DeepEqual(got, []string{"tool copy"}) {
		t.Errorf("move_deal sits behind change_deal, moved = %v, want [tool copy]", got)
	}
}

func TestEachOtherPartNamesItself(t *testing.T) {
	for _, tc := range []struct {
		file, from, to, part string
	}{
		{mcpInfoFile, "Be careful.", "Be very careful.", "server instructions"},
		{candidatesFile, "judge-1", "judge-2", "judge"},
		{"e2e/llm/check.py", "check.py", "check.py changed", "harness"},
		{filepath.Join(ScenarioDir, "case1-log.yaml"), "Log it.", "Log it now.", "scenario"},
	} {
		root := tree(t)
		before := stampsOf(t, root)
		edit(t, root, tc.file, tc.from, tc.to)
		if got := before["case1_log"].Moved(stampsOf(t, root)["case1_log"]); !reflect.DeepEqual(got, []string{tc.part}) {
			t.Errorf("editing %s: moved = %v, want [%s]", tc.file, got, tc.part)
		}
	}
}

func TestAScenarioWithNoJudgedCriterionNamesNoJudge(t *testing.T) {
	stamps := stampsOf(t, tree(t))
	if stamps["case1_log"].Judge != "judge-1" || stamps["case2_read"].Judge != "" {
		t.Errorf("judge parts: %q and %q, want judge-1 and empty", stamps["case1_log"].Judge, stamps["case2_read"].Judge)
	}
}

func TestAVerdictWithoutAStampIsStale(t *testing.T) {
	if got := Recheck(nil, Stamp{}, true); !reflect.DeepEqual(got, []string{Unstamped}) {
		t.Errorf("an unstamped verdict: %v, want [%s]", got, Unstamped)
	}
	if got := Recheck(&RecordedStamp{}, Stamp{}, false); !reflect.DeepEqual(got, []string{"no scenario"}) {
		t.Errorf("a verdict whose scenario is gone: %v", got)
	}
}

func TestNewlyStaleListsOnlyWhatTheChangeMoved(t *testing.T) {
	base := tree(t)
	head := tree(t)
	stamps := stampsOf(t, base)
	dir := filepath.Join(head, VerdictTrees[DoorMCP], "model-a")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, stamp := range stamps {
		body, err := json.Marshal(map[string]any{"scenario": name, "stamp": RecordedStamp{Stamp: stamp, Commit: "c"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "case9_old.json"), []byte(`{"scenario":"case9_old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	edit(t, head, mcpInfoFile, "Log one thing.", "Log one thing, once.")
	lines, err := NewlyStale(base, head)
	if err != nil {
		t.Fatalf("NewlyStale: %v", err)
	}
	if len(lines) != 1 || lines[0].Scenario != "case1_log" || lines[0].Folder != "model-a" {
		t.Fatalf("newly stale = %+v, want only model-a/case1_log", lines)
	}
}

func TestAnUnknownToolNameStillMovesTheDigestWhenItAppears(t *testing.T) {
	root := tree(t)
	edit(t, root, filepath.Join(ScenarioDir, "case2-read.yaml"), "[read_record]", "[read_record, made_up]")
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, `{"name":"read_record"`, `{"name":"made_up","description":"New."},{"name":"read_record"`)
	if got := before["case2_read"].Moved(stampsOf(t, root)["case2_read"]); !reflect.DeepEqual(got, []string{"tool copy"}) {
		t.Errorf("a named tool that starts being served: moved = %v, want [tool copy]", got)
	}
}

func TestTheReaderTakesEveryToolKeyAndItsSpelling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.yaml")
	body := "name: s\n# a comment\nmust_call:\n  # why\n  - a|b\nmay_call: [c, d]\n" +
		"must_call_with:\n  - e.x=1\nmust_not_call_with:\n  - 'f~g|h'\nmust_call_matching:\n  - 'i~j || k~l'\n" +
		"prompt: |\n  must_call:\n  - not_a_tool\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	sc, err := readScenario(path)
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(slices.Values(sc.tools))
	if want := []string{"a", "b", "c", "d", "e", "f", "i", "k"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

// The real tree stamps whole: every scenario reads, and the published catalog
// carries the instructions the stamp needs.
func TestTheCommittedTreeStampsEveryScenario(t *testing.T) {
	// Go's test cache never rechecks a file outside this module.
	if err := gatekit.DeclareInputs(os.Getenv, "../../../../docs", "../../../../e2e"); err != nil {
		t.Fatal(err)
	}
	stamps := stampsOf(t, filepath.Join("..", "..", "..", ".."))
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "..", ScenarioDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stamps) != len(files) || len(files) == 0 {
		t.Fatalf("stamped %d of %d scenarios", len(stamps), len(files))
	}
}

func TestTheCatalogsLayoutIsNotToolCopy(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, `{"name":"read_record","description":"Read one record."}`,
		"{\n  \"description\": \"Read one record.\",\n  \"name\": \"read_record\"\n}")
	if got := before["case2_read"].Moved(stampsOf(t, root)["case2_read"]); len(got) != 0 {
		t.Errorf("re-indenting the catalog moved %v, want nothing", got)
	}
}

func TestTheRestDoorIsStampedWithWhatItsModelReads(t *testing.T) {
	root := tree(t)
	before := stampsAt(t, root, DoorREST)
	edit(t, root, mcpInfoFile, "Log one thing.", "Log one thing, once.")
	edit(t, root, mcpInfoFile, "Be careful.", "Be very careful.")
	if got := before["case1_log"].Moved(stampsAt(t, root, DoorREST)["case1_log"]); len(got) != 0 {
		t.Errorf("MCP copy moved a REST stamp: %v", got)
	}
	for _, tc := range []struct{ file, part string }{
		{"e2e/llm/rest_ops.json", "tool copy"},
		{"e2e/llm/restmap.py", "tool copy"},
		{"backend/internal/compose/agentbundle/SKILL.md.tmpl", "server instructions"},
	} {
		root := tree(t)
		before := stampsAt(t, root, DoorREST)
		edit(t, root, tc.file, "# ", "# edited ")
		if got := before["case1_log"].Moved(stampsAt(t, root, DoorREST)["case1_log"]); !reflect.DeepEqual(got, []string{tc.part}) {
			t.Errorf("editing %s moved %v of a REST stamp, want [%s]", tc.file, got, tc.part)
		}
	}
}
