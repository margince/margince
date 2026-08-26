// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// WHERE the claim sweep looks, as against what it looks for.
//
// The register's number is held per shape in uniquenessclaims_test.go, and that
// prices narrowing the PATTERNS. It leaves the corpus free, and the corpus is
// the cheaper place to make the number fall: drop a walk root, or add a file to
// the skip list, and every per-shape row still agrees — because both sides of
// that comparison derive from the same narrowed walk.
//
// So both doors are checked against the tree rather than against a list.

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// gateIdentifiers are the top-level declarations that make a file part of this
// gate, and so the only thing that earns a file its exemption from the sweep.
var gateIdentifiers = []string{"claimShapes", "namedExhaustiveness", "heldBy", "gateFiles", "shapeCensus"}

// exemptGateFiles is how many files may be exempt from the sweep, pinned.
//
// The count is the guard, and the declaration check below is the reason. A
// skip list validated only by a property of the listed file is circular when
// that property is one the sweep itself looks for: 597 of this tree's sources
// contain a claim shape, so "contains a claim" qualifies every file worth
// exempting, by construction. Pinning the size makes a third exemption a line
// somebody agrees to, the way a shape's row makes a narrowing one.
const exemptGateFiles = 2

func TestOnlyTheGatesOwnSourcesAreExemptFromTheSweep(t *testing.T) {
	if len(gateFiles) != exemptGateFiles {
		t.Fatalf("%d file(s) are exempt from the sweep against a pin of %d.\n\n"+
			"An exemption removes a file's claims from the register without holding or "+
			"deleting one. Adding another is a decision, so it moves this number too.",
			len(gateFiles), exemptGateFiles)
	}
	for _, path := range slices.Sorted(maps.Keys(gateFiles)) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Errorf("%s is exempt from the sweep and cannot be parsed: %v — an exemption naming "+
				"a file that is not there skips nothing and hides a rename", path, err)
			continue
		}
		if declared := gateNamesDeclaredIn(file); len(declared) == 0 {
			t.Errorf("%s is exempt from the sweep and declares none of %s.\n\n"+
				"The exemption is for a source that DECLARES the detector, because such a file "+
				"spells the phrases out and a sweep reading it would register its own prose. "+
				"Merely containing a claim is not that — most of this tree contains one.",
				path, strings.Join(gateIdentifiers, ", "))
		}
	}
}

// gateNamesDeclaredIn returns the gate identifiers a file declares at top
// level.
//
// A DECLARATION, not a mention. Searching the text for `claimShapes` passes any
// source that references the gate — including a comment — and the two exempt
// files reference far more of it than they declare, so a text search would have
// rested on a word an author can type anywhere for free.
func gateNamesDeclaredIn(file *ast.File) []string {
	declared := map[string]bool{}
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			if node.Recv == nil {
				declared[node.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range value.Names {
						declared[name.Name] = true
					}
				}
			}
		}
	}
	var found []string
	for _, name := range gateIdentifiers {
		if declared[name] {
			found = append(found, name)
		}
	}
	return found
}

// TestTheSweptCorpusIsEveryModuleThatCouldCarryAClaim holds the other door.
//
// Every Go module that holds a source somebody could write a claim in must lie
// under a claimed root. A module is derived from its go.mod, and "authored"
// from the same rule the sweep itself uses — a module of generated files is
// legitimately unswept and needs no exemption anybody has to remember.
func TestTheSweptCorpusIsEveryModuleThatCouldCarryAClaim(t *testing.T) {
	const repoRoot = ".."
	rootOf := func(tree claimedTree) string {
		return filepath.Clean(filepath.Join("backend", tree.root))
	}
	covered := func(module string) (string, bool) {
		for _, tree := range claimedTrees {
			root := rootOf(tree)
			if module == root || strings.HasPrefix(module, root+string(filepath.Separator)) {
				return tree.root, true
			}
		}
		return "", false
	}
	perRoot := map[string]int{}
	var unswept []string
	modules := 0
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if unauthoredDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		// A symlink is neither a directory nor a file to this walk, and the
		// claim sweep does not follow one either — so sources behind one are
		// unswept AND invisible to this arm. The tree carries none today;
		// refusing the first is cheaper than discovering it as a blind spot.
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink: neither walk follows one, so anything behind it "+
				"is outside the sweep and outside this check of the sweep", path)
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		module, relErr := filepath.Rel(repoRoot, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		authored, authoredErr := holdsAuthoredGo(filepath.Dir(path))
		if authoredErr != nil {
			return authoredErr
		}
		if !authored {
			return nil
		}
		modules++
		if root, ok := covered(module); ok {
			perRoot[root]++
			return nil
		}
		unswept = append(unswept, module)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s for modules: %v", repoRoot, err)
	}
	roots := make([]string, 0, len(claimedTrees))
	for _, tree := range claimedTrees {
		roots = append(roots, tree.root)
	}
	swept := strings.Join(roots, ", ")
	slices.Sort(unswept)
	for _, module := range unswept {
		t.Errorf("module %s holds authored Go and lies under no claimed root (%s), so a "+
			"uniqueness claim written there needs no gate and takes no register line.\n\n"+
			"Add the root, or say beside claimedTrees why this module cannot carry a claim. "+
			"Removing a root is how the register's number falls without a claim being audited.",
			module, swept)
	}
	// PER ROOT, not against a total. The repo holds roughly twice as many
	// modules as roots, so a floor on the total stays green while a whole tier
	// drops out of the walk — which is the only thing this floor is for.
	for _, tree := range claimedTrees {
		if perRoot[tree.root] == 0 {
			t.Errorf("the walk found no module holding authored Go under %s — a root that "+
				"matches nothing certifies nothing, so either the walk stopped seeing that "+
				"tier or the root is stale", tree.root)
		}
	}
	if modules == 0 {
		t.Error("the walk found no modules at all, so every check above ran over nothing")
	}
}

