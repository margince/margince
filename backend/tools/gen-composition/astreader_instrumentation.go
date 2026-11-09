// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading a unit's INSTRUMENTATION into the manifest: the declaration that lets
// a unit observe the whole process, which an operator reads before enabling it.

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"net/http"

	"github.com/margince/margince/backend/pkg/extension"
)

// unitInstrumentation is the operator-facing shape of an Instrumentation: which
// of its three parts the unit declares.
type unitInstrumentation struct {
	Start   bool            `json:"start,omitempty"`
	Browser string          `json:"browser,omitempty"`
	Edge    *instrumentEdge `json:"edge,omitempty"`
}

// instrumentEdge is the session-less route an Instrumentation.Edge opens,
// published with its bounds as an Inbound endpoint is.
type instrumentEdge struct {
	Path           string `json:"path"`
	MaxBody        int64  `json:"max_body"`
	PerIPPerMinute int    `json:"per_ip_per_minute"`
}

// readInstrumentation reads the &extension.Instrumentation{...} literal. Start
// and Edge are code and are only recorded as present, never evaluated. Browser
// is a literal. All three pass the published Validate the boot runs, so
// generation refuses what the boot would.
func (r *unitReader) readInstrumentation(expr ast.Expr, file *ast.File) (*unitInstrumentation, error) {
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return nil, r.errAt(expr, "Instrumentation must be a &extension.Instrumentation{...} literal")
	}
	lit, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return nil, r.errAt(expr, "Instrumentation must be a &extension.Instrumentation{...} literal")
	}
	httpPkg := importAlias(file, "net/http")
	out := &unitInstrumentation{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return nil, r.errAt(elt, "Instrumentation fields must be keyed")
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return nil, r.errAt(kv.Key, "Instrumentation fields must be keyed by name")
		}
		var err error
		switch key.Name {
		case "Start":
			out.Start = !isNilPart(kv.Value, "")
		case "Edge":
			if !isNilPart(kv.Value, httpPkg) {
				out.Edge = &instrumentEdge{
					Path:           extension.InstrumentEdgePrefix + r.unit + "/",
					MaxBody:        extension.InstrumentEdgeMaxBody,
					PerIPPerMinute: extension.InstrumentEdgePerIPPerMinute,
				}
			}
		case "Browser":
			out.Browser, err = r.stringLit(kv.Value, "Instrumentation.Browser")
		default:
			err = r.errAt(kv, "Instrumentation field %s is not derivable by this generator", key.Name)
		}
		if err != nil {
			return nil, err
		}
	}
	// Validate asks only whether the code fields are set, so a stand-in
	// carries each one the declaration names.
	decl := extension.Instrumentation{Browser: out.Browser}
	if out.Start {
		decl.Start = func(context.Context, extension.Process) (extension.Instruments, error) {
			return extension.Instruments{}, nil
		}
	}
	if out.Edge != nil {
		decl.Edge = http.NotFoundHandler()
	}
	if err := decl.Validate(); err != nil {
		return nil, r.errPos(expr, "%v", err)
	}
	return out, nil
}

// isNilPart reports whether a Start or Edge value is nil as written: a bare
// nil, in parentheses, or converted through http.Handler or http.HandlerFunc.
// A call to anything else is code that runs, so it is never read as nil.
func isNilPart(expr ast.Expr, httpPkg string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "nil"
	case *ast.ParenExpr:
		return isNilPart(e.X, httpPkg)
	case *ast.CallExpr:
		converts := isSelector(e.Fun, httpPkg, "Handler") || isSelector(e.Fun, httpPkg, "HandlerFunc")
		return converts && len(e.Args) == 1 && isNilPart(e.Args[0], httpPkg)
	}
	return false
}

// checkInstrumentFrontends refuses a unit that names a browser instrument and
// ships no frontend package: the registry would import an export nothing has.
func checkInstrumentFrontends(manifests []derivedManifest) error {
	for _, m := range manifests {
		if in := m.Manifest.Instrumentation; in != nil && in.Browser != "" && m.Unit.Frontend == nil {
			return fmt.Errorf("extensions/%s: Instrumentation.Browser names %q but the unit ships no %s/package.json — a browser instrument is a function the unit's frontend package exports",
				m.Unit.Name, in.Browser, frontendLayer)
		}
	}
	return nil
}

// manifestBrowserInstruments is each unit's browser instrument export, keyed by
// unit name.
func manifestBrowserInstruments(manifests []derivedManifest) map[string]string {
	out := map[string]string{}
	for _, m := range manifests {
		if in := m.Manifest.Instrumentation; in != nil && in.Browser != "" {
			out[m.Unit.Name] = in.Browser
		}
	}
	return out
}
