// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The backend module type-checked from source as one universe. Each module
// package imports the others as checked from source, and its outside imports
// from compiled export data through one shared importer. So a type means the
// same object in every package, and types.Implements and types.Identical hold
// across packages.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// typedPackage is one package of this module, checked.
type typedPackage struct {
	path string
	// dir is the package's directory, spelled from the module root.
	dir   string
	files []*ast.File
	info  *types.Info
	types *types.Package
}

type listedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	Imports    []string
	Export     string
	Module     *struct{ Path string }
}

// typeCheckWorkers bounds how many packages are checked at once, which bounds
// the memory the parsed and checked module holds in flight.
const typeCheckWorkers = 4

var typedModuleOnce = sync.OnceValues(func() ([]typedPackage, error) { return loadTypedModule("", "./...") })

// typedTreeTags are the build tags the tree's hand-written files use.
// typedTreeUnder fails a file that no build with these tags reads.
const typedTreeTags = "integration,loadrig"

var typedTreeOnce = sync.OnceValues(func() ([]typedPackage, error) {
	return loadTypedModule(typedTreeTags, "./...", "./tools/...")
})

// gatekitWorkingDir is the module root TestMain moved to.
var gatekitWorkingDir = sync.OnceValue(func() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
})

// typedModule holds the non-test packages of this module, built once per
// test binary.
func typedModule(t *testing.T) []typedPackage {
	t.Helper()
	return typedOrFatal(t, "the module", typedModuleOnce)
}

// typedTree is typedModule built with typedTreeTags, the tools module
// included, once per test binary. A caller must not write into what it holds.
func typedTree(t *testing.T) []typedPackage {
	t.Helper()
	return typedOrFatal(t, "the tree with tags "+typedTreeTags, typedTreeOnce)
}

func typedOrFatal(t *testing.T, what string, load func() ([]typedPackage, error)) []typedPackage {
	t.Helper()
	pkgs, err := load()
	if err != nil {
		t.Fatalf("type-checking %s: %v", what, err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("type-checked no package of %s, and an empty module agrees with every census", what)
	}
	return pkgs
}

// typedTreeUnder is the packages of tree under dir. It fails on a hand-written
// file there that tree does not hold, unless only a build without
// typedTreeTags reads it. That file comes back in shippedOnly.
func typedTreeUnder(t *testing.T, tree []typedPackage, dir string) (pkgs []typedPackage, shippedOnly []string) {
	t.Helper()
	held := map[string]bool{}
	for _, pkg := range tree {
		if dir == "." || pkg.dir == dir || strings.HasPrefix(pkg.dir, dir+"/") {
			pkgs = append(pkgs, pkg)
		}
		for _, file := range pkg.files {
			held[gatekit.SourceFileSet().Position(file.Pos()).Filename] = true
		}
	}
	paths, err := goFilesUnder(dir)
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	for _, path := range paths {
		if held[path] {
			continue
		}
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			t.Errorf("parsing %s: %v", path, err)
			continue
		}
		switch shipped, err := onlyShippedBuildReads(file); {
		case err != nil:
			t.Errorf("reading the build constraint of %s: %v", path, err)
		case shipped:
			shippedOnly = append(shippedOnly, path)
		default:
			t.Errorf("%s is in no package the typed tree built with tags %s, so no gate reading that tree judges it. "+
				"Add its build tag to typedTreeTags, or make go list see its package.", path, typedTreeTags)
		}
	}
	return pkgs, shippedOnly
}

// typedCallGraphUnder is typedTreeUnder for a call graph, which holds one
// declaration per name. It reads the tagged twin of a shipped-only file, so it
// fails on a shipped-only file that runs code the twin may not.
func typedCallGraphUnder(t *testing.T, dir string) []typedPackage {
	t.Helper()
	pkgs, shippedOnly := typedTreeUnder(t, typedTree(t), dir)
	for _, path := range shippedOnly {
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			t.Errorf("parsing %s: %v", path, err)
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			_, isCall := n.(*ast.CallExpr)
			body, isBody := n.(*ast.BlockStmt)
			if isCall || (isBody && len(body.List) > 0) {
				t.Errorf("%s: only the shipped build reads this file, and it runs code. A call graph over the typed tree "+
					"reads its tagged twin in its place, so judge typedModule as well.", gatekit.SourceFileSet().Position(n.Pos()))
				return false
			}
			return true
		})
	}
	return pkgs
}

// onlyShippedBuildReads reports whether file's build constraint holds with no
// tag set and fails with typedTreeTags. Such a file is in the shipped build,
// and the typed tree reads its tagged twin instead.
func onlyShippedBuildReads(file *ast.File) (bool, error) {
	tagged := map[string]bool{}
	for tag := range strings.SplitSeq(typedTreeTags, ",") {
		tagged[tag] = true
	}
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, line := range group.List {
			if !constraint.IsGoBuild(line.Text) {
				continue
			}
			expr, err := constraint.Parse(line.Text)
			if err != nil {
				return false, err
			}
			return expr.Eval(func(string) bool { return false }) && !expr.Eval(func(tag string) bool { return tagged[tag] }), nil
		}
	}
	return false, nil
}

