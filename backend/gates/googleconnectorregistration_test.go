// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The Google connectors are built for the registry in ONE function, because
// putting one INTO the registry is a decision about REACHABILITY and that
// decision has been wrong once.
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
//
// The CONSTRUCTION is what this reads, not the Register call it is passed to.
// A registration can hoist its connector into a local, hand it to a helper, or
// reach the registry through a chain of them, and a reader that followed the
// argument would have to be a dataflow analysis to see any of that — one that
// answers "no Google connector here" for every shape it has not been taught.
// A construction cannot be hidden the same way: a second registrar has to build
// the connector somewhere, and that somewhere is a syntactic fact. The price is
// that a construction which is NOT a registration has to be named below.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// googleConnectorRegistrar is the one function that may build them for the
// registry.
//
// Held by: TestOnlyOneFunctionRegistersTheGoogleConnectors
// (backend/gates/googleconnectorregistration_test.go) — the census below is
// what makes this a fact rather than a note, and it fails naming the second
// function the moment one appears.
const googleConnectorRegistrar = "newCaptureRegistryWithGoogle"

// googleConnectorPackages are the connector constructors whose registration is
// governed by the reachability rule, by import path so that an alias is read
// the same as the plain name. graph is deliberately absent: it answers to a
// Microsoft app with a rule of its own, and folding the two together would be a
// third spelling of a question neither is asking.
var googleConnectorPackages = []string{
	"github.com/margince/margince/backend/internal/modules/capture/gmail",
	"github.com/margince/margince/backend/internal/modules/capture/gcal",
}

// builtForSomethingOtherThanTheRegistry names the constructions that are a USE
// rather than an offer. The registry is what the transport reads to decide
// whether the consent flow may run at all, so a connector built to call a
// method on it and thrown away carries no reachability decision.
var builtForSomethingOtherThanTheRegistry = gatekit.Waive(map[string]string{
	"internal/compose/connectorapp.go:connectorHandlers.gmailApp": "builds a connector to call Authenticate on it and drops it; the registry never sees this one, so it decides nothing about whether Gmail is reachable",
	"internal/compose/connectorapp.go:connectorHandlers.gcalApp":  "builds a connector to call Authenticate on it and drops it; the registry never sees this one, so it decides nothing about whether Calendar is reachable",
})

// TestOnlyOneFunctionRegistersTheGoogleConnectors is the census.
func TestOnlyOneFunctionRegistersTheGoogleConnectors(t *testing.T) {
	t.Parallel()
	// Distinct connectors, not a count: one connector built twice reads as two
	// registrations, and a census that certified on the total would pass a tree
	// that had lost Calendar entirely.
	inRegistrar := map[string]bool{}
	err := filepath.WalkDir("internal/compose", func(walked string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(walked, ".go") ||
			strings.HasSuffix(walked, "_test.go") || isIntegrationTagged(walked) {
			return err
		}
		walked = filepath.ToSlash(walked)
		file, err := parser.ParseFile(token.NewFileSet(), walked, nil, 0)
		if err != nil {
			return err
		}
		for _, built := range googleConnectorsBuiltIn(file) {
			switch {
			case built.function == googleConnectorRegistrar:
				inRegistrar[built.connector] = true
			case builtForSomethingOtherThanTheRegistry.Waived(t, walked+":"+built.function):
			default:
				t.Errorf("%s: %s builds the %s connector — the rule for WHETHER it may be registered lives in %s, and a second place to decide it is how the stored app became unreachable while every test passed. Build the registry through %s and add what this needs to it, or name this one in builtForSomethingOtherThanTheRegistry if the registry never sees it",
					walked, built.function, built.connector, googleConnectorRegistrar, googleConnectorRegistrar)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	builtForSomethingOtherThanTheRegistry.AssertAllMatched(t)
	// Every connector, or the reader has stopped seeing constructions rather
	// than the tree having lost them — and a census that finds none certifies
	// nothing while reading exactly like a clean one.
	for _, importPath := range googleConnectorPackages {
		if connector := path.Base(importPath); !inRegistrar[connector] {
			t.Fatalf("this census did not find the %s connector built in %s — either the registrar has stopped building it or the reader has gone quiet, and it cannot tell the difference",
				connector, googleConnectorRegistrar)
		}
	}
}

// googleConnectorBuild is one construction and the function it sits in.
type googleConnectorBuild struct {
	function  string
	connector string
}

// googleConnectorsBuiltIn names the Google connectors a file constructs, with
// the enclosing function of each. The qualifier comes from the file's own
// import block rather than from the package's name, so an aliased import is
// read as the connector it is.
func googleConnectorsBuiltIn(file *ast.File) []googleConnectorBuild {
	byQualifier := map[string]string{}
	for _, importPath := range googleConnectorPackages {
		// A dot import has no qualifier to match on, and a blank one cannot be
		// called through at all; ImportedAs reports both as an empty name.
		if qualifier, _ := gatekit.ImportedAs(file, importPath); qualifier != "" {
			byQualifier[qualifier] = path.Base(importPath)
		}
	}
	if len(byQualifier) == 0 {
		return nil
	}
	var out []googleConnectorBuild
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		for _, connector := range googleConnectorsConstructedIn(fn.Body, byQualifier) {
			out = append(out, googleConnectorBuild{function: functionName(fn), connector: connector})
		}
	}
	return out
}

// googleConnectorsConstructedIn names the connectors a body builds, sorted so
// that a function building both reports them in a settled order.
func googleConnectorsConstructedIn(body *ast.BlockStmt, byQualifier map[string]string) []string {
	seen := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "New" {
			return true
		}
		if qualifier, isIdent := sel.X.(*ast.Ident); isIdent && byQualifier[qualifier.Name] != "" {
			seen[byQualifier[qualifier.Name]] = true
		}
		return true
	})
	var out []string
	for connector := range seen {
		out = append(out, connector)
	}
	sort.Strings(out)
	return out
}

// functionName spells a method as Receiver.Name, so that two files declaring
// the same method name on different types cannot share a waiver.
func functionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := fn.Recv.List[0].Type
	if star, isStar := receiver.(*ast.StarExpr); isStar {
		receiver = star.X
	}
	if ident, isIdent := receiver.(*ast.Ident); isIdent {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
