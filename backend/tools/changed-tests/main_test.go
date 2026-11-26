// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// plantedTree writes files into a fresh repository root.
func plantedTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// hunk is the diff of one file that changed `length` lines from `start` on;
// length 0 is lines deleted after `start`.
func hunk(file string, start, length int) string {
	return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\n--- a/%[1]s\n+++ b/%[1]s\n@@ -1,1 +%[2]d,%[3]d @@\n", file, start, length)
}

const unitFile = `package thing

import "testing"

func TestUsesTheHelper(t *testing.T) {
	viaWrapper(t)
}

func TestStandsAlone(t *testing.T) {
	t.Log("alone")
}

// viaWrapper calls the helper, so a change to the helper reaches its callers.
func viaWrapper(t *testing.T) {
	helper(t)
}

func helper(t *testing.T) {
	t.Log("help")
}

type fake struct{ calls int }

func (f *fake) Complete() { f.calls++ }

func TestNamesTheFake(t *testing.T) {
	f := &fake{}
	f.Complete()
}

func Testlowercase(t *testing.T) {}
`

const integrationFile = `//go:build integration

package thing

import "testing"

func TestNeedsTheDatabase(t *testing.T) {
	helper(t)
}
`

const optInFile = `//go:build integration && e2e_llm

package thing

import "testing"

func TestCallsAModel(t *testing.T) {
	helper(t)
}
`

func plannedTests(t *testing.T, root, diff string) []goTest {
	t.Helper()
	changes, err := parseDiff(strings.NewReader(diff))
	if err != nil {
		t.Fatal(err)
	}
	tests, err := changedGoTests(root, changes)
	if err != nil {
		t.Fatal(err)
	}
	return tests
}

func thingTree(t *testing.T) string {
	t.Helper()
	return plantedTree(t, map[string]string{
		"backend/thing/unit_test.go":        unitFile,
		"backend/thing/integration_test.go": integrationFile,
		"backend/thing/model_test.go":       optInFile,
	})
}

func names(tests []goTest) []string {
	var out []string
	for _, test := range tests {
		out = append(out, fmt.Sprintf("%s:%s:%t", test.lane, test.name, test.direct))
	}
	return out
}

func TestAChangedTestFunctionIsChosenByName(t *testing.T) {
	got := names(plannedTests(t, thingTree(t), hunk("backend/thing/unit_test.go", 10, 1)))
	if want := []string{"go-unit:TestStandsAlone:true"}; !slices.Equal(got, want) {
		t.Errorf("a change inside TestStandsAlone chose %v, want %v", got, want)
	}
}

func TestAChangedHelperReachesEveryTestThatCallsItAtAnyDepth(t *testing.T) {
	got := names(plannedTests(t, thingTree(t), hunk("backend/thing/unit_test.go", 19, 1)))
	want := []string{"go-integration:TestNeedsTheDatabase:false", "go-unit:TestUsesTheHelper:false"}
	if !slices.Equal(got, want) {
		t.Errorf("a change inside helper chose %v, want %v: the opt-in lane's test is in neither lane", got, want)
	}
}

func TestAChangedMethodReachesTheTestsThatNameItsType(t *testing.T) {
	got := names(plannedTests(t, thingTree(t), hunk("backend/thing/unit_test.go", 24, 1)))
	if want := []string{"go-unit:TestNamesTheFake:false"}; !slices.Equal(got, want) {
		t.Errorf("a change to a method of fake chose %v, want %v", got, want)
	}
}

func TestADocCommentOrADeletionBetweenDeclarationsReachesNoTest(t *testing.T) {
	root := thingTree(t)
	for _, diff := range []string{
		hunk("backend/thing/unit_test.go", 13, 1),
		hunk("backend/thing/unit_test.go", 11, 0),
	} {
		if got := plannedTests(t, root, diff); len(got) != 0 {
			t.Errorf("%q chose %v, want no test", diff, names(got))
		}
	}
}

func TestLinesDeletedInsideATestChooseIt(t *testing.T) {
	got := names(plannedTests(t, thingTree(t), hunk("backend/thing/unit_test.go", 9, 0)))
	if want := []string{"go-unit:TestStandsAlone:true"}; !slices.Equal(got, want) {
		t.Errorf("lines deleted inside TestStandsAlone chose %v, want %v", got, want)
	}
}

func TestAChangedTestMainReachesEveryTestOfItsPackage(t *testing.T) {
	root := plantedTree(t, map[string]string{
		"backend/thing/main_test.go": "package thing\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) {\n\tm.Run()\n}\n",
		"backend/thing/unit_test.go": unitFile,
	})
	got := plannedTests(t, root, hunk("backend/thing/main_test.go", 6, 1))
	if len(got) != 3 {
		t.Errorf("a change to TestMain chose %v, want the three tests of the package", names(got))
	}
}

func TestOnlyBackendTestFilesAreRead(t *testing.T) {
	root := plantedTree(t, map[string]string{
		"extensions/unit/thing_test.go": unitFile,
		"backend/thing/thing.go":        "package thing\n",
	})
	diff := hunk("extensions/unit/thing_test.go", 10, 1) + hunk("backend/thing/thing.go", 1, 1)
	if got := plannedTests(t, root, diff); len(got) != 0 {
		t.Errorf("a diff of no backend test file chose %v", names(got))
	}
}