// loadTypedModule checks the packages patterns name under tags, a comma list
// that may be empty.
func loadTypedModule(tags string, patterns ...string) ([]typedPackage, error) {
	listed, err := listModule(tags, patterns)
	if err != nil {
		return nil, err
	}
	universe := newModuleUniverse(listed)
	var roots []listedPackage
	for _, pkg := range listed {
		if universe.fromSource(pkg.ImportPath) {
			roots = append(roots, pkg)
		}
	}
	out := make([]typedPackage, len(roots))
	errs := make([]error, len(roots))
	slots := make(chan struct{}, typeCheckWorkers)
	var wg sync.WaitGroup
	for i, pkg := range roots {
		wg.Go(func() {
			// A package waits for the module packages it imports, so each is
			// checked once and every importer sees that one check.
			for _, dep := range pkg.Imports {
				universe.wait(dep)
			}
			slots <- struct{}{}
			out[i], errs[i] = checkListedPackage(pkg, universe)
			<-slots
			universe.finish(pkg.ImportPath, out[i], errs[i])
		})
	}
	wg.Wait()
	var module []typedPackage
	for i, pkg := range roots {
		if inBackendTree(pkg) {
			module = append(module, out[i])
		}
	}
	return module, errors.Join(errs...)
}

// moduleUniverse answers an import with the module package checked from
// source, or with the outside package read from export data.
type moduleUniverse struct {
	done    map[string]chan struct{}
	checked sync.Map // import path to *types.Package
	mu      sync.Mutex
	outside types.Importer
}

// newModuleUniverse checks from source each backend package and each other
// workspace package that imports one. Export data of such a package would name
// its own copy of the backend types. go list lists a package after its
// imports, so one pass finds them all.
func newModuleUniverse(listed []listedPackage) *moduleUniverse {
	exports := map[string]string{}
	u := &moduleUniverse{done: map[string]chan struct{}{}}
	for _, pkg := range listed {
		exports[pkg.ImportPath] = pkg.Export
		reaches := inBackendTree(pkg)
		for _, dep := range pkg.Imports {
			reaches = reaches || u.fromSource(dep)
		}
		if reaches && len(pkg.GoFiles) > 0 {
			u.done[pkg.ImportPath] = make(chan struct{})
		}
	}
	u.outside = importer.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok || file == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
	return u
}

func (u *moduleUniverse) fromSource(path string) bool {
	_, ok := u.done[path]
	return ok
}

// inBackendTree holds for a package of this module or of its tools module.
func inBackendTree(pkg listedPackage) bool {
	return pkg.Module != nil && (pkg.Module.Path == modulePath || pkg.Module.Path == modulePath+"/tools") && len(pkg.GoFiles) > 0
}

func (u *moduleUniverse) wait(path string) {
	if done, ok := u.done[path]; ok {
		<-done
	}
}

func (u *moduleUniverse) finish(path string, pkg typedPackage, err error) {
	if err == nil {
		u.checked.Store(path, pkg.types)
	}
	close(u.done[path])
}

func (u *moduleUniverse) Import(path string) (*types.Package, error) {
	if _, ok := u.done[path]; ok {
		u.wait(path)
		pkg, ok := u.checked.Load(path)
		if !ok {
			return nil, fmt.Errorf("%s did not type-check", path)
		}
		typed, ok := pkg.(*types.Package)
		if !ok {
			return nil, fmt.Errorf("%s holds no package", path)
		}
		return typed, nil
	}
	// The export-data importer caches what it reads and is not safe to share.
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.outside.Import(path)
}

// listModule asks the go command for every package patterns name and their
// dependencies, with the export data a build left for each.
func listModule(tags string, patterns []string) ([]listedPackage, error) {
	args := []string{"list", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,Imports,Export,Module"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	cmd := exec.Command("go", append(args, patterns...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var listed []listedPackage
	for decoder := json.NewDecoder(stdout); ; {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("reading go list: %w", err)
		}
		listed = append(listed, pkg)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("go list -export: %w\n%s", err, stderr.String())
	}
	return listed, nil
}

func checkListedPackage(pkg listedPackage, imports types.Importer) (typedPackage, error) {
	// Spelled from the module root, as the other gates spell it, so the parse
	// cache holds each file once.
	dir, err := filepath.Rel(gatekitWorkingDir(), pkg.Dir)
	if err != nil {
		return typedPackage{}, err
	}
	files := make([]*ast.File, 0, len(pkg.GoFiles))
	for _, name := range pkg.GoFiles {
		file, err := gatekit.ParseFile(filepath.Join(dir, name), parser.ParseComments)
		if err != nil {
			return typedPackage{}, err
		}
		files = append(files, file)
	}
	info := newTypedInfo()
	conf := types.Config{Importer: imports}
	checked, err := conf.Check(pkg.ImportPath, gatekit.SourceFileSet(), files, info)
	if err != nil {
		return typedPackage{}, fmt.Errorf("type-checking %s: %w", pkg.ImportPath, err)
	}
	return typedPackage{path: pkg.ImportPath, dir: filepath.ToSlash(dir), files: files, info: info, types: checked}, nil
}

// newTypedInfo asks the checker for everything a census reads.
func newTypedInfo() *types.Info {
	return &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Instances:  map[*ast.Ident]types.Instance{},
	}
}
