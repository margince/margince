// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every exported name in pkg/extension has a caller outside it, or a written
// reason to exist. A released name is kept for ever, so one added for a need
// nobody has yet is a lasting promise. The names come from the package's own
// declarations, so a new one is covered without being listed.
//
// Without type information a method counts as used wherever a file that imports
// the surface selects a member of that name. That errs toward "used". Struct
// fields and generated files are not read.

import (
	"go/ast"
	"go/parser"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	extensionSurfaceDir    = "pkg/extension"
	extensionSurfaceImport = modulePath + "/pkg"
)

// exportUserTrees are where a caller of the surface lives, from the module root.
var exportUserTrees = []string{"internal", "cmd", "tools", "../extensions", "../composition"}

// surfaceFile is one parsed file of the surface and the key of its package,
// the import path below backend/pkg (`extension`, `extension/crm`).
type surfaceFile struct {
	pkg  string
	file *ast.File
}

func TestEveryExportedExtensionNameHasACallerOrAReason(t *testing.T) {
	t.Parallel()
	surface := parseSurface(t)
	exported := surfaceExports(surface)
	if len(exported) < exportCensusFloor {
		t.Fatalf("read %d exported names under %s, want at least the %d this gate was written against: the walk or the surface changed shape",
			len(exported), extensionSurfaceDir, exportCensusFloor)
	}
	unused := unusedExports(exported, parseExportUsers(t))
	for _, name := range unused {
		if _, kept := exportWaivers[name]; !kept {
			t.Errorf("%s is exported from %s and nothing outside it uses it: give it a caller in the same change, "+
				"or unexport it. A name kept for code outside this tree needs a line in exportWaivers saying why.", name, extensionSurfaceDir)
		}
	}
	for name := range exportWaivers {
		switch {
		case !slices.Contains(exported, name):
			t.Errorf("exportWaivers names %s, which %s no longer exports: remove the line", name, extensionSurfaceDir)
		case !slices.Contains(unused, name):
			t.Errorf("exportWaivers names %s, which now has a caller: remove the line", name)
		}
	}
}

func parseSurface(t *testing.T) []surfaceFile {
	t.Helper()
	var files []surfaceFile
	walkGoSources(t, extensionSurfaceDir, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_gen.go") {
			return
		}
		rel, err := filepath.Rel("pkg", filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, surfaceFile{pkg: filepath.ToSlash(rel), file: file})
	})
	return files
}

func parseExportUsers(t *testing.T) []*ast.File {
	t.Helper()
	var files []*ast.File
	for _, tree := range exportUserTrees {
		walkGoSources(t, tree, func(_ string, file *ast.File) { files = append(files, file) })
	}
	return files
}

// walkGoSources parses every non-test Go file under root, skipping the folders
// that hold no product code.
func walkGoSources(t *testing.T, root string, visit func(path string, file *ast.File)) {
	t.Helper()
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "testdata" || name == "node_modules" || name == "vendor" {
				return fs.SkipDir
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
		visit(path, file)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// surfaceExports lists the exported names: `pkg.Name` for a package-level
// declaration, `pkg.Type.Method` for a method of an exported type or interface.
func surfaceExports(surface []surfaceFile) []string {
	var names []string
	for _, sf := range surface {
		for _, decl := range sf.file.Decls {
			names = append(names, declExports(sf.pkg, decl)...)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func declExports(pkg string, decl ast.Decl) []string {
	var names []string
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if !d.Name.IsExported() {
			return nil
		}
		if d.Recv == nil {
			return []string{pkg + "." + d.Name.Name}
		}
		if recv := receiverTypeName(d); ast.IsExported(recv) {
			return []string{pkg + "." + recv + "." + d.Name.Name}
		}
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if !s.Name.IsExported() {
					continue
				}
				names = append(names, pkg+"."+s.Name.Name)
				if iface, ok := s.Type.(*ast.InterfaceType); ok {
					for _, method := range iface.Methods.List {
						for _, name := range method.Names {
							if name.IsExported() {
								names = append(names, pkg+"."+s.Name.Name+"."+name.Name)
							}
						}
					}
				}
			case *ast.ValueSpec:
				for _, name := range s.Names {
					if name.IsExported() {
						names = append(names, pkg+"."+name.Name)
					}
				}
			}
		}
	}
	return names
}

// unusedExports returns the exported names no user file selects.
func unusedExports(exported []string, users []*ast.File) []string {
	used, members := map[string]bool{}, map[string]bool{}
	for _, file := range users {
		aliases := surfaceAliases(file)
		if len(aliases) == 0 {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, isIdent := sel.X.(*ast.Ident); isIdent {
				if pkg, imported := aliases[ident.Name]; imported {
					used[pkg+"."+sel.Sel.Name] = true
					return true
				}
			}
			members[sel.Sel.Name] = true
			return true
		})
	}
	var unused []string
	for _, name := range exported {
		parts := strings.Split(name, ".")
		if used[name] || (len(parts) == 3 && members[parts[2]]) {
			continue
		}
		unused = append(unused, name)
	}
	return unused
}

// surfaceAliases maps the name a file imports each surface package under to
// that package's key.
func surfaceAliases(file *ast.File) map[string]string {
	aliases := map[string]string{}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		pkg, ok := strings.CutPrefix(importPath, extensionSurfaceImport+"/")
		if !ok || (pkg != "extension" && !strings.HasPrefix(pkg, "extension/")) {
			continue
		}
		alias := path.Base(pkg)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		aliases[alias] = pkg
	}
	return aliases
}

func TestTheExportCensusReadsANewNameWithNoCaller(t *testing.T) {
	t.Parallel()
	parse := func(src string) *ast.File {
		file, err := parser.ParseFile(gatekit.SourceFileSet(), "fixture.go", src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	surface := []surfaceFile{{pkg: "extension", file: parse(`package extension
type Core interface { Deals() int; Notices() int }
func Kept() {}
func Fresh() {}
`)}}
	user := parse(`package unit
import ext "` + extensionSurfaceImport + `/extension"
func run(c ext.Core) { ext.Kept(); c.Deals() }
`)
	got := unusedExports(surfaceExports(surface), []*ast.File{user})
	if want := []string{"extension.Core.Notices", "extension.Fresh"}; !slices.Equal(got, want) {
		t.Fatalf("unused = %v, want %v", got, want)
	}
	stranger := parse(`package other
func run(x interface{ Notices() int }) { x.Notices() }
func Fresh() {}
`)
	if got := unusedExports(surfaceExports(surface), []*ast.File{user, stranger}); !slices.Contains(got, "extension.Core.Notices") {
		t.Errorf("a file that does not import the surface counted as a caller of Core.Notices: %v", got)
	}
}
