// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The censuses in this package read source with its types resolved. What they
// read is listed by the go command — this package and every package beneath it
// — so a new subpackage is read the day it is added, without a list to extend.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// typeCheckedSources is parsed source with the type information the census
// resolves both halves through.
type typeCheckedSources struct {
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
	pkg   *types.Package
}

// composePackage is one package of this tree as the go command lists it.
type composePackage struct {
	ImportPath, Dir  string
	GoFiles, Imports []string
}

// composeLoad is what type-checking this tree needs from the go command: each
// package's production file list, honouring build tags, and an importer reading
// the export data its imports already compiled to for this test binary.
type composeLoad struct {
	fset     *token.FileSet
	root     string
	packages []composePackage
	importer types.Importer
}

// loadCompose runs once per test binary; the importer caches what it reads, so
// the planted fixtures resolve runner to the same package the real files do.
var loadCompose = sync.OnceValues(func() (*composeLoad, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("finding this package's directory: %w", err)
	}
	packages, err := listComposeTree()
	if err != nil {
		return nil, err
	}
	imports := map[string]bool{}
	for _, p := range packages {
		for _, path := range p.Imports {
			imports[path] = true
		}
	}
	args := []string{"-export", "-deps", "-f", "{{if .Export}}{{.ImportPath}}\t{{.Export}}{{end}}"}
	for path := range imports {
		args = append(args, path)
	}
	out, err := goList(args...)
	if err != nil {
		return nil, err
	}
	exports := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if path, file, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			exports[path] = file
		}
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("go list reported no export data for %s", path)
		}
		return os.Open(file)
	})
	return &composeLoad{fset: fset, root: root, packages: packages, importer: imp}, nil
})

// listComposeTree lists this package and every package beneath it that has
// production files.
func listComposeTree() ([]composePackage, error) {
	out, err := goList("-json=ImportPath,Dir,GoFiles,Imports", "./...")
	if err != nil {
		return nil, err
	}
	var packages []composePackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p composePackage
		err := dec.Decode(&p)
		if errors.Is(err, io.EOF) {
			return packages, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decoding go list for this tree: %w", err)
		}
		if len(p.GoFiles) > 0 {
			packages = append(packages, p)
		}
	}
}

func goList(args ...string) ([]byte, error) {
	cmd := exec.Command("go", append([]string{"list"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

// typeCheckedComposeSources type-checks this package's production files.
func typeCheckedComposeSources(t *testing.T) *typeCheckedSources {
	t.Helper()
	load := mustLoadCompose(t)
	for _, p := range load.packages {
		if p.Dir == load.root {
			return typeCheckComposePackage(t, load, p)
		}
	}
	t.Fatal("go list did not report this package")
	return nil
}

// typeCheckedComposeTree type-checks the production files of this package and
// of every package beneath it.
func typeCheckedComposeTree(t *testing.T) []*typeCheckedSources {
	t.Helper()
	load := mustLoadCompose(t)
	checked := make([]*typeCheckedSources, 0, len(load.packages))
	for _, p := range load.packages {
		checked = append(checked, typeCheckComposePackage(t, load, p))
	}
	return checked
}

// typeCheckComposePackage checks one package under its path relative to this
// directory — "compose", "compose/company360" — so a census names it readably.
func typeCheckComposePackage(t *testing.T, load *composeLoad, p composePackage) *typeCheckedSources {
	t.Helper()
	rel, err := filepath.Rel(load.root, p.Dir)
	if err != nil {
		t.Fatalf("placing %s under this package: %v", p.ImportPath, err)
	}
	files := make([]*ast.File, 0, len(p.GoFiles))
	for _, name := range p.GoFiles {
		parsed, err := parser.ParseFile(load.fset, filepath.Join(p.Dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, parsed)
	}
	return typeCheck(t, load, filepath.ToSlash(filepath.Join("compose", rel)), files)
}

func mustLoadCompose(t *testing.T) *composeLoad {
	t.Helper()
	load, err := loadCompose()
	if err != nil {
		t.Fatalf("loading this tree for type-checking: %v", err)
	}
	return load
}

// typeCheckPlanted type-checks one planted file against the same imports.
func typeCheckPlanted(t *testing.T, src string) *typeCheckedSources {
	t.Helper()
	return typeCheckPlantedAs(t, "planted", src)
}

// typeCheckPlantedAs is typeCheckPlanted under the package path a census will
// name it by.
func typeCheckPlantedAs(t *testing.T, path, src string) *typeCheckedSources {
	t.Helper()
	load := mustLoadCompose(t)
	parsed, err := parser.ParseFile(load.fset, "planted.go", src, 0)
	if err != nil {
		t.Fatalf("parse the planted file: %v", err)
	}
	return typeCheck(t, load, path, []*ast.File{parsed})
}

func typeCheck(t *testing.T, load *composeLoad, path string, files []*ast.File) *typeCheckedSources {
	t.Helper()
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
		Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
	}
	pkg, err := (&types.Config{Importer: load.importer}).Check(path, load.fset, files, info)
	if err != nil {
		// A file the checker cannot resolve is a file the census cannot clear.
		t.Fatalf("type-checking %s: %v", path, err)
	}
	return &typeCheckedSources{fset: load.fset, files: files, info: info, pkg: pkg}
}

// The listing is the census's whole subject, so it is checked against the
// directories themselves: every directory beneath this one holding a Go file
// this build compiles is a package the censuses read. A pattern or a filter
// that dropped one would otherwise pass by reading less.
func TestTheComposeTreeListingReadsEveryPackage(t *testing.T) {
	load := mustLoadCompose(t)
	listed := map[string]bool{}
	for _, p := range load.packages {
		listed[p.Dir] = true
	}
	err := filepath.WalkDir(load.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_")) {
			return filepath.SkipDir
		}
		name, dir := d.Name(), filepath.Dir(path)
		if d.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		// A file this build's constraints exclude — an integration harness
		// tagged out — is not production for this build, the same rule the
		// go command's listing applies.
		built, err := build.Default.MatchFile(dir, name)
		if err != nil {
			return err
		}
		if built && !listed[dir] {
			t.Errorf("%s holds production Go but the census listing does not read it", dir)
			listed[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking this tree: %v", err)
	}
	if len(load.packages) < 2 {
		t.Fatalf("listed %d packages — the listing is not reading beneath this one", len(load.packages))
	}
}
