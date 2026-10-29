// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared COMPANY TYPES into the manifest: the types the unit
// adds beside core's own, by the short name it declared them under.

import (
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredCompanyType is one company type as the manifest carries it.
type declaredCompanyType struct {
	Type  string `json:"type"`
	Label string `json:"label"`
}

// readCompanyTypes reads the CompanyTypes slice literal. Each type passes the
// published CompanyType.Validate the boot runs.
func (r *unitReader) readCompanyTypes(expr ast.Expr, file *ast.File) ([]declaredCompanyType, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "CompanyTypes must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredCompanyType, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		t, err := r.readCompanyType(elt, ext)
		if err != nil {
			return nil, err
		}
		if err := t.Validate(); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if seen[t.Type] {
			return nil, r.errPos(elt, "two company types named %q", t.Type)
		}
		seen[t.Type] = true
		out = append(out, declaredCompanyType{Type: t.Type, Label: t.Label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out, nil
}

// readCompanyType reads one extension.CompanyType literal.
func (r *unitReader) readCompanyType(elt ast.Expr, ext string) (extension.CompanyType, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "CompanyType")) {
		return extension.CompanyType{}, r.errAt(elt, "a CompanyTypes entry must be an extension.CompanyType literal")
	}
	var t extension.CompanyType
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return extension.CompanyType{}, r.errAt(e, "CompanyType fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return extension.CompanyType{}, r.errAt(kv.Key, "CompanyType fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Type":
			t.Type, err = r.stringLit(kv.Value, "CompanyType.Type")
		case "Label":
			t.Label, err = r.stringLit(kv.Value, "CompanyType.Label")
		default:
			err = r.errAt(kv, "CompanyType field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return extension.CompanyType{}, err
		}
	}
	return t, nil
}
