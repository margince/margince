// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"bytes"
	"cmp"
	"flag"
	"go/ast"
	"go/parser"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var recut = flag.Bool("recut", false, "drop the stylebaseline.txt lines whose sentence passes its rule or is gone")

func TestEveryStyleWaiverStillBreaksItsRule(t *testing.T) {
	t.Parallel()
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	passing := waiversThatPass(t, waivers)
	for i, w := range waivers {
		switch _, known := ruleNamed(w.Rule); {
		case !known:
			t.Errorf("%s waives %q under %s, which is no rule", baselinePath, w.Text, w.Rule)
		case passing[w]:
			t.Errorf("%s waives %q under %s, and it now passes: delete the line", baselinePath, w.Text, w.Rule)
		}
		if i > 0 && waiverOrder(waivers[i-1], w) >= 0 {
			t.Errorf("%s is not sorted and unique at %s; move the line", baselinePath, w.line())
		}
	}
}

// waiversThatPass answers the waivers whose sentence no longer breaks their
// known rule.
func waiversThatPass(t *testing.T, waivers []Waiver) map[Waiver]bool {
	t.Helper()
	src, err := loadSources()
	if err != nil {
		t.Fatal(err)
	}
	passing := map[Waiver]bool{}
	for _, w := range waivers {
		if rule, known := ruleNamed(w.Rule); known && !rule.breaks(src, w.Text, w.English) {
			passing[w] = true
		}
	}
	return passing
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
	spelled := sentencesInTheTree(t, waivers)
	for _, w := range waivers {
		if !spelled[w.Text] {
			t.Errorf("%s waives %q under %s, and no Go string spells it any more: delete the line",
				baselinePath, w.Text, w.Rule)
		}
	}
}

// sentencesInTheTree answers which waived sentences a Go string of the module
// spells.
func sentencesInTheTree(t *testing.T, waivers []Waiver) map[string]bool {
	t.Helper()
	wanted := map[string]bool{}
	for _, w := range waivers {
		wanted[w.Text] = false
	}
	files, root := 0, gatekit.ModuleRoot(t)
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
	return wanted
}

// sourceFile is a hand-written, non-test Go file of the module.
func sourceFile(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(filepath.Dir(rel)), "/") {
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

// TestRecutStyleBaseline drops the lines the two tests above fail for a
// sentence that passes or is gone. It never adds a line.
func TestRecutStyleBaseline(t *testing.T) {
	if !*recut {
		t.Skip("drops stale lines from stylebaseline.txt; run with -recut")
	}
	waivers, err := StyleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	passing, spelled := waiversThatPass(t, waivers), sentencesInTheTree(t, waivers)
	var kept []Waiver
	for _, w := range waivers {
		if !passing[w] && spelled[w.Text] {
			kept = append(kept, w)
		}
	}
	writeBaseline(t, "stylebaseline.txt", kept)
	t.Logf("kept %d of %d waivers", len(kept), len(waivers))
}

func writeBaseline(t *testing.T, path string, waivers []Waiver) {
	t.Helper()
	var out bytes.Buffer
	for line := range strings.SplitSeq(baselineText, "\n") {
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
