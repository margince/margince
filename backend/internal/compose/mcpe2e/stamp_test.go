// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
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
		`{"name":"read_record","description":"Read one record."}]},`+
		`"action_catalog":{"change_deal":{"router":"change_deal","actions":[{"action":"move","body":{"properties":{"stage":{"description":"The stage to move to."}}}}]}},`+
		`"resources":{"resources":[{"name":"query_vocabulary","uri":"margince://schema/query","description":"What a plan may say."}]},`+
		`"documents":{"margince://schema/query":{"fields":["name","stage"]}}}`)
	write(routesFile, `{"routers":{"change_deal":{"field":"action","actions":{"move":"move_deal"}}}}`)
	write(candidatesFile, `{"judge":{"judge_routes":{"api":{"model":"judge-1"}}}}`)
	for _, name := range append(append(append([]string{}, harnessFiles...), restTools...), restSkill...) {
		write(name, "# "+name)
	}
	write(transcriptFile, "SERVER = 'x'\n\ndef tool_name(bare):\n    return bare\n\n"+
		"# codex reads its own stream\ndef from_codex(lines):\n    return 'codex'\n\nCODEX_ONLY_AFTER = 1\n\n"+
		"_CODEX_SKIPPED = {'a'}\n\n# agy steps\n@staticmethod\ndef _agy_step(step):\n    \"\"\"Doc.\n"+
		"def from_codex(not_real):\n    \"\"\"\n    return 'agy'\n\ndef from_agy(lines):\n    return _agy_step(lines)\n")
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
	if got := before["case1_log"].Moved(after["case1_log"], ""); !reflect.DeepEqual(got, []string{"tool copy"}) {
		t.Errorf("case1_log names log_activity, moved = %v, want [tool copy]", got)
	}
	if got := before["case2_read"].Moved(after["case2_read"], ""); len(got) != 0 {
		t.Errorf("case2_read names no log_activity, moved = %v, want nothing", got)
	}
}

func TestAVerbIsStampedByTheRouterThatFrontsIt(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, "Change a deal.", "Change one deal.")
	if got := before["case1_log"].Moved(stampsOf(t, root)["case1_log"], ""); !reflect.DeepEqual(got, []string{"tool copy"}) {
		t.Errorf("move_deal sits behind change_deal, moved = %v, want [tool copy]", got)
	}
}

// A model reads a routed action's body in the action catalog, not in tools/list.
// A body change stales the scenarios that name a verb behind that router.
func TestAnActionCatalogChangeStalesOnlyTheScenariosThatReachItsRouter(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, "The stage to move to.", "The stage to move to; an agent never sends won.")
	after := stampsOf(t, root)
	if got := before["case1_log"].Moved(after["case1_log"], ""); !reflect.DeepEqual(got, []string{"tool copy"}) {
		t.Errorf("case1_log names move_deal behind change_deal, moved = %v, want [tool copy]", got)
	}
	if got := before["case2_read"].Moved(after["case2_read"], ""); len(got) != 0 {
		t.Errorf("case2_read reaches no change_deal, moved = %v, want nothing", got)
	}
}

