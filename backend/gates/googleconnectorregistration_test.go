// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The Google connectors are registered in ONE function, because registering
// them is a decision about REACHABILITY and that decision has been wrong once.
//
// An installation can supply its Google app from either of two places: the pair
// the deployment composed, or the setting the installation stored through its
// own Settings screen. The registration used to require the first, which made
// the second unusable rather than merely unread — the transport asks the
// registry whether a connector exists before it will run the consent flow, so
// an installation that set its app through Settings was answered with the
// declared 501 and had no way to connect Gmail at all.
//
// That was fixed in newCaptureRegistryWithGoogle, and two OTHER constructors
// went on carrying the old rule: NewCaptureRegistryWithGmail passed no resolver
// at all, and GmailPollRegistry returned nothing unless the environment carried
// the pair. Neither had a production caller, so neither could cause the defect
// — and both were exactly what somebody wiring up a poller next would have
// reached for, with tests already passing over the old rule to reassure them.
// They are gone; this is what stops the third one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// googleConnectorRegistrar is the one function that may register them.
//
// Held by: TestOnlyOneFunctionRegistersTheGoogleConnectors
// (backend/gates/googleconnectorregistration_test.go) — the census below is
// what makes this a fact rather than a note, and it fails naming the second
// function the moment one appears.
const googleConnectorRegistrar = "newCaptureRegistryWithGoogle"

// googleConnectorPackages are the connector constructors whose registration is
// governed by the reachability rule. graph is deliberately absent: it answers
// to a Microsoft app with a rule of its own, and folding the two together would
// be a third spelling of a question neither is asking.
var googleConnectorPackages = map[string]bool{"gmail": true, "gcal": true}

// TestOnlyOneFunctionRegistersTheGoogleConnectors is the census.
func TestOnlyOneFunctionRegistersTheGoogleConnectors(t *testing.T) {
	t.Parallel()
	found := 0
	err := filepath.WalkDir("internal/compose", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
			return err
		}
		path = filepath.ToSlash(path)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			for _, connector := range googleConnectorsRegisteredIn(fn) {
				found++
				if fn.Name.Name == googleConnectorRegistrar {
					continue
				}
				t.Errorf("%s: %s registers the %s connector — the rule for WHETHER it may be registered lives in %s, and a second place to decide it is how the stored app became unreachable while every test passed. Build the registry through %s and add what this needs to it",
					path, fn.Name.Name, connector, googleConnectorRegistrar, googleConnectorRegistrar)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Both connectors, or the reader has stopped seeing registrations rather
	// than the tree having lost them — and a census that finds none certifies
	// nothing while reading exactly like a clean one.
	if found < len(googleConnectorPackages) {
		t.Fatalf("this census found %d Google connector registration(s) and expects at least %d — the reader has gone quiet",
			found, len(googleConnectorPackages))
	}
}

// googleConnectorsRegisteredIn names the Google connectors a function registers.
//
// The REGISTRATION is what is judged, not the construction. connectorapp.go
// builds a gmail.Connector to call Authenticate on it, which is a use rather
// than an offer: the registry is what the transport reads to decide whether the
// consent flow may run at all, so putting one INTO it is the act with the
// reachability question attached.
func googleConnectorsRegisteredIn(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Register" {
			return true
		}
		for _, arg := range call.Args {
			if connector := googleConnectorConstructed(arg); connector != "" {
				out = append(out, connector)
			}
		}
		return true
	})
	return out
}

// googleConnectorConstructed names the Google connector an expression builds,
// or "" for anything else. It looks THROUGH the option chain a registration
// wraps its connector in — `gmail.New(…).WithBounceSink(…)` is still a gmail
// registration, and a reader that only matched the outermost call would see a
// method name and pass.
func googleConnectorConstructed(expr ast.Expr) string {
	found := ""
	ast.Inspect(expr, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "New" {
			return true
		}
		if pkg, isIdent := sel.X.(*ast.Ident); isIdent && googleConnectorPackages[pkg.Name] {
			found = pkg.Name
		}
		return found == ""
	})
	return found
}
