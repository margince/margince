// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every inbound endpoint a unit declares must check the signature.
//
// The core admits an inbound request. It resolves the workspace, caps the body,
// meters the sender and refuses a stale timestamp. It cannot read the unit's
// secret, so only the unit can tell whether the request is signed
// (pkg/extension/inbound.go). A unit that forgot the call accepts a request
// anybody can post. Nothing else fails: the endpoint answers, and the handler's
// tests pass with a request they built themselves.
//
// The check is one method, InboundRequest.SignatureHolds. An endpoint whose
// handler never reaches it is the defect. The gate follows each declared
// endpoint's own Handle, through the functions and methods of its package, so a
// unit that verifies one of two endpoints still fails for the other.

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	inboundEndpointType = "InboundEndpoint"
	signatureCheck      = "SignatureHolds"
)

func TestEveryInboundEndpointChecksItsSignature(t *testing.T) {
	t.Parallel()
	declared := 0
	for _, root := range []string{"../extensions", "../fixtures/extensions"} {
		units, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, unit := range units {
			if !unit.IsDir() {
				continue
			}
			for _, endpoint := range inboundEndpoints(t, filepath.Join(root, unit.Name())) {
				declared++
				if !endpoint.verifies {
					t.Errorf("%s declares the inbound endpoint %q and its handler never calls "+
						"InboundRequest.%s, so it accepts a request anybody can post: the core admits it and "+
						"cannot read the unit's secret. Call req.%s(secret) before trusting any byte of the request",
						unit.Name(), endpoint.slug, signatureCheck, signatureCheck)
				}
			}
		}
	}
	// An endpoint is what this gate exists for. Reading none means the walk no
	// longer sees them, which reports a clean answer to the one question it asks.
	if declared == 0 {
		t.Fatal("no unit declares an inbound endpoint, so the walk compared nothing: the reference " +
			"unit is gone or the declaration no longer reads as a composite literal of the endpoint type")
	}
}

// declaredEndpoint is one InboundEndpoint a unit builds and whether its handler
// reaches the signature check.
type declaredEndpoint struct {
	slug     string
	verifies bool
}

// inboundEndpoints reads every endpoint a unit's non-test code declares.
func inboundEndpoints(t *testing.T, dir string) []declaredEndpoint {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed []*ast.File
	declarations := map[string][]*ast.FuncDecl{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		parsed = append(parsed, file)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				declarations[fn.Name.Name] = append(declarations[fn.Name.Name], fn)
			}
		}
	}
	var out []declaredEndpoint
	for _, file := range parsed {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch {
			case literalOfType(lit.Type, inboundEndpointType) && isArrayType(lit.Type):
				for _, element := range lit.Elts {
					if endpoint, ok := element.(*ast.CompositeLit); ok {
						out = append(out, readEndpoint(endpoint, declarations))
					}
				}
			case literalOfType(lit.Type, inboundEndpointType):
				out = append(out, readEndpoint(lit, declarations))
			}
			return true
		})
	}
	return out
}

func isArrayType(expr ast.Expr) bool {
	_, ok := expr.(*ast.ArrayType)
	return ok
}

// readEndpoint reads one endpoint literal: its slug, and whether the handler
// it names reaches the signature check.
func readEndpoint(lit *ast.CompositeLit, declarations map[string][]*ast.FuncDecl) declaredEndpoint {
	endpoint := declaredEndpoint{slug: "(no slug)"}
	for _, element := range lit.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Slug":
			if text, ok := gatekit.LiteralText(pair.Value); ok {
				endpoint.slug = text
			}
		case "Handle":
			endpoint.verifies = reachesSignatureCheck(pair.Value, declarations, map[string]bool{})
		}
	}
	return endpoint
}

// reachesSignatureCheck follows a handler through the functions and methods of
// its package, by name, to a call of the signature check.
func reachesSignatureCheck(node ast.Node, declarations map[string][]*ast.FuncDecl, visited map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		var name string
		switch typed := n.(type) {
		case *ast.SelectorExpr:
			name = typed.Sel.Name
		case *ast.Ident:
			name = typed.Name
		default:
			return true
		}
		if name == signatureCheck {
			found = true
			return false
		}
		if visited[name] {
			return true
		}
		visited[name] = true
		for _, fn := range declarations[name] {
			if reachesSignatureCheck(fn.Body, declarations, visited) {
				found = true
			}
		}
		return !found
	})
	return found
}

// literalOfType reports whether a composite literal's type is the named one, as
// `extension.InboundEndpoint` from a unit or bare inside the package.
func literalOfType(expr ast.Expr, name string) bool {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		return typed.Sel.Name == name
	case *ast.Ident:
		return typed.Name == name
	case *ast.ArrayType:
		// `[]extension.InboundEndpoint{{...}}`: the elements elide their type, so
		// the slice literal is where it is written.
		return literalOfType(typed.Elt, name)
	}
	return false
}
