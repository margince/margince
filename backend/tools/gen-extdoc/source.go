// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// pkgSource is the parsed, hand-written source of backend/pkg/extension. The
// published lists come from the package itself. The source adds what a value
// cannot say: which struct holds which field, and the comment beside a name.
type pkgSource struct {
	types  map[string]*ast.TypeSpec
	consts map[string]constDecl
	// recordKinds are the values of the RecordKind constants.
	recordKinds []string
}

type constDecl struct {
	typ, doc string
}

func parsePackage(dir string) (*pkgSource, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("gen-extdoc: listing %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	src := &pkgSource{types: map[string]*ast.TypeSpec{}, consts: map[string]constDecl{}}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("gen-extdoc: parsing %s: %w", path, err)
		}
		for _, d := range file.Decls {
			if gd, ok := d.(*ast.GenDecl); ok {
				src.add(gd)
			}
		}
	}
	slices.Sort(src.recordKinds)
	return src, nil
}

func (s *pkgSource) add(gd *ast.GenDecl) {
	for _, spec := range gd.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			s.types[sp.Name.Name] = sp
		case *ast.ValueSpec:
			s.addConst(gd, sp)
		}
	}
}

func (s *pkgSource) addConst(gd *ast.GenDecl, sp *ast.ValueSpec) {
	id, ok := sp.Type.(*ast.Ident)
	if !ok || len(sp.Values) != 1 {
		return
	}
	lit, ok := sp.Values[0].(*ast.BasicLit)
	if !ok {
		return
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	doc := sp.Doc
	if doc == nil && len(gd.Specs) == 1 {
		doc = gd.Doc
	}
	s.consts[value] = constDecl{typ: id.Name, doc: doc.Text()}
	if id.Name == "RecordKind" {
		s.recordKinds = append(s.recordKinds, value)
	}
}

// fields are the field names of a struct type, nil when it is not one.
func (s *pkgSource) fields(typeName string) []*ast.Field {
	sp, ok := s.types[typeName]
	if !ok {
		return nil
	}
	st, ok := sp.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	return st.Fields.List
}

func hasField(fields []*ast.Field, name string) bool {
	return slices.ContainsFunc(fields, func(f *ast.Field) bool {
		return slices.ContainsFunc(f.Names, func(n *ast.Ident) bool { return n.Name == name })
	})
}

// methods are the method names of an interface type.
func (s *pkgSource) methods(typeName string) []string {
	sp, ok := s.types[typeName]
	if !ok {
		return nil
	}
	it, ok := sp.Type.(*ast.InterfaceType)
	if !ok {
		return nil
	}
	var out []string
	for _, m := range it.Methods.List {
		for _, n := range m.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

// coreRepos are the repository interfaces the Core door returns.
func (s *pkgSource) coreRepos() []string {
	var out []string
	for _, m := range s.interfaceMethods("Core") {
		fn, ok := m.Type.(*ast.FuncType)
		if !ok || fn.Results == nil || len(fn.Results.List) == 0 {
			continue
		}
		if id, ok := fn.Results.List[0].Type.(*ast.Ident); ok {
			out = append(out, id.Name)
		}
	}
	return out
}

func (s *pkgSource) interfaceMethods(typeName string) []*ast.Field {
	sp, ok := s.types[typeName]
	if !ok {
		return nil
	}
	it, ok := sp.Type.(*ast.InterfaceType)
	if !ok {
		return nil
	}
	return it.Methods.List
}

// leadingSubject is the "Name is" or "Names are" a Go doc comment opens with.
var leadingSubject = regexp.MustCompile(`^\w+ (?:is|are) (?:the |a |an )?`)

// noise is what a reader of the page cannot follow: a pointer to a source
// file or a decision number.
var noise = regexp.MustCompile(` \((?:see |ADR)[^)]*\)`)

// firstSentence is a doc comment up to its first full stop, on one line, with
// the subject its comment opens with taken off.
func firstSentence(doc string) string {
	flat := strings.Join(strings.Fields(doc), " ")
	if i := strings.Index(flat, ". "); i >= 0 {
		flat = flat[:i+1]
	}
	flat = noise.ReplaceAllString(flat, "")
	flat = strings.ReplaceAll(flat, " — ", ", ")
	if rest := leadingSubject.ReplaceAllString(flat, ""); rest != flat && rest != "" {
		flat = strings.ToUpper(rest[:1]) + rest[1:]
	}
	return flat
}

func parseDir(root string) (*pkgSource, error) {
	return parsePackage(filepath.Join(root, "backend", "pkg", "extension"))
}