func TestEachOtherPartNamesItself(t *testing.T) {
	for _, tc := range []struct {
		file, from, to, part string
	}{
		{mcpInfoFile, "Be careful.", "Be very careful.", "server instructions and resources"},
		{mcpInfoFile, `"stage"]`, `"stage","owner"]`, "server instructions and resources"},
		{candidatesFile, "judge-1", "judge-2", "judge"},
		{"e2e/llm/check.py", "check.py", "check.py changed", "harness"},
		{"e2e/llm/transcript.py", "return bare", "return bare + bare", "harness"},
		{filepath.Join(ScenarioDir, "case1-log.yaml"), "Log it.", "Log it now.", "scenario"},
	} {
		root := tree(t)
		before := stampsOf(t, root)
		edit(t, root, tc.file, tc.from, tc.to)
		if got := before["case1_log"].Moved(stampsOf(t, root)["case1_log"], ""); !reflect.DeepEqual(got, []string{tc.part}) {
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
	if got := Recheck(nil, Stamp{}, true, ""); !reflect.DeepEqual(got, []string{Unstamped}) {
		t.Errorf("an unstamped verdict: %v, want [%s]", got, Unstamped)
	}
	if got := Recheck(&RecordedStamp{}, Stamp{}, false, ""); !reflect.DeepEqual(got, []string{"no scenario"}) {
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
	if got := before["case2_read"].Moved(stampsOf(t, root)["case2_read"], ""); !reflect.DeepEqual(got, []string{"tool copy"}) {
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

// judgeTransport are the modules only judge.py imports, to reach the judge:
// how its CLI starts and the wire to its API. The stamp names the judge's model
// on its own, so a change to them moves no verdict and they are not harness.
var judgeTransport = []string{"e2e/llm/clean.py", "e2e/llm/providers.py"}

// lintOnly are lane modules that check how a scenario is written and grade no
// run. They stay outside the harness stamp only while no grader imports them.
var lintOnly = []string{"e2e/llm/scenariolint.py"}

// importLine and fromLine are one import statement of a lane module, at any
// indentation, since the grader imports some lazily: `import a, b as c` and
// `from x import y`.
var (
	importLine = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+([\w., \t]+)`)
	fromLine   = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+([\w.]+)[ \t]+import\b`)
)

// importedModules names every top-level module a Python source imports.
func importedModules(body string) []string {
	var modules []string
	for _, m := range importLine.FindAllStringSubmatch(body, -1) {
		for item := range strings.SplitSeq(m[1], ",") {
			if fields := strings.Fields(item); len(fields) > 0 {
				modules = append(modules, strings.Split(fields[0], ".")[0])
			}
		}
	}
	for _, m := range fromLine.FindAllStringSubmatch(body, -1) {
		modules = append(modules, strings.Split(m[1], ".")[0])
	}
	return modules
}

// graderClosure walks the imports from check.py and maps each lane module it
// reaches to the lane modules that import it.
func graderClosure(t *testing.T, root string) map[string][]string {
	t.Helper()
	importers := map[string][]string{"e2e/llm/check.py": nil}
	queue := []string{"e2e/llm/check.py"}
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		body, err := os.ReadFile(filepath.Join(root, file)) // #nosec G304 -- a lane file under the repository root
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range importedModules(string(body)) {
			module := "e2e/llm/" + name + ".py"
			if _, err := os.Stat(filepath.Join(root, module)); err != nil {
				continue
			}
			if _, seen := importers[module]; !seen {
				queue = append(queue, module)
			}
			if !slices.Contains(importers[module], file) {
				importers[module] = append(importers[module], file)
			}
		}
	}
	return importers
}

func TestTheHarnessHoldsEveryModuleTheGraderImports(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../../e2e"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "..", "..")
	closure := graderClosure(t, root)
	for module, by := range closure {
		switch {
		case slices.Contains(judgeTransport, module):
			for _, importer := range by {
				if importer != "e2e/llm/judge.py" && !slices.Contains(judgeTransport, importer) {
					t.Errorf("%s is exempt as judge transport, but %s imports it, so it grades and belongs in harnessFiles", module, importer)
				}
			}
		case !slices.Contains(harnessFiles, module):
			t.Errorf("%s imports %s, which grades a run but is not in harnessFiles, so a change to it stales no verdict", by, module)
		}
	}
	for _, file := range harnessFiles {
		if _, reached := closure[file]; strings.HasSuffix(file, ".py") && !reached {
			t.Errorf("harnessFiles names %s, which no grader imports: the walk reads fewer imports than the grader makes, or the entry is stale", file)
		}
	}
	for _, module := range judgeTransport {
		if _, reached := closure[module]; !reached {
			t.Errorf("judgeTransport exempts %s, which nothing the grader imports reaches any more", module)
		}
	}
	for _, module := range lintOnly {
		if _, err := os.Stat(filepath.Join(root, module)); err != nil {
			t.Errorf("lintOnly names %s, which does not exist", module)
		}
		if by, reached := closure[module]; reached {
			t.Errorf("%s imports %s, a lint module, which puts a wording rule into every verdict's harness stamp", by, module)
		}
	}
}

// A file that names check.py, or runs it as a module, and passes --lint
// anywhere may run check.py's own lint, which skips the wording rules. The
// rule reads the whole file, so no binding, wrap or env block hides a call.
var (
	namesCheckPy = regexp.MustCompile(`check\.py\b|-m\s+check\b`)
	lintFlag     = regexp.MustCompile(`(?:^|[^\w-])--lint(?:[^\w-]|$)`)
)

func mayRunCheckPysLint(body string) bool {
	return namesCheckPy.MatchString(body) && lintFlag.MatchString(body)
}

func TestTheLintCensusReadsAFileWhole(t *testing.T) {
	for name, body := range map[string]string{
		"a plain command":      "python3 e2e/llm/check.py --lint x.yaml\n",
		"a binding over lines": "CHECK = os.path.join(\n    HERE,\n    \"check.py\",\n)\nrun([CHECK, \"--lint\"])\n",
		"a YAML env":           "env:\n  CHECKER: e2e/llm/check.py\nrun: python3 \"$CHECKER\" --lint x.yaml\n",
		"a module run":         "python3 -m check --lint x.yaml\n",
	} {
		if !mayRunCheckPysLint(body) {
			t.Errorf("%s: %q reads clean, want it caught", name, body)
		}
	}
	for name, body := range map[string]string{
		"the lane's own lint": "python3 e2e/llm/scenariolint.py e2e/llm/scenarios/*.yaml\n",
		"a grade":             "python3 e2e/llm/check.py --check s.yaml run.jsonl\n",
		"a longer flag":       "python3 e2e/llm/check.py --check s.yaml\nlinter --lint-all\n",
		"another module":      "python3 -m checker --lint x\n",
	} {
		if mayRunCheckPysLint(body) {
			t.Errorf("%s: %q is caught, want it clean", name, body)
		}
	}
}

func TestTheLaneLintsThroughScenariolintAlone(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	if err := gatekit.DeclareInputs(os.Getenv, "../../../../scripts", "../../../../docs", "../../../../.github",
		"../../../../e2e", "../../../../Makefile", "../../../Makefile"); err != nil {
		t.Fatal(err)
	}
	// Tracked files only: a __pycache__ or other build leftover is not the
	// tree, and the test cache's digest does not cover it.
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--",
		"scripts", "docs", ".github", "e2e", "Makefile", "backend/Makefile").Output()
	if err != nil {
		t.Fatalf("asking git for the tracked scripts, docs and workflows: %v", err)
	}
	var files []string
	for path := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if filepath.Base(path) == "Makefile" || slices.Contains([]string{".sh", ".md", ".yml", ".yaml", ".py"}, filepath.Ext(path)) {
			files = append(files, path)
		}
	}
	for _, path := range files {
		body, err := os.ReadFile(filepath.Join(root, path)) // #nosec G304 -- a file git tracks in this repository
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) != "check.py" && mayRunCheckPysLint(string(body)) {
			t.Errorf("%s names check.py and --lint: check.py's own lint skips scenariolint.py's wording rules; call e2e/llm/scenariolint.py", path)
		}
	}
	if len(files) < 10 {
		t.Fatalf("read %d files; git listed too little to hold anything", len(files))
	}
}

