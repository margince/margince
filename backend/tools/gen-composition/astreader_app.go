// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's declared APP into the manifest: the destination the unit
// asks for in the home side drawer.

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/margince/margince/backend/pkg/extension"
)

// declaredApp is the app as the manifest and the SPA registry carry it.
type declaredApp struct {
	Icon   string `json:"icon"`
	Label  string `json:"label"`
	Group  string `json:"group"`
	Object string `json:"object"`
	Export string `json:"export"`
}

// appDecl is embedded in unitManifest: the unit's own destination in the home
// side drawer, nil when it declares none.
type appDecl struct {
	App *declaredApp `json:"app,omitempty"`
}

// readApp reads the App field, which is a pointer to an extension.App literal.
// The app passes the published App.Validate the boot runs. Its object must be
// one the unit's own operations gate on, which the boot checks as well.
func (r *unitReader) readApp(expr ast.Expr, file *ast.File) (*declaredApp, error) {
	ext := importAlias(file, extensionPkgPath)
	addr, ok := expr.(*ast.UnaryExpr)
	if !ok || addr.Op != token.AND {
		return nil, r.errAt(expr, "App must be &extension.App{...}")
	}
	lit, ok := addr.X.(*ast.CompositeLit)
	if !ok || (lit.Type != nil && !isSelector(lit.Type, ext, "App")) {
		return nil, r.errAt(expr, "App must be an extension.App literal")
	}
	var app declaredApp
	for _, e := range lit.Elts {
		if err := r.readAppField(&app, e, ext); err != nil {
			return nil, err
		}
	}
	decl := extension.App{
		Icon: extension.EntryIcon(app.Icon), Label: app.Label,
		Group: extension.AppGroup(app.Group), Object: app.Object, Export: app.Export,
	}
	if err := decl.Validate(extension.Name(r.unit)); err != nil {
		return nil, r.errPos(expr, "%v", err)
	}
	for _, v := range r.verbs {
		if v.verb.RbacObject == app.Object {
			return &app, nil
		}
	}
	return nil, r.errPos(expr, "app is gated on %q, which none of the unit's operations gates on — no grant of it lets a reader use the unit", app.Object)
}

// readAppField reads one keyed field of the App literal.
func (r *unitReader) readAppField(app *declaredApp, e ast.Expr, ext string) error {
	kv, ok := e.(*ast.KeyValueExpr)
	if !ok {
		return r.errAt(e, "App fields must be keyed")
	}
	k, ok := kv.Key.(*ast.Ident)
	if !ok {
		return r.errAt(kv.Key, "App fields must be keyed by name")
	}
	var err error
	switch k.Name {
	case "Icon":
		app.Icon, err = r.constValue(kv.Value, ext)
	case "Group":
		app.Group, err = r.constValue(kv.Value, ext)
	case "Label":
		app.Label, err = r.stringLit(kv.Value, "App.Label")
	case "Object":
		app.Object, err = r.stringLit(kv.Value, "App.Object")
	case "Export":
		app.Export, err = r.stringLit(kv.Value, "App.Export")
	default:
		err = r.errAt(kv, "App field %s is not derivable by this generator", k.Name)
	}
	return err
}

// checkAppFrontends refuses a unit that declares an app and ships no frontend
// package: the registry would name a component nothing exports.
func checkAppFrontends(manifests []derivedManifest) error {
	for _, m := range manifests {
		if m.Manifest.App != nil && m.Unit.Frontend == nil {
			return fmt.Errorf("extensions/%s: declares an app but ships no %s/package.json — an app is a component the unit's frontend package exports",
				m.Unit.Name, frontendLayer)
		}
	}
	return nil
}

// checkAppLabels refuses an app whose label is missing from the unit's own
// copy in any locale.
func checkAppLabels(manifests []derivedManifest, merged map[string]map[string]string) error {
	for _, m := range manifests {
		if m.Manifest.App == nil {
			continue
		}
		if err := checkCopyKey(m.Unit.Name, "app", m.Manifest.App.Label, merged); err != nil {
			return err
		}
	}
	return nil
}

// manifestApps is each unit's declared app, keyed by unit name.
func manifestApps(manifests []derivedManifest) map[string]declaredApp {
	out := make(map[string]declaredApp, len(manifests))
	for _, m := range manifests {
		if m.Manifest.App != nil {
			out[m.Unit.Name] = *m.Manifest.App
		}
	}
	return out
}
