// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"path"
	"sort"
	"strconv"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const threadRestoreOwner = "internal/platform/database/storekit"

func threadRestoreReference(file *ast.File, node ast.Node) bool {
	aliases := map[string]bool{}
	for _, spec := range file.Imports {
		module, err := strconv.Unquote(spec.Path.Value)
		if err != nil || module != "github.com/margince/margince/backend/"+threadRestoreOwner {
			continue
		}
		name := "storekit"
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = true
	}
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if ok && aliases[owner.Name] && (selector.Sel.Name == "Unarchive" || selector.Sel.Name == "UnarchiveShape") {
			found = true
		}
		return !found
	})
	return found
}

func (index *threadProducerIndex) restoreCatalog() []string {
	if index.restoreCatalogLoaded {
		return index.restoreDeclarations
	}
	index.restoreCatalogLoaded = true
	scope := gatekit.Scope{Roots: []string{"internal"}, Subject: func(_ string, file *ast.File) bool { return threadRestoreReference(file, file) }}
	for _, source := range scope.Files(index.t) {
		index.restoreDeclarations = append(index.restoreDeclarations, threadRestoreSites(path.Dir(source.Path), source.File)...)
	}
	sort.Strings(index.restoreDeclarations)
	return index.restoreDeclarations
}

func threadRestoreSites(directory string, file *ast.File) []string {
	var declarations []string
	for _, declaration := range file.Decls {
		if !threadRestoreReference(file, declaration) {
			continue
		}
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			declarations = append(declarations, directory+":"+scrubKey(receiverTypeName(declaration), declaration.Name.Name))
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				if !threadRestoreReference(file, spec) {
					continue
				}
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						declarations = append(declarations, directory+":"+name.Name)
					}
				case *ast.TypeSpec:
					declarations = append(declarations, directory+":"+spec.Name.Name)
				}
			}
		}
	}
	return declarations
}
