// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared LINK TYPES into the manifest, by the short name the
// unit declared them under.

import (
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredLinkType is one link type as the manifest carries it.
type declaredLinkType struct {
	Type  string   `json:"type"`
	Label string   `json:"label"`
	From  []string `json:"from,omitempty"`
	To    []string `json:"to,omitempty"`
}

// readLinkTypes reads the LinkTypes slice literal. Each type passes the
// published LinkType.Validate the boot runs.
func (r *unitReader) readLinkTypes(expr ast.Expr, file *ast.File) ([]declaredLinkType, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "LinkTypes must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredLinkType, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		t, err := r.readLinkType(elt, ext)
		if err != nil {
			return nil, err
		}
		if err := t.Validate(); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if seen[t.Type] {
			return nil, r.errPos(elt, "two link types named %q", t.Type)
		}
		seen[t.Type] = true
		out = append(out, declaredLinkType{Type: t.Type, Label: t.Label, From: t.From, To: t.To})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out, nil
}

// readLinkType reads one extension.LinkType literal.
func (r *unitReader) readLinkType(elt ast.Expr, ext string) (extension.LinkType, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "LinkType")) {
		return extension.LinkType{}, r.errAt(elt, "a LinkTypes entry must be an extension.LinkType literal")
	}
	var t extension.LinkType
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return extension.LinkType{}, r.errAt(e, "LinkType fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return extension.LinkType{}, r.errAt(kv.Key, "LinkType fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Type":
			t.Type, err = r.stringLit(kv.Value, "LinkType.Type")
		case "Label":
			t.Label, err = r.stringLit(kv.Value, "LinkType.Label")
		case "From":
			t.From, err = r.readStringSlice(kv.Value, "LinkType.From")
		case "To":
			t.To, err = r.readStringSlice(kv.Value, "LinkType.To")
		default:
			err = r.errAt(kv, "LinkType field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return extension.LinkType{}, err
		}
	}
	return t, nil
}