func TestTheGraderWalkReadsEveryImportForm(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"check.py":     "import json\nimport judge, endstate as ends\n\ndef grade():\n    from casesetup import STEPS\n",
		"judge.py":     "",
		"endstate.py":  "",
		"casesetup.py": "",
	} {
		if err := os.MkdirAll(filepath.Join(root, "e2e", "llm"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "e2e", "llm", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	closure := graderClosure(t, root)
	for _, module := range []string{"e2e/llm/judge.py", "e2e/llm/endstate.py", "e2e/llm/casesetup.py"} {
		if _, reached := closure[module]; !reached {
			t.Errorf("the walk missed %s; got %v", module, closure)
		}
	}
}

func TestTheCatalogsLayoutIsNotToolCopy(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, `{"name":"read_record","description":"Read one record."}`,
		"{\n  \"description\": \"Read one record.\",\n  \"name\": \"read_record\"\n}")
	if got := before["case2_read"].Moved(stampsOf(t, root)["case2_read"], ""); len(got) != 0 {
		t.Errorf("re-indenting the catalog moved %v, want nothing", got)
	}
}

func TestTheRestDoorIsStampedWithWhatItsModelReads(t *testing.T) {
	root := tree(t)
	before := stampsAt(t, root, DoorREST)
	edit(t, root, mcpInfoFile, "Log one thing.", "Log one thing, once.")
	edit(t, root, mcpInfoFile, "Be careful.", "Be very careful.")
	if got := before["case1_log"].Moved(stampsAt(t, root, DoorREST)["case1_log"], ""); len(got) != 0 {
		t.Errorf("MCP copy moved a REST stamp: %v", got)
	}
	for _, tc := range []struct{ file, part string }{
		{"e2e/llm/rest_ops.json", "tool copy"},
		{"e2e/llm/restmap.py", "tool copy"},
		{"backend/internal/compose/agentbundle/SKILL.md.tmpl", "server instructions and resources"},
	} {
		root := tree(t)
		before := stampsAt(t, root, DoorREST)
		edit(t, root, tc.file, "# ", "# edited ")
		if got := before["case1_log"].Moved(stampsAt(t, root, DoorREST)["case1_log"], ""); !reflect.DeepEqual(got, []string{tc.part}) {
			t.Errorf("editing %s moved %v of a REST stamp, want [%s]", tc.file, got, tc.part)
		}
	}
}

// A driver's own parser moves that driver's verdicts and no other's. A change
// to what every driver's transcript passes through moves all of them.
func TestAParserChangeStalesOnlyTheDriverWhoseStreamItReads(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		moved          map[string][]string
	}{
		{"the codex parser", "return 'codex'", "return 'codex2'", map[string][]string{
			"gpt:cli": {"transcript parser"}, "gemini:cli": nil, "claude:cli": nil,
		}},
		{"a helper only the agy parser calls", "return 'agy'", "return 'agy2'", map[string][]string{
			"gpt:cli": nil, "gemini:cli": {"transcript parser"}, "claude:cli": nil,
		}},
		{"what every transcript passes through", "return bare", "return bare + bare", map[string][]string{
			"gpt:cli": {"harness"}, "gemini:cli": {"harness"}, "claude:cli": {"harness"},
		}},
		{"a constant after a parser", "CODEX_ONLY_AFTER = 1", "CODEX_ONLY_AFTER = 2", map[string][]string{
			"gpt:cli": {"harness"}, "gemini:cli": {"harness"}, "claude:cli": {"harness"},
		}},
		{"a constant only codex reads", "_CODEX_SKIPPED = {'a'}", "_CODEX_SKIPPED = {'b'}", map[string][]string{
			"gpt:cli": {"transcript parser"}, "gemini:cli": nil, "claude:cli": nil,
		}},
		{"the comment above the agy parser", "# agy steps", "# agy steps, once", map[string][]string{
			"gpt:cli": nil, "gemini:cli": {"transcript parser"}, "claude:cli": nil,
		}},
		{"a def inside a docstring is not a parser", "not_real", "still_not_real", map[string][]string{
			"gpt:cli": nil, "gemini:cli": {"transcript parser"}, "claude:cli": nil,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := tree(t)
			before := stampsOf(t, root)["case1_log"]
			edit(t, root, transcriptFile, tc.from, tc.to)
			after := stampsOf(t, root)["case1_log"]
			for driver, want := range tc.moved {
				if got := before.Moved(after, driver); !slices.Equal(got, want) {
					t.Errorf("%s verdict: moved = %v, want %v", driver, got, want)
				}
			}
		})
	}
}

