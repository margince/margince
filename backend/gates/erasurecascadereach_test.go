// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every file the Art. 17 cascade executes SQL from is one the PII censuses read.
//
// erasureCascadeFiles is hand-maintained, and it has to be: a call graph alone
// over-reaches into the retention engine through a helper both share, and
// letting a retention sweep answer for a subject's erasure request is the
// confusion those censuses exist to prevent.
//
// What a hand-maintained list cannot do is notice its own omissions. Twice a
// scrub moved to a neighbouring file and the list stayed as it was; the censuses
// then reported coverage over a smaller cascade and said the same word for it,
// which is the one way a census must not break. Two tables — activity and
// deal_room_participant — were purged by files no census had ever parsed.
//
// So the reach is DERIVED and the list is judged against it. Every file
// ErasePerson can reach that executes SQL is either in the cascade or in the
// register that says why it is not.
func TestEveryFileTheCascadeExecutesSQLFromIsCensused(t *testing.T) {
	t.Parallel()
	reached := filesReachableFrom(t, "internal/modules/privacy", "ErasePerson")

	cascade := map[string]bool{}
	for _, path := range erasureCascadeFiles {
		cascade[path] = true
	}
	var unaccounted []string
	for _, path := range sortedStrings(reached) {
		if cascade[path] || reachedButNotCascade.Waived(t, path) {
			continue
		}
		if !executesSQL(t, path) {
			continue
		}
		unaccounted = append(unaccounted, path)
	}
	if len(unaccounted) > 0 {
		t.Errorf("ErasePerson reaches %d file(s) that execute SQL and no census reads:\n\t%s\n\n"+
			"Add each to erasureCascadeFiles, or to reachedButNotCascade with the reason it is not "+
			"part of the cascade. A file left off is a table whose columns no census asks about, "+
			"reported as covered.", len(unaccounted), strings.Join(unaccounted, "\n\t"))
	}

	// AND THE LIST IS NOT STALE THE OTHER WAY. A path that names nothing the
	// cascade reaches is a census parsing a file for no reason, and the next
	// reader takes it for evidence that the cascade still goes there.
	for _, path := range erasureCascadeFiles {
		if !reached[path] {
			t.Errorf("erasureCascadeFiles names %s, which ErasePerson cannot reach — either the "+
				"cascade stopped calling it or this list outlived the call", path)
		}
	}
	// And a register entry describing a file the cascade no longer reaches is
	// itself a claim nobody checks.
	reachedButNotCascade.AssertAllMatched(t)
	// The floor: a walk that reached nothing would pass both loops above in
	// silence.
	if len(reached) < 10 {
		t.Fatalf("the call graph from ErasePerson reached %d file(s), want at least ten — the walk "+
			"has stopped following the cascade and this census is judging almost nothing", len(reached))
	}
}

// filesReachableFrom walks one package's call graph from entry and answers the
// files it lands in.
//
// Within the package only: a call into another module is that module's own
// business, and the censuses this serves are about the privacy package's SQL.
// Method calls are followed by NAME, which over-reaches rather than under —
// two methods sharing a name pull in both files, and the register above is where
// a file that turns out not to belong is accounted for.
func filesReachableFrom(t *testing.T, dir, entry string) map[string]bool {
	t.Helper()
	// File by file rather than parser.ParseDir, which is deprecated for a
	// reason that bites here: it associates files with packages without reading
	// build tags. This walk wants every non-test source in the directory, which
	// is a simpler question and one a directory read answers exactly.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	fileOf := map[string]string{}
	declOf := map[string]*ast.FuncDecl{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join(dir, name))
		parsed, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		for _, decl := range parsed.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn {
				continue
			}
			fileOf[fn.Name.Name] = path
			declOf[fn.Name.Name] = fn
		}
	}
	if declOf[entry] == nil {
		t.Fatalf("%s declares no %s — this census would walk nothing and pass", dir, entry)
	}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		fn := declOf[name]
		if fn == nil {
			return
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				walk(fun.Name)
			case *ast.SelectorExpr:
				walk(fun.Sel.Name)
			}
			return true
		})
	}
	walk(entry)

	out := map[string]bool{}
	for name := range seen {
		if path, known := fileOf[name]; known {
			out[path] = true
		}
	}
	return out
}

// executesSQL reports whether a file hands a statement to the database itself.
// A file that only computes — a cutoff, a set of ids — writes nothing this
// census has anything to say about.
func executesSQL(t *testing.T, path string) bool {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := string(body)
	for _, call := range []string{".Exec(", ".Query(", ".QueryRow(", ".SendBatch(", ".CopyFrom("} {
		if strings.Contains(text, call) {
			return true
		}
	}
	return false
}

func sortedStrings(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
