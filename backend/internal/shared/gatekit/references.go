// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// References reports whether a parsed file reaches symbol in the package at
// importPath — as a call, or as a value it could call later.
//
// It is how a path-scoped gate names the site it judges without type-aware
// analysis of the whole module, and it exists once because every gate that
// wants it wants the same four things:
//
//   - NAMING is enough. A check that matched only `pkg.Sym(...)` is walked past
//     by `f := pkg.Sym; f(...)`, and following a function value needs the type
//     information a fitness test does not have. A file has no reason to hold a
//     symbol it never intends to use, so a gate that treats a reference as use
//     closes that route; a file that genuinely needs the symbol for something
//     else says so through a waiver, which is visible, rather than through a
//     spelling the gate cannot see.
//   - The QUALIFIER is the caller's choice. `import dm ".../dbmigrate"` and a
//     dot-import that leaves the symbol bare reach it exactly as much as the
//     canonical spelling does, so the name is resolved through the file's own
//     imports rather than assumed.
//   - A file INSIDE the package reaches the symbol with no import at all.
//   - PROSE IS NOT A REFERENCE. A gate's own doc comment and failure message
//     name the symbol it hunts, and a text scan over a tree that includes the
//     gate flags the gate. Comments and string literals carry no identifier for
//     the syntax tree to match, so this cannot.
//
// The in-package case keys on the package clause matching importPath's last
// element, which is this repo's convention for every package a gate targets. A
// package whose name differs from its directory would need its own spelling.
func References(file *ast.File, importPath, symbol string) bool {
	qualifier, dotImported := ImportedAs(file, importPath)
	inPackage := file.Name != nil && file.Name.Name == path.Base(importPath)
	if qualifier == "" && !dotImported && !inPackage {
		return false
	}
	bare := dotImported || inPackage
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		found = reachesSymbol(n, qualifier, symbol, bare)
		return !found
	})
	return found
}

// reachesSymbol answers the question for ONE node: is this the symbol, under
// whichever of the three spellings this file can reach it by.
func reachesSymbol(n ast.Node, qualifier, symbol string, bare bool) bool {
	switch node := n.(type) {
	case *ast.SelectorExpr:
		// qualifier.Symbol, whether or not it is being applied here.
		if qualifier == "" || node.Sel == nil || node.Sel.Name != symbol {
			return false
		}
		pkg, ok := node.X.(*ast.Ident)
		return ok && pkg.Name == qualifier
	case *ast.Ident:
		// A bare symbol: reachable under a dot-import, or from inside the
		// package that declares it.
		return bare && node.Name == symbol
	}
	return false
}

// ImportedAs returns the identifier this file binds importPath to, and whether
// it was dot-imported. Both are empty/false when the file does not import it at
// all, which is the cheap way to skip most of a tree.
//
// Exported because a gate that must LOCATE calls, rather than merely ask whether
// a file makes any, needs the same answer References needs: the qualifier is the
// caller's choice, and a gate that assumes the canonical spelling stops seeing a
// file that aliases the import.
func ImportedAs(file *ast.File, importPath string) (qualifier string, dotImported bool) {
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil || imported != importPath {
			continue
		}
		switch {
		case spec.Name == nil:
			// The package's OWN name, not its directory's: Go binds the
			// package clause, and for backend/internal/contracts those differ
			// (it declares crmcontracts). A gate resolving the directory hunts
			// a qualifier no file spells and reports a clean sweep.
			return DeclaredPackageName(importPath), false
		case spec.Name.Name == ".":
			return "", true
		case spec.Name.Name == "_":
			// Imported for side effects only; it cannot be called through.
			return "", false
		default:
			return spec.Name.Name, false
		}
	}
	return "", false
}

// declaredNames caches the package clause found for an import path, so a gate
// sweeping a tree resolves each imported package once rather than per file.
var declaredNames sync.Map // importPath -> string

// DeclaredPackageName answers the identifier Go binds for an UNALIASED import
// of importPath: the package's own `package` clause, not the last segment of
// its directory.
//
// The two differ, and they differ for the path most gates care about:
//
//	directory   backend/internal/contracts
//	package     crmcontracts
//
// A gate resolving the directory hunts `contracts.Foo` through a file that
// says `crmcontracts.Foo`, finds nothing, and reports a clean package — which
// is the under-recognition a census may not have, arriving through a helper
// rather than through the gate's own reader.
//
// A path this cannot resolve falls back to the directory name, which is the
// right answer for the standard library and for any module whose sources are
// not beside this one: `net/http` declares `http`, `go/ast` declares `ast`.
// Falling back is safe in a way guessing is not — the name is only wrong when
// a package renames itself, and that is exactly the case the module-local read
// below covers.
func DeclaredPackageName(importPath string) string {
	if cached, ok := declaredNames.Load(importPath); ok {
		// Checked rather than forced: this map is written only below, so a
		// non-string cannot be in it — and a gate that panicked here would
		// take down a census over a cache, which is the wrong trade for a
		// value it can simply re-derive.
		if name, isString := cached.(string); isString {
			return name
		}
	}
	name := path.Base(importPath)
	if dir, ok := moduleLocalDir(importPath); ok {
		if declared, found := packageClauseIn(dir); found {
			name = declared
		}
	}
	declaredNames.Store(importPath, name)
	return name
}

// moduleLocalDir maps an import path inside THIS module onto the directory
// holding it, or reports that the path belongs to somebody else.
func moduleLocalDir(importPath string) (string, bool) {
	root, modPath, ok := moduleRootAndPath()
	if !ok || modPath == "" {
		return "", false
	}
	if importPath == modPath {
		return root, true
	}
	rel, inside := strings.CutPrefix(importPath, modPath+"/")
	if !inside {
		return "", false
	}
	return filepath.Join(root, filepath.FromSlash(rel)), true
}

// packageClauseIn reads the package clause of the production sources in dir.
//
// `_test.go` files are skipped because one may declare the EXTERNAL test
// package — `crmcontracts_test` — and a directory listing is ordered by name,
// so one sorting first would hand back a package no production file imports.
// That is the same fail-short direction this helper exists to close, and a
// wrong name is not an absent one.
func packageClauseIn(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if perr != nil || file.Name == nil {
			continue
		}
		return file.Name.Name, true
	}
	return "", false
}

// moduleRootAndPath walks up from the working directory for the go.mod that
// bounds this module, and answers its directory and its declared module path.
//
// Cached because every gate in a sweep asks, and the answer cannot change
// during a run.
var (
	moduleOnce sync.Once
	moduleDir  string
	modulePath string
	moduleOK   bool
)

func moduleRootAndPath() (dir, modPath string, ok bool) {
	moduleOnce.Do(func() {
		cwd, err := os.Getwd()
		if err != nil {
			return
		}
		for dir := cwd; ; {
			candidate := filepath.Join(dir, "go.mod")
			// The path is this process's own working directory with "go.mod"
			// appended, walked upward — no caller supplies any part of it, and
			// a gate that could not read its own module root would be a census
			// resolving nothing.
			//nolint:gosec // G304: the path is Getwd plus a fixed filename, never an input
			if body, readErr := os.ReadFile(candidate); readErr == nil {
				for _, line := range strings.Split(string(body), "\n") {
					if rest, found := strings.CutPrefix(strings.TrimSpace(line), "module "); found {
						moduleDir, modulePath, moduleOK = dir, strings.TrimSpace(rest), true
						return
					}
				}
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	})
	return moduleDir, modulePath, moduleOK
}