func TestVitestTakesChangedTestFilesOfTheDefaultLaneOnly(t *testing.T) {
	root := plantedTree(t, map[string]string{
		"frontend/src/a.test.tsx":             "",
		"frontend/scripts/b.spec.ts":          "",
		"frontend/census/c.test.ts":           "",
		"frontend/e2e/d.test.ts":              "",
		"frontend/src/component.tsx":          "",
		"extensions/unit/frontend/e.test.tsx": "",
	})
	diff := hunk("frontend/src/a.test.tsx", 1, 1) + hunk("frontend/scripts/b.spec.ts", 1, 1) +
		hunk("frontend/census/c.test.ts", 1, 1) + hunk("frontend/e2e/d.test.ts", 1, 1) +
		hunk("frontend/src/component.tsx", 1, 1) + hunk("frontend/src/gone.test.tsx", 1, 1) +
		hunk("extensions/unit/frontend/e.test.tsx", 1, 1)
	changes, err := parseDiff(strings.NewReader(diff))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range changedVitestFiles(root, changes) {
		got = append(got, item.name)
	}
	if want := []string{"scripts/b.spec.ts", "src/a.test.tsx"}; !slices.Equal(got, want) {
		t.Errorf("vitest chose %v, want %v", got, want)
	}
}

func TestTheCountFallsAsTheSelectionGrowsAndStopsAtTheFloor(t *testing.T) {
	b := budget{most: 20, fewest: 3, spend: 100}
	tests := func(n int) []selected {
		out := make([]selected, n)
		for i := range out {
			out[i] = selected{name: fmt.Sprintf("Test%03d", i), cost: 1, direct: i%2 == 1}
		}
		return out
	}
	for _, c := range []struct{ tests, count, kept int }{{1, 20, 1}, {10, 10, 10}, {33, 3, 33}, {40, 3, 33}} {
		count, kept, left := fit(tests(c.tests), b)
		if count != c.count || len(kept) != c.kept || len(kept)+len(left) != c.tests {
			t.Errorf("%d tests: count %d, kept %d, left %d; want count %d, kept %d", c.tests, count, len(kept), len(left), c.count, c.kept)
		}
	}
	_, kept, _ := fit(tests(40), b)
	for _, item := range kept[:20] {
		if !item.direct {
			t.Fatalf("over the budget, %s (reached through a helper) was kept before a changed test", item.name)
		}
	}
}

func TestOneTestOverTheWholeBudgetStillRunsAtTheFloor(t *testing.T) {
	count, kept, left := fit([]selected{{name: "TestSlow", cost: 500}}, budget{most: 10, fewest: 3, spend: 100})
	if count != 3 || len(kept) != 1 || len(left) != 0 {
		t.Errorf("one slow test: count %d, kept %d, left %d; want it kept, 3 times", count, len(kept), len(left))
	}
}

func TestAnIntegrationTestCostsItsTimeThenItsPackageMeanThenAGuess(t *testing.T) {
	timing := filepath.Join(t.TempDir(), "timing.txt")
	body := "# dir|package|Test|seconds\nbackend|./internal/thing|TestSlow|40.5\nbackend|./internal/thing|*|0.2\n"
	if err := os.WriteFile(timing, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	times, err := testTimes(timing)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		dir, name string
		want      float64
	}{
		{"backend/internal/thing", "TestSlow", 40.5},
		{"backend/internal/thing", "TestQuick", 0.2},
		{"backend/internal/other", "TestNew", unknownTestSeconds},
	} {
		if got := integrationCost(times, c.dir, c.name); got != c.want {
			t.Errorf("%s %s costs %v, want %v", c.dir, c.name, got, c.want)
		}
	}
}

func TestThePlanHasOneLinePerPackageAndLaneWithAnAnchoredPattern(t *testing.T) {
	root := thingTree(t)
	diff := hunk("backend/thing/unit_test.go", 19, 1) + hunk("backend/thing/unit_test.go", 10, 1)
	var out, notes bytes.Buffer
	if err := run(strings.NewReader(diff), &out, &notes, root, ""); err != nil {
		t.Fatal(err)
	}
	want := "go-unit\t20\tbackend/thing\t^(TestStandsAlone|TestUsesTheHelper)$\n" +
		"go-integration\t10\tbackend/thing\t^(TestNeedsTheDatabase)$\n"
	if out.String() != want {
		t.Errorf("plan:\n%s\nwant:\n%s", out.String(), want)
	}
	if !strings.Contains(notes.String(), "go-unit: 2 selected, each run 20 times") {
		t.Errorf("notes %q do not say what each lane runs", notes.String())
	}
}

func TestAHunkHeaderWithoutANewSideIsAnError(t *testing.T) {
	if _, err := parseDiff(strings.NewReader("+++ b/backend/x_test.go\n@@ -1 @@\n")); err == nil {
		t.Error("a hunk header with no new side was read without an error")
	}
}