// A resource's text is read by the model in the run, so a change to it moves
// every verdict. A resource the server lists with no text to stamp is a
// refusal, not a hole.
func TestAResourceTextChangeStalesEveryVerdict(t *testing.T) {
	root := tree(t)
	before := stampsOf(t, root)
	edit(t, root, mcpInfoFile, `["name","stage"]`, `["name","stage","owner"]`)
	after := stampsOf(t, root)
	for _, name := range []string{"case1_log", "case2_read"} {
		if got := before[name].Moved(after[name], ""); !slices.Equal(got, []string{"server instructions and resources"}) {
			t.Errorf("%s: moved = %v, want the resources", name, got)
		}
	}
	edit(t, root, mcpInfoFile, `"documents":`, `"documents_gone":`)
	if _, err := CurrentStamps(root, DoorMCP); err == nil || !strings.Contains(err.Error(), "no resources") {
		t.Errorf("a surface with no resource text stamped anyway: %v", err)
	}
}

// Every resource the published server lists has its text published beside it.
// The stamp then covers all of them, and a new one cannot slip out of it.
func TestEveryListedResourceHasItsTextInThePublishedSurface(t *testing.T) {
	var info struct {
		Resources struct {
			Resources []struct {
				URI string `json:"uri"`
			} `json:"resources"`
		} `json:"resources"`
		Documents map[string]json.RawMessage `json:"documents"`
	}
	if err := readJSON(filepath.Join("..", "..", "..", "..", mcpInfoFile), &info); err != nil {
		t.Fatal(err)
	}
	if len(info.Resources.Resources) == 0 {
		t.Fatal("the published surface lists no resources, so this reads nothing")
	}
	for _, resource := range info.Resources.Resources {
		if _, published := info.Documents[resource.URI]; !published {
			t.Errorf("%s is listed but has no published text, so a change to it stales no verdict", resource.URI)
		}
	}
}

