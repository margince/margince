// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The boundary rules between packages hold as a plain `go test`, with the
// package list derived from the tree so a new package is enrolled the moment it
// exists. depguard and
// go-arch-lint cover the same rules as lint gates; this covers them as a
// plain `go test` no contributor can skip.

import (
	"go/ast"
	"go/build"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const modulePath = "github.com/margince/margince/backend"

// packagesUnder walks root and returns the import-path-relative directory
// of every Go package beneath it (root itself included when it holds Go
// files). Vendor-less repo: every *.go directory is a package.
func packagesUnder(t *testing.T, root string) []string {
	t.Helper()
	seen := map[string]bool{}
	var pkgs []string
	err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if !seen[dir] {
			seen[dir] = true
			pkgs = append(pkgs, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return pkgs
}

// projectImports resolves a package directory and returns its non-stdlib
// imports (tests included — a test-only forbidden edge is still a
// forbidden edge).
func projectImports(t *testing.T, dir string) []string {
	t.Helper()
	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		if _, ok := err.(*build.NoGoError); ok {
			return nil
		}
		t.Fatalf("resolving %s: %v", dir, err)
	}
	var out []string
	for _, group := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range group {
			if strings.Contains(imp, ".") {
				out = append(out, imp)
			}
		}
	}
	return out
}

// TestPlatformOwnsNoDomain: internal/platform is technical plumbing
// (ADR-0054 §5) — it may import shared and other platform packages, but
// never a domain module, the composition layer, or a process role.
func TestPlatformOwnsNoDomain(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		modulePath + "/internal/modules/",
		modulePath + "/internal/compose",
		modulePath + "/cmd/",
	}
	for _, dir := range packagesUnder(t, "internal/platform") {
		for _, imp := range projectImports(t, dir) {
			for _, bad := range forbidden {
				if strings.HasPrefix(imp, bad) {
					t.Errorf("%s imports %s: platform owns no domain", dir, imp)
				}
			}
		}
	}
}

// TestNoSiblingModuleImports: a module reaches another module's behavior
// only through a shared/ports seam or a dependency injected by the
// composition layer (ADR-0054 §9) — never by importing the sibling. The
// module list is derived from the tree, so a new module is enrolled the
// moment its directory exists.
func TestNoSiblingModuleImports(t *testing.T) {
	t.Parallel()
	modules, err := gatekit.Glob("internal/modules/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, mod := range modules {
		modImportPrefix := modulePath + "/" + filepath.ToSlash(mod)
		for _, dir := range packagesUnder(t, mod) {
			for _, imp := range projectImports(t, dir) {
				if !strings.HasPrefix(imp, modulePath+"/internal/modules/") {
					continue
				}
				if imp == modImportPrefix || strings.HasPrefix(imp, modImportPrefix+"/") {
					continue // a module may import its own subpackages
				}
				t.Errorf("%s imports %s: modules never import a sibling module", dir, imp)
			}
		}
	}
}

// TestSharedIsPure: internal/shared (kernel + apperrors + ports) is the
// Tier-0 leaf layer — stdlib, each other, and the published extension
// surface beneath it (ports alias published seam types so a core module
// and an extension register the same type, ADR-0120 §3). Anything else
// is an architecture defect.
// sharedUnicodeForms is the one outside package shared may use: values composes
// an identifier to NFC, which the stdlib cannot.
const sharedUnicodeForms = "golang.org/x/text/unicode/norm"

func TestSharedIsPure(t *testing.T) {
	t.Parallel()
	for _, dir := range packagesUnder(t, "internal/shared") {
		for _, imp := range projectImports(t, dir) {
			if strings.HasPrefix(imp, modulePath+"/internal/shared/") {
				continue // leaf → leaf is allowed (types only, no cycles)
			}
			if strings.HasPrefix(imp, modulePath+"/pkg/") {
				continue // the published surface sits below shared
			}
			if imp == sharedUnicodeForms && strings.HasSuffix(dir, "/kernel/values") {
				continue
			}
			t.Errorf("%s imports %s: shared packages must be stdlib-only", dir, imp)
		}
	}
}

// TestPublishedSurfaceIsPure: backend/pkg is the published extension
// surface (ADR-0120 §3) and the outermost leaf — stdlib and itself,
// nothing else. Any other import would become transitively-frozen
// published API the moment an extension binds it.
func TestPublishedSurfaceIsPure(t *testing.T) {
	t.Parallel()
	for _, dir := range packagesUnder(t, "pkg") {
		for _, imp := range projectImports(t, dir) {
			if strings.HasPrefix(imp, modulePath+"/pkg/") {
				continue
			}
			t.Errorf("%s imports %s: published surface packages must be stdlib-only", dir, imp)
		}
	}
}

// mintedModelTypes are the results that make an exported function of
// internal/modules/ai a constructor: a router, the fake, or a raw client of
// either wire.
var mintedModelTypes = map[string]bool{"*Router": true, "*FakeClient": true, "model.Client": true, "decision.Client": true}

