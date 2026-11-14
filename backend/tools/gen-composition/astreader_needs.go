// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared NEEDS into the manifest: the core capabilities it
// asks to use, each with the reason an installing admin reads.

import (
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredNeed is one need as the manifest carries it.
type declaredNeed struct {
	Capability string `json:"capability"`
	Reason     string `json:"reason"`
}

// readNeeds reads the Needs slice literal. Each need passes the published
// Need.Validate the boot runs, and a capability listed twice is refused.
func (r *unitReader) readNeeds(expr ast.Expr, file *ast.File) ([]declaredNeed, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Needs must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	out := make([]declaredNeed, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		need, err := r.readNeed(elt, ext)
		if err != nil {
			return nil, err
		}
		if err := need.Validate(); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if seen[string(need.Capability)] {
			return nil, r.errPos(elt, "need %q declared twice", need.Capability)
		}
		seen[string(need.Capability)] = true
		out = append(out, declaredNeed{Capability: string(need.Capability), Reason: need.Reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Capability < out[j].Capability })
	return out, nil
}

// readNeed reads one extension.Need literal.
func (r *unitReader) readNeed(elt ast.Expr, ext string) (extension.Need, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "Need")) {
		return extension.Need{}, r.errAt(elt, "a Needs entry must be an extension.Need literal")
	}
	var need extension.Need
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return extension.Need{}, r.errAt(e, "Need fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return extension.Need{}, r.errAt(kv.Key, "Need fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Capability":
			var capability string
			capability, err = r.constValue(kv.Value, ext)
			need.Capability = extension.Capability(capability)
		case "Reason":
			need.Reason, err = r.stringLit(kv.Value, "Need.Reason")
		default:
			err = r.errAt(kv, "Need field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return extension.Need{}, err
		}
	}
	return need, nil
}
