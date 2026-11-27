// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared LANGUAGES into the manifest: the languages the unit
// lets stored data carry, beside the three core ships.

import (
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredLanguage is one language as the manifest carries it.
type declaredLanguage struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// readLanguages reads the Languages slice literal. Each language passes the
// published Language.Validate the boot runs.
func (r *unitReader) readLanguages(expr ast.Expr, file *ast.File) ([]declaredLanguage, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Languages must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredLanguage, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		language, err := r.readLanguage(elt, ext)
		if err != nil {
			return nil, err
		}
		if err := language.Validate(); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if seen[language.Code] {
			return nil, r.errPos(elt, "two languages with the code %q", language.Code)
		}
		seen[language.Code] = true
		out = append(out, declaredLanguage{Code: language.Code, Label: language.Label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// readLanguage reads one extension.Language literal.
func (r *unitReader) readLanguage(elt ast.Expr, ext string) (extension.Language, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "Language")) {
		return extension.Language{}, r.errAt(elt, "a Languages entry must be an extension.Language literal")
	}
	var language extension.Language
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return extension.Language{}, r.errAt(e, "Language fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return extension.Language{}, r.errAt(kv.Key, "Language fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Code":
			language.Code, err = r.stringLit(kv.Value, "Language.Code")
		case "Label":
			language.Label, err = r.stringLit(kv.Value, "Language.Label")
		default:
			err = r.errAt(kv, "Language field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return extension.Language{}, err
		}
	}
	return language, nil
}
