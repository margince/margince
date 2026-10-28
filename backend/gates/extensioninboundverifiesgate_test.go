// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A unit that declares an inbound endpoint must check the signature.
//
// The core admits an inbound request. It resolves the workspace, caps the body,
// meters the sender and refuses a stale timestamp. It cannot read the unit's
// secret, so only the unit can tell whether the request is signed
// (pkg/extension/inbound.go). A unit that forgot the call accepts a request
// anybody can post. Nothing else fails: the endpoint answers, and the handler's
// tests pass with a request they built themselves.
//
// The check is one method, InboundRequest.SignatureHolds. A unit that declares
// an InboundEndpoint and never calls it is the defect. The gate asks each unit,
// staged fixtures included, for both facts and compares them.

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

func TestEveryUnitThatDeclaresAnInboundEndpointChecksItsSignature(t *testing.T) {
	t.Parallel()
	declaring := 0
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
			declares, checks := inboundFacts(t, filepath.Join(root, unit.Name()))
			if !declares {
				continue
			}
			declaring++
			if !checks {
				t.Errorf("%s declares an %s and never calls InboundRequest.%s, so its endpoint accepts "+
					"a request anybody can post: the core admits it and cannot read the unit's secret. "+
					"Call req.%s(secret) before trusting any byte of the request",
					unit.Name(), inboundEndpointType, signatureCheck, signatureCheck)
			}
		}
	}
	// A unit that declares an endpoint is what this gate exists for. Reading
	// none means the walk no longer sees them, which reports a clean answer to
	// the one question it asks.
	if declaring == 0 {
		t.Fatal("no unit declares an inbound endpoint, so the walk compared nothing: the reference " +
			"unit is gone or the declaration no longer reads as a composite literal of the endpoint type")
	}
}

// inboundFacts says whether a unit's non-test code builds an InboundEndpoint and
// whether it calls the signature check.
func inboundFacts(t *testing.T, dir string) (declares, checks bool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				declares = declares || literalOfType(node.Type, inboundEndpointType)
			case *ast.SelectorExpr:
				checks = checks || node.Sel.Name == signatureCheck
			}
			return true
		})
	}
	return declares, checks
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
