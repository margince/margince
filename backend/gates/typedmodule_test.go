// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The backend module type-checked from source, each package against its
// imports' compiled export data. A census reading it gets scope, constants,
// promotion and method resolution from the type checker.

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// typedPackage is one package of this module, checked.
type typedPackage struct {
	path  string
	files []*ast.File
	info  *types.Info
}

type listedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	Export     string
	Module     *struct{ Path string }
}

var typedModuleOnce = sync.OnceValues(loadTypedModule)

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
	pkgs, err := typedModuleOnce()
	if err != nil {
		t.Fatalf("type-checking the module: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("type-checked no package of the module, and an empty module agrees with every census")
	}
	return pkgs
}

func loadTypedModule() ([]typedPackage, error) {
	listed, err := listModule()
	if err != nil {
		return nil, err
	}
	exports := map[string]string{}
	var roots []listedPackage
	for _, pkg := range listed {
		exports[pkg.ImportPath] = pkg.Export
		if pkg.Module != nil && pkg.Module.Path == modulePath && len(pkg.GoFiles) > 0 {
			roots = append(roots, pkg)
		}
	}
	out := make([]typedPackage, len(roots))
	errs := make([]error, len(roots))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			// An importer caches what it reads and is not safe to share, so
			// each worker holds its own.
			imports := importer.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
				file, ok := exports[path]
				if !ok || file == "" {
					return nil, fmt.Errorf("no export data for %s", path)
				}
				return os.Open(file)
			})
			for i := range jobs {
				out[i], errs[i] = checkListedPackage(roots[i], imports)
			}
		})
	}
	for i := range roots {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out, errors.Join(errs...)
}

// listModule asks the go command for every package of the module and its
// dependencies, with the export data a build left for each.
func listModule() ([]listedPackage, error) {
	cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,Export,Module", "./...")
	cmd.Stderr = os.Stderr
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
		return nil, fmt.Errorf("go list -export: %w", err)
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
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: imports}
	if _, err := conf.Check(pkg.ImportPath, gatekit.SourceFileSet(), files, info); err != nil {
		return typedPackage{}, fmt.Errorf("type-checking %s: %w", pkg.ImportPath, err)
	}
	return typedPackage{path: pkg.ImportPath, files: files, info: info}, nil
}
