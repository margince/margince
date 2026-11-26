// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"bytes"
	"cmp"
	"context"
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

var recut = flag.Bool("recut", false, "run every caller of this package and rewrite stylebaseline.txt from what fails")

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
		case !rule.breaks(src, w.Text, ""):
			t.Errorf("%s waives %q under %s, and it now passes: delete the line", baselinePath, w.Text, w.Rule)
		}
		if i > 0 && waiverOrder(waivers[i-1], w) >= 0 {
			t.Errorf("%s is not sorted and unique at %s; re-cut it or move the line", baselinePath, w.line())
		}
	}
}

// TestEveryStyleWaiverNamesASentenceInTheTree fails a waiver whose sentence no
// Go string in the module spells any more, so a rewrite takes its line along.
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
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); path != root && (name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
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
	if files < 1000 {
		t.Fatalf("read %d Go files, and the module holds thousands; the walk is reading the wrong tree", files)
	}
	for _, w := range waivers {
		if !wanted[w.Text] {
			t.Errorf("%s waives %q under %s, and no Go string spells it any more: delete the line",
				baselinePath, w.Text, w.Rule)
		}
	}
}

func waiverOrder(a, b Waiver) int {
	return cmp.Or(strings.Compare(a.Rule, b.Rule), strings.Compare(a.Text, b.Text))
}

// TestRecutStyleBaseline runs the tests that reach this package with every
// finding recorded, then writes the findings as the new baseline.
func TestRecutStyleBaseline(t *testing.T) {
	if !*recut {
		t.Skip("rewrites stylebaseline.txt; run with -recut")
	}
	module, err := filepath.Abs(gatekit.ModuleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	packages, tests := styleCallers(t, module)
	records := filepath.Join(t.TempDir(), "findings")
	command := exec.CommandContext(context.Background(), "go", append([]string{"test", "-count=1",
		"-run", "^(" + strings.Join(tests, "|") + ")$"}, packages...)...)
	command.Dir = module
	command.Env = append(os.Environ(), recordVariable+"="+records)
	if output, err := command.CombinedOutput(); err != nil {
		t.Errorf("the callers' tests failed outside the style check, so some findings may be missing: %v\n%s",
			err, tail(output))
	}
	recorded, err := os.ReadFile(filepath.Clean(records))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	waivers, byPackage := recordedWaivers(t, module, string(recorded))
	writeBaseline(t, filepath.Join(module, "internal/shared/kernel/langcopy/langcopytest/stylebaseline.txt"), waivers)
	t.Logf("%d waivers from %d packages; by package:", len(waivers), len(byPackage))
	for _, dir := range slices.SortedFunc(maps.Keys(byPackage), func(a, b string) int {
		return cmp.Or(byPackage[b]-byPackage[a], strings.Compare(a, b))
	}) {
		t.Logf("  %4d %s", byPackage[dir], dir)
	}
}

func tail(output []byte) []byte {
	const keep = 4000
	if len(output) > keep {
		return output[len(output)-keep:]
	}
	return output
}

func recordedWaivers(t *testing.T, module, recorded string) ([]Waiver, map[string]int) {
	t.Helper()
	unique := map[Waiver]bool{}
	byPackage := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(recorded), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("a recorded finding is not three fields: %q", line)
		}
		text, textErr := strconv.Unquote(fields[1])
		dir, dirErr := strconv.Unquote(fields[2])
		if textErr != nil || dirErr != nil {
			t.Fatalf("a recorded finding does not unquote: %q", line)
		}
		w := Waiver{Rule: fields[0], Text: text}
		if !unique[w] {
			rel, err := filepath.Rel(module, dir)
			if err != nil {
				t.Fatal(err)
			}
			byPackage[filepath.ToSlash(rel)]++
		}
		unique[w] = true
	}
	return slices.SortedFunc(maps.Keys(unique), waiverOrder), byPackage
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