// holdsAuthoredGo reports whether a module directory holds a source an author
// could have written a claim in, stopping at a nested module so a parent does
// not answer for its child.
func holdsAuthoredGo(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() {
			if authoredGoFile(name) {
				return true, nil
			}
			continue
		}
		if unauthoredDir(name) {
			continue
		}
		nested, nestedErr := declaresItsOwnModule(filepath.Join(dir, name))
		if nestedErr != nil {
			return false, nestedErr
		}
		if nested {
			continue
		}
		holds, holdsErr := holdsAuthoredGo(filepath.Join(dir, name))
		if holdsErr != nil {
			return false, holdsErr
		}
		if holds {
			return true, nil
		}
	}
	return false, nil
}

// declaresItsOwnModule reports whether a directory carries a go.mod FILE.
//
// The file kind is checked, not merely the name's existence: a DIRECTORY named
// go.mod satisfies a bare Stat, and the subtree would then be skipped here as
// "a module answering for itself" while the walk above, which only registers a
// go.mod file, never registers it as a module either — invisible to both.
func declaresItsOwnModule(dir string) (bool, error) {
	info, err := os.Stat(filepath.Join(dir, "go.mod"))
	switch {
	case err == nil:
		return !info.IsDir(), nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// TestTheTwoDoorsAreJudgedRatherThanTrusted holds the branches this tree does
// not currently reach.
//
// No claim binds to a gate arm today and no ordinary source is exempt today,
// so both refusals would go unexercised by the sweep — and a guard the tree
// happens not to reach is a guard with no test.
func TestTheTwoDoorsAreJudgedRatherThanTrusted(t *testing.T) {
	for _, c := range []struct {
		name  string
		paths []string
		want  bool
	}{
		{"one of this gate's arms", []string{"uniquenessclaims_test.go"}, true},
		{"the detector beside it", []string{"uniquenessclaimsdetector_test.go"}, true},
		{"an ordinary gate elsewhere", []string{"internal/modules/deals/pipeline_test.go"}, false},
		{"an arm among ordinary files", []string{"enumsync_test.go", "uniquenessclaims_test.go"}, true},
		{"nothing at all", nil, false},
	} {
		if _, got := declaredInAGateArm(c.paths); got != c.want {
			t.Errorf("declaredInAGateArm(%q) = %v, want %v — %s", c.paths, got, c.want, c.name)
		}
	}

	// The exemption's own reason. A source that CARRIES a claim must not
	// qualify — most of this tree does — and one that merely names the gate must
	// not either. Only declaring it counts.
	for _, c := range []struct {
		name, source string
		want         bool
	}{
		{"declaring the shapes", "package p\n\nvar claimShapes = map[string]int{}\n", true},
		{"declaring the binding", "package p\n\nvar heldBy = 1\n", true},
		{"merely carrying a claim", "package p\n\n// The store is the only writer of that column.\nfunc Write() {}\n", false},
		{"naming the gate in prose", "package p\n\n// See claimShapes in the detector.\nfunc Write() {}\n", false},
		{"using but not declaring it", "package p\n\nfunc use() { _ = claimShapes }\n", false},
		{"an ordinary source", "package p\n\nfunc Total(rows []int) int { return len(rows) }\n", false},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "probe.go", c.source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing the %q probe: %v", c.name, err)
		}
		if got := len(gateNamesDeclaredIn(file)) > 0; got != c.want {
			t.Errorf("a source %s qualified = %v, want %v", c.name, got, c.want)
		}
	}

	// A go.mod DIRECTORY must not read as a module declaration: the walk above
	// registers only a go.mod FILE, so a subtree skipped here on that basis is
	// registered by neither and escapes both.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "go.mod"), 0o750); err != nil {
		t.Fatalf("planting a go.mod directory: %v", err)
	}
	declares, err := declaresItsOwnModule(dir)
	if err != nil {
		t.Fatalf("reading the planted directory: %v", err)
	}
	if declares {
		t.Error("a DIRECTORY named go.mod read as a module declaration — the subtree behind it " +
			"would be skipped as answering for itself while nothing registers it as a module")
	}
}
