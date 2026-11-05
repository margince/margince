// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared SETTINGS into the manifest: the values the core
// lets an admin change on the unit's card.

import (
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredSetting is one setting as the manifest carries it.
type declaredSetting struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
	Kind  string `json:"kind"`
}

// readSettings reads the Settings slice literal. Each setting passes the
// published Setting.Validate the boot runs.
func (r *unitReader) readSettings(expr ast.Expr, file *ast.File) ([]declaredSetting, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Settings must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredSetting, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		s, err := r.readSetting(elt, ext)
		if err != nil {
			return nil, err
		}
		decl := extension.Setting{Key: s.Key, Label: s.Label, Hint: s.Hint, Kind: extension.SettingKind(s.Kind)}
		if err := decl.Validate(); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if seen[s.Key] {
			return nil, r.errPos(elt, "two settings with key %q", s.Key)
		}
		seen[s.Key] = true
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// readSetting reads one extension.Setting literal.
func (r *unitReader) readSetting(elt ast.Expr, ext string) (declaredSetting, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "Setting")) {
		return declaredSetting{}, r.errAt(elt, "a Settings entry must be an extension.Setting literal")
	}
	var s declaredSetting
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return declaredSetting{}, r.errAt(e, "Setting fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return declaredSetting{}, r.errAt(kv.Key, "Setting fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Key":
			s.Key, err = r.stringLit(kv.Value, "Setting.Key")
		case "Label":
			s.Label, err = r.stringLit(kv.Value, "Setting.Label")
		case "Hint":
			s.Hint, err = r.stringLit(kv.Value, "Setting.Hint")
		case "Kind":
			s.Kind, err = r.constValue(kv.Value, ext)
		default:
			err = r.errAt(kv, "Setting field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return declaredSetting{}, err
		}
	}
	return s, nil
}
