// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// fieldDocs are the doc comments of one package's struct types and fields,
// keyed "Type" and "Type.Field". The schema describes a reflected field with
// them, so its hover text is the comment its author keeps beside it.
type fieldDocs struct {
	pkgPath string
	docs    map[string]string
}

// of is the field's own doc, else the doc of the named type it holds.
func (d fieldDocs) of(owner reflect.Type, f reflect.StructField) string {
	if owner.PkgPath() != d.pkgPath || d.pkgPath == "" {
		return ""
	}
	if doc := d.docs[owner.Name()+"."+f.Name]; doc != "" {
		return doc
	}
	held := f.Type
	for held.Kind() == reflect.Pointer || held.Kind() == reflect.Slice {
		held = held.Elem()
	}
	if held.PkgPath() != d.pkgPath {
		return ""
	}
	return d.docs[held.Name()]
}

// readFieldDocs parses the package in dir, which pkgPath names.
func readFieldDocs(dir, pkgPath string) (fieldDocs, error) {
	out := fieldDocs{pkgPath: pkgPath, docs: map[string]string{}}
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return out, fmt.Errorf("gen-config: listing %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path) // #nosec G304 -- a source file of the repository's own package
		if err != nil {
			return out, fmt.Errorf("gen-config: reading %s: %w", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return out, fmt.Errorf("gen-config: parsing %s: %w", path, err)
		}
		collectDocs(file, out.docs)
	}
	if len(out.docs) == 0 {
		return out, fmt.Errorf("gen-config: no documented struct in %s; the package moved", dir)
	}
	return out, nil
}

func collectDocs(file *ast.File, docs map[string]string) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeDoc := ts.Doc
			if typeDoc == nil && len(gen.Specs) == 1 {
				typeDoc = gen.Doc
			}
			docs[ts.Name.Name] = firstParagraph(typeDoc)
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					docs[ts.Name.Name+"."+name.Name] = firstParagraph(field.Doc)
				}
			}
		}
	}
}

// firstParagraph is a comment's first paragraph on one line.
func firstParagraph(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	first, _, _ := strings.Cut(group.Text(), "\n\n")
	return strings.Join(strings.Fields(first), " ")
}