// modelClientConstructors is every exported function of internal/modules/ai
// that mints a model client or a router in front of one. It is read from the
// package, so a new constructor is held the moment it exists. That package is
// the one gate, and modelPathAssemblySeam names the only other callers.
// Held by: TestNoModelClientOutsideTheGate (backend/gates/arch_test.go).
func modelClientConstructors(t *testing.T) map[string]bool {
	t.Helper()
	dir := "internal/modules/ai"
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	constructors := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(dir, name), 0)
		if err != nil {
			t.Fatalf("parsing %s/%s: %v", dir, name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || fn.Type.Results == nil {
				continue
			}
			for _, result := range fn.Type.Results.List {
				if mintedModelTypes[types.ExprString(result.Type)] {
					constructors[fn.Name.Name] = true
					break
				}
			}
		}
	}
	// The production constructor is a minted router; a census without it read
	// the package wrong and would pass every file.
	if !constructors["NewRouter"] {
		t.Fatalf("the constructor census of %s is %v and misses NewRouter", dir, constructors)
	}
	return constructors
}

// aiModulePath is the import path the constructors are reached through.
const aiModulePath = modulePath + "/internal/modules/ai"

// constructorUses names each constructor file reaches, under whatever name it
// imports the ai package by, as a call or as a function value.
func constructorUses(t *testing.T, path string, constructors map[string]bool) []string {
	t.Helper()
	file, err := gatekit.ParseFile(path, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	local := ""
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != aiModulePath {
			continue
		}
		local = "ai"
		if imp.Name != nil {
			local = imp.Name.Name
		}
	}
	switch local {
	case "", "_":
		return nil
	case ".":
		return []string{"a dot import of internal/modules/ai"}
	}
	var uses []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == local && constructors[sel.Sel.Name] {
			uses = append(uses, "ai."+sel.Sel.Name)
		}
		return true
	})
	return uses
}

// modelPathAssemblySeam: the only non-test, non-ai-module files allowed to
// call a modelClientConstructor. This is enumerated, not tree-derived,
// because the invariant it encodes — "these two files ARE the seam" — is
// a statement about what the files DO, not where they sit; nothing about
// their location distinguishes them from any other file in
// internal/compose. Growing this list is itself the review signal: a
// third entry is a second gate and should be rejected on sight, not waved
// through because the rule technically allows appending to it.
var modelPathAssemblySeam = map[string]bool{
	"internal/compose/brain.go":         true,
	"internal/compose/sitereaddebug.go": true,
}

// TestNoModelClientOutsideTheGate: walks every hand-written, non-test Go
// file and fails if it constructs a model client outside the seam above,
// or if a cmd/ process role calls compose.NewLocalModelPath — that
// assembly is DB-less by design (siteread debug, tests); a production
// role has a pool and belongs on the metered compose.NewModelPath.
func TestNoModelClientOutsideTheGate(t *testing.T) {
	t.Parallel()
	constructors := modelClientConstructors(t)
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if strings.HasPrefix(rel, "internal/modules/ai/") {
			return nil // the gate's own module: constructing clients here is its job
		}
		src, err := gatekit.ReadFile(path) // #nosec G304 G122 -- path is a *.go file from walking the trusted source tree
		if err != nil {
			return err
		}
		text := string(src)

		if !modelPathAssemblySeam[rel] {
			for _, use := range constructorUses(t, path, constructors) {
				t.Errorf("%s: uses %s outside the compose gate — only internal/compose/brain.go and internal/compose/sitereaddebug.go may construct a model client directly", rel, use)
			}
		}

		if strings.HasPrefix(rel, "cmd/") && strings.Contains(text, "compose.NewLocalModelPath(") {
			t.Errorf("%s: a cmd/ process role calls compose.NewLocalModelPath — that seam is DB-less by design; production roles resolve through the metered compose.NewModelPath", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking backend tree: %v", err)
	}
}

// funcsCallingSelector returns, for every top-level function declared in
// the non-test Go files directly under dir, the "file:func" label of each
// function whose body contains a call shaped like pkgIdent.funcName
// (e.g. "compose.NewModelPath"). It counts distinct FUNCTIONS, not raw
// call-expression occurrences: a function is allowed to branch to more
// than one config (modelwiring.go's resolveModelPath calls
// compose.NewModelPath from both its routing-file and --ai-fake arms) and
// still be the package's single decision point — what the fitness rule
// below guards against is a SECOND function growing its own competing
// resolution.
func funcsCallingSelector(t *testing.T, dir, pkgIdent, funcName string) []string {
	t.Helper()
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var sites []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			calls := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if ok && ident.Name == pkgIdent && sel.Sel.Name == funcName {
					calls = true
				}
				return true
			})
			if calls {
				sites = append(sites, filepath.ToSlash(path)+":"+fn.Name.Name)
			}
		}
	}
	return sites
}

// TestOneModelPathPerRole: each cmd/<role> process resolves its AI
// surfaces through exactly one function that calls compose.NewModelPath —
// the single decision modelwiring.go's resolveModelPath documents ("a
// process holds one Router, one cache, one budget — never a doubled pair
// from two callers each resolving their own"). A second function calling
// it would be a second, competing resolution point the role never
// intended to grow. The role list is derived from the tree, so a new
// cmd/<role> is enrolled the moment its directory exists.
func TestOneModelPathPerRole(t *testing.T) {
	t.Parallel()
	roles, err := gatekit.Glob("cmd/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		info, err := os.Stat(role)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			continue
		}
		sites := funcsCallingSelector(t, role, "compose", "NewModelPath")
		if len(sites) > 1 {
			t.Errorf("%s: %d functions call compose.NewModelPath (%s) — a role resolves its model path in exactly one place", role, len(sites), strings.Join(sites, ", "))
		}
	}
}
