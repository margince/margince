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
	"strings"
)

// docIndex maps Type.Field to the doc comment written above that field.
//
// The descriptions are read from the SOURCE rather than written again here.
// They are what an editor shows on hover, and a schema carrying its own prose
// would be a second explanation of every switch, free to disagree with the one
// beside the code — which is the failure this whole generator exists to avoid.
type docIndex map[string]string

func (d docIndex) lookup(typeName, fieldName string) string { return d[typeName+"."+fieldName] }

// forType is the doc comment on a named type, used when the FIELD carrying it
// has none. Several sections here are documented on the type rather than at the
// single place they are embedded — Operations explains the kill switches where
// they are declared — and taking the field's silence at face value would drop
// the best description in the file.
func (d docIndex) forType(typeName string) string { return d["type:"+typeName] }

// fieldDocs parses the package and collects every struct field's doc comment.
//
// File by file rather than parser.ParseDir, which is deprecated for not
// honouring build tags. Tests are skipped: a struct declared in one is not part
// of the config an operator writes.
func fieldDocs(dir string) (docIndex, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("gen-configschema: reading %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	out := docIndex{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("gen-configschema: parsing %s: %w", name, err)
		}
		collectFile(file, out)
	}
	if len(out) == 0 {
		// NOT a tolerated zero: this package documents its fields, so an empty
		// index means the walk stopped matching and every description would go
		// silently missing from a schema that still looks complete.
		return nil, fmt.Errorf("gen-configschema: no field documentation found under %s — the parse found nothing to read", dir)
	}
	return out, nil
}

func collectFile(file *ast.File, out docIndex) {
	ast.Inspect(file, func(n ast.Node) bool {
		// A GenDecl, not the TypeSpec inside it: for a standalone `type X
		// struct`, go/ast hangs the doc comment on the DECLARATION, and reading
		// TypeSpec.Doc finds nil for every type written that way — which is all
		// of them here.
		decl, ok := n.(*ast.GenDecl)
		if !ok {
			return true
		}
		for _, s := range decl.Specs {
			collectSpec(s, decl.Doc, out)
		}
		return true
	})
}

func collectSpec(s ast.Spec, declDoc *ast.CommentGroup, out docIndex) {
	spec, ok := s.(*ast.TypeSpec)
	if !ok {
		return
	}
	structType, ok := spec.Type.(*ast.StructType)
	if !ok {
		return
	}
	doc := spec.Doc
	if doc == nil {
		doc = declDoc
	}
	if text := summarize(doc); text != "" {
		out["type:"+spec.Name.Name] = text
	}
	for _, field := range structType.Fields.List {
		text := summarize(field.Doc)
		if text == "" {
			continue
		}
		for _, name := range field.Names {
			out[spec.Name.Name+"."+name.Name] = text
		}
	}
}

// summarize turns a Go doc comment into one description line.
//
// Only the leading paragraph. These comments run long — several explain a
// decision at length, which is right in the source and wrong in an editor
// tooltip that renders it as one unbroken line.
func summarize(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	var para []string
	for _, line := range strings.Split(doc.Text(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		para = append(para, line)
	}
	return strings.Join(para, " ")
}
