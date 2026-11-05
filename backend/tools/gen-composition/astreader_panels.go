// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared PANELS into the manifest: the places on core pages
// where the unit's own component is drawn inline.

import (
	"fmt"
	"go/ast"
	"sort"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredPanel is one panel as the manifest and the SPA registries carry it.
type declaredPanel struct {
	Slot   string `json:"slot"`
	Export string `json:"export"`
	Object string `json:"object"`
}

// readPanels reads the Panels slice literal. Each panel passes the published
// Panel.Validate the boot runs. Its object must be one the unit's own
// operations gate on, which the boot checks as well.
func (r *unitReader) readPanels(expr ast.Expr, file *ast.File) ([]declaredPanel, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Panels must be a slice literal")
	}
	ext := importAlias(file, extensionPkgPath)
	gated := map[string]bool{}
	for _, v := range r.verbs {
		gated[v.verb.RbacObject] = true
	}
	out := make([]declaredPanel, 0, len(lit.Elts))
	seen := map[string]bool{}
	for _, elt := range lit.Elts {
		panel, err := r.readPanel(elt, ext)
		if err != nil {
			return nil, err
		}
		decl := extension.Panel{Slot: extension.PanelSlot(panel.Slot), Export: panel.Export, Object: panel.Object}
		if err := decl.Validate(extension.Name(r.unit)); err != nil {
			return nil, r.errPos(elt, "%v", err)
		}
		if !gated[panel.Object] {
			return nil, r.errPos(elt, "panel in slot %q is gated on %q, which none of the unit's operations gates on — no grant of it lets a reader use the unit", panel.Slot, panel.Object)
		}
		if seen[panel.Slot] {
			return nil, r.errPos(elt, "two panels in slot %q", panel.Slot)
		}
		seen[panel.Slot] = true
		out = append(out, panel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out, nil
}

// readPanel reads one extension.Panel literal.
func (r *unitReader) readPanel(elt ast.Expr, ext string) (declaredPanel, error) {
	lit, ok := elt.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "Panel")) {
		return declaredPanel{}, r.errAt(elt, "a Panels entry must be an extension.Panel literal")
	}
	var panel declaredPanel
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return declaredPanel{}, r.errAt(e, "Panel fields must be keyed")
		}
		k, ok := kv.Key.(*ast.Ident)
		if !ok {
			return declaredPanel{}, r.errAt(kv.Key, "Panel fields must be keyed by name")
		}
		var err error
		switch k.Name {
		case "Slot":
			panel.Slot, err = r.constValue(kv.Value, ext)
		case "Export":
			panel.Export, err = r.stringLit(kv.Value, "Panel.Export")
		case "Object":
			panel.Object, err = r.stringLit(kv.Value, "Panel.Object")
		default:
			err = r.errAt(kv, "Panel field %s is not derivable by this generator", k.Name)
		}
		if err != nil {
			return declaredPanel{}, err
		}
	}
	return panel, nil
}

// checkPanelFrontends refuses a unit that declares a panel and ships no
// frontend package: the registry would name a component nothing exports.
func checkPanelFrontends(manifests []derivedManifest) error {
	for _, m := range manifests {
		if len(m.Manifest.Panels) > 0 && m.Unit.Frontend == nil {
			return fmt.Errorf("extensions/%s: declares a panel but ships no %s/package.json — a panel is a component the unit's frontend package exports",
				m.Unit.Name, frontendLayer)
		}
	}
	return nil
}

// manifestPanels is each unit's declared panels, keyed by unit name.
func manifestPanels(manifests []derivedManifest) map[string][]declaredPanel {
	out := make(map[string][]declaredPanel, len(manifests))
	for _, m := range manifests {
		if len(m.Manifest.Panels) > 0 {
			out[m.Unit.Name] = m.Manifest.Panels
		}
	}
	return out
}