// A top-level function or constant in the transcript module that names a driver
// belongs to that driver's parser. One left in the shared part would stale every
// driver for a fix only one of them needs. The names come from routeParsers.
func TestEveryDriverSpecificTranscriptNameHasAnOwner(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", transcriptFile))
	if err != nil {
		t.Fatal(err)
	}
	var words []string
	for _, prefixes := range routeParsers {
		for _, prefix := range prefixes {
			words = append(words, strings.Trim(strings.TrimPrefix(prefix, "from_"), "_"))
		}
	}
	owned := 0
	for line := range strings.Lines(string(body)) {
		name, declares := declaredName(line)
		if !declares {
			continue
		}
		named := slices.ContainsFunc(words, func(word string) bool { return strings.Contains(strings.ToLower(name), word) })
		if named && ownerOf(name) == "" {
			t.Errorf("%s names a driver but no route owns it, so it sits in the shared part", name)
		}
		if ownerOf(name) != "" {
			owned++
		}
	}
	if owned == 0 {
		t.Error("no transcript name belongs to a driver, so the split reads nothing")
	}
}

// No name is claimed by two drivers, since the owner would then depend on map order.
func TestNoTwoDriversClaimTheSameTranscriptName(t *testing.T) {
	for driver, prefixes := range routeParsers {
		for other, rivals := range routeParsers {
			for _, prefix := range prefixes {
				for _, rival := range rivals {
					if driver != other && (strings.HasPrefix(prefix, rival) || strings.HasPrefix(rival, prefix)) {
						t.Errorf("%s (%s) and %s (%s) overlap", driver, prefix, other, rival)
					}
				}
			}
		}
	}
}

// A verdict's driver is its candidate and route, and the parser is chosen by
// the route's wire. Every route that reaches a CLI wire has an entry, and every
// entry is a real route, so a new candidate cannot go unstamped.
func TestEveryCommandLineRouteHasAParserEntry(t *testing.T) {
	var table map[string]struct {
		Routes map[string]struct {
			Wire string `json:"wire"`
		} `json:"routes"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", candidatesFile))
	if err != nil {
		t.Fatal(err)
	}
	var whole map[string]json.RawMessage
	if err := json.Unmarshal(raw, &whole); err != nil {
		t.Fatal(err)
	}
	table = map[string]struct {
		Routes map[string]struct {
			Wire string `json:"wire"`
		} `json:"routes"`
	}{}
	for name, entry := range whole {
		var candidate struct {
			Routes map[string]struct {
				Wire string `json:"wire"`
			} `json:"routes"`
		}
		if json.Unmarshal(entry, &candidate) == nil && candidate.Routes != nil {
			table[name] = candidate
		}
	}
	routed := map[string]bool{}
	for name, candidate := range table {
		for via, route := range candidate.Routes {
			if !strings.HasSuffix(route.Wire, "-cli") {
				continue
			}
			routed[name+":"+via] = true
			if _, keyed := routeParsers[name+":"+via]; !keyed {
				t.Errorf("%s:%s reaches the %s wire but routeParsers has no entry, so a parser fix would not stale its verdicts", name, via, route.Wire)
			}
		}
	}
	for driver := range routeParsers {
		if !routed[driver] {
			t.Errorf("routeParsers names %s, which is no command-line route in %s", driver, candidatesFile)
		}
	}
}
