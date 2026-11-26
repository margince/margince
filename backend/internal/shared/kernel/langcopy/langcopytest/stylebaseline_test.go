// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var recut = flag.Bool("recut", false, "run every caller of this package and drop the stylebaseline.txt lines nothing reports")

func TestEveryStyleWaiverStillBreaksItsRule(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	src, err := loadSources()
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range waivers {
		rule, known := ruleNamed(w.Rule)
		switch {
		case !known:
			t.Errorf("%s waives %q under %s, which is no rule", baselinePath, w.Text, w.Rule)
		case !rule.breaks(src, w.Text, w.English):
			t.Errorf("%s waives %q under %s, and it now passes: delete the line", baselinePath, w.Text, w.Rule)
		}
		if i > 0 && waiverOrder(waivers[i-1], w) >= 0 {
			t.Errorf("%s is not sorted and unique at %s; re-cut it or move the line", baselinePath, w.line())
		}
	}
}

// TestEveryStyleWaiverNamesASentenceInTheTree fails a waiver whose sentence no
// Go string in the module spells any more, so a rewrite takes its line along.
// A sentence joined from a named constant is not folded and would read as gone.
func TestEveryStyleWaiverNamesASentenceInTheTree(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{}
	for _, w := range waivers {
		wanted[w.Text] = false
	}
	files, root := 0, gatekit.ModuleRoot(t)
	err = gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !sourceFile(root, path) {
			return walkErr
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		ast.Inspect(file, func(node ast.Node) bool {
			switch node.(type) {
			case *ast.BasicLit, *ast.BinaryExpr:
				if text, ok := gatekit.StringExpr(node, nil, gatekit.FoldStrict); ok {
					if _, want := wanted[text]; want {
						wanted[text] = true
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := plainSourceCount(t, root); files != want {
		t.Fatalf("read %d Go files, and a plain walk of the module finds %d; the walk is reading another tree", files, want)
	}
	for _, w := range waivers {
		if !wanted[w.Text] {
			t.Errorf("%s waives %q under %s, and no Go string spells it any more: delete the line",
				baselinePath, w.Text, w.Rule)
		}
	}
}

// sourceFile is a hand-written, non-test Go file of the module.
func sourceFile(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if segment == "testdata" || (segment != "." && strings.HasPrefix(segment, ".")) {
			return false
		}
	}
	return true
}

func plainSourceCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr == nil && !entry.IsDir() && sourceFile(root, path) {
			count++
		}
		return walkErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func waiverOrder(a, b Waiver) int {
	return cmp.Or(strings.Compare(a.Rule, b.Rule), strings.Compare(a.Text, b.Text), strings.Compare(a.English, b.English))
}

// TestRecutStyleBaseline runs the tests that reach this package with every
// finding recorded. It drops the baseline lines nothing reported, and fails on
// a finding the baseline does not hold, which a rewrite must fix instead.
func TestRecutStyleBaseline(t *testing.T) {
	if !*recut {
		t.Skip("drops stale lines from stylebaseline.txt; run with -recut")
	}
	module, err := filepath.Abs(gatekit.ModuleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	packages, tests := styleCallers(t, module)
	records, token := filepath.Join(t.TempDir(), "findings"), rand.Text()
	if err := os.WriteFile(records, []byte(recordHeader+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), "go", append([]string{"test", "-count=1",
		"-run", "^(" + strings.Join(tests, "|") + ")$"}, packages...)...)
	command.Dir = module
	command.Env = append(os.Environ(), recordVariable+"="+records, tokenVariable+"="+token)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("the callers' tests failed outside the style check, so the findings are not complete: %v\n%s",
			err, tail(output))
	}
	recorded, err := os.ReadFile(filepath.Clean(records))
	if err != nil {
		t.Fatal(err)
	}
	found := recordedWaivers(t, module, string(recorded))
	baseline, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	kept, byPackage := shrunk(baseline, found)
	for _, w := range slices.SortedFunc(maps.Keys(found), waiverOrder) {
		if !slices.Contains(baseline, w) {
			t.Errorf("%s finds %s, which the baseline does not hold; rewrite the sentence", found[w], w.line())
		}
	}
	writeBaseline(t, filepath.Join(module, "internal/shared/kernel/langcopy/langcopytest/stylebaseline.txt"), kept)
	t.Logf("%d of %d waivers still found, from %d packages:", len(kept), len(baseline), len(byPackage))
	for _, dir := range slices.SortedFunc(maps.Keys(byPackage), func(a, b string) int {
		return cmp.Or(byPackage[b]-byPackage[a], strings.Compare(a, b))
	}) {
		t.Logf("  %4d %s", byPackage[dir], dir)
	}
}

// shrunk keeps the baseline lines a recut found again, in baseline order, and
// counts them by the package that found each.
func shrunk(baseline []Waiver, found map[Waiver]string) ([]Waiver, map[string]int) {
	var kept []Waiver
	byPackage := map[string]int{}
	for _, w := range baseline {
		if dir, still := found[w]; still {
			kept = append(kept, w)
			byPackage[dir]++
		}
	}
	return kept, byPackage
}

func tail(output []byte) []byte {
	const keep = 4000
	if len(output) > keep {
		return output[len(output)-keep:]
	}
	return output
}

// recordedWaivers answers each recorded finding and the first package, by
// sorted path, that found it.
func recordedWaivers(t *testing.T, module, recorded string) map[Waiver]string {
	t.Helper()
	found := map[Waiver]string{}
	for _, line := range strings.Split(strings.TrimSpace(recorded), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		quotedDir, rest, cut := strings.Cut(line, "\t")
		dir, dirErr := strconv.Unquote(quotedDir)
		w, err := parseWaiver(rest)
		if !cut || dirErr != nil || err != nil {
			t.Fatalf("a recorded finding is not a quoted directory and a baseline line: %q", line)
		}
		rel, err := filepath.Rel(module, dir)
		if err != nil {
			t.Fatal(err)
		}
		if first, seen := found[w]; !seen || filepath.ToSlash(rel) < first {
			found[w] = filepath.ToSlash(rel)
		}
	}
	return found
}

func writeBaseline(t *testing.T, path string, waivers []Waiver) {
	t.Helper()
	var out bytes.Buffer
	for _, line := range strings.Split(baselineText, "\n") {
		if !strings.HasPrefix(line, "#") {
			break
		}
		out.WriteString(line + "\n")
	}
	for _, w := range waivers {
		out.WriteString(w.line() + "\n")
	}
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

const langcopytestImport = "github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"

// styleCallers finds every package whose tests import this one, and the
// top-level tests there that reach it directly or through a helper.
func styleCallers(t *testing.T, module string) (packages, tests []string) {
	t.Helper()
	files := map[string][]*ast.File{}
	err := filepath.WalkDir(module, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); path != module && (strings.HasPrefix(name, ".") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files[filepath.Dir(path)] = append(files[filepath.Dir(path)], file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir, parsed := range files {
		reaching := testsReaching(parsed)
		if dir == here || len(reaching) == 0 {
			continue
		}
		rel, err := filepath.Rel(module, dir)
		if err != nil {
			t.Fatal(err)
		}
		packages = append(packages, "./"+filepath.ToSlash(rel))
		tests = append(tests, reaching...)
	}
	if len(packages) == 0 {
		t.Fatal("no package's tests reach langcopytest; the walk is reading the wrong tree")
	}
	slices.Sort(packages)
	slices.Sort(tests)
	return packages, slices.Compact(tests)
}

// testsReaching names the Test functions of one package that call into
// langcopytest, following calls to the package's own test helpers.
func testsReaching(files []*ast.File) []string {
	reaches := map[string]bool{}
	calls := map[string][]string{}
	for _, file := range files {
		local := importedAs(file)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.SelectorExpr:
					if x, ok := n.X.(*ast.Ident); ok && local != "" && x.Name == local {
						reaches[fn.Name.Name] = true
					}
				case *ast.Ident:
					calls[fn.Name.Name] = append(calls[fn.Name.Name], n.Name)
				}
				return true
			})
		}
	}
	for grew := true; grew; {
		grew = false
		for caller, callees := range calls {
			if !reaches[caller] && slices.ContainsFunc(callees, func(name string) bool { return reaches[name] }) {
				reaches[caller], grew = true, true
			}
		}
	}
	var tests []string
	for name := range reaches {
		if strings.HasPrefix(name, "Test") {
			tests = append(tests, name)
		}
	}
	return tests
}

func importedAs(file *ast.File) string {
	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err == nil && path == langcopytestImport {
			if spec.Name != nil {
				return spec.Name.Name
			}
			return "langcopytest"
		}
	}
	return ""
}
