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
		built, dotImported := googleConnectorsBuiltIn(file)
		for _, hidden := range dotImported {
			t.Errorf("%s: dot-imports %s, so a construction in this file reads as a bare New( ) with no package to match on and this census cannot see it. Import it with a qualifier — a reader that cannot see a registration is how the stored app became unreachable while every test passed",
				walked, hidden)
		}
		for _, built := range built {
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

// TestTheGoogleConnectorCensusFailsClosedOnADotImport proves the arm the tree
// cannot exercise.
//
// A dot import of gmail or gcal into package compose does not compile today —
// `New` collides with compose's own — so the reader is asked directly, with a
// planted file. The arm is not dead code for that: the collision is an accident
// of two names, and a census whose blind spot is only closed by an accident is
// a census nobody should trust.
func TestTheGoogleConnectorCensusFailsClosedOnADotImport(t *testing.T) {
	t.Parallel()
	const planted = `package compose

import . "github.com/margince/margince/backend/internal/modules/capture/gmail"

func hidden() any { return New(nil, nil) }
`
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", planted, 0)
	if err != nil {
		t.Fatal(err)
	}
	builds, dotImported := googleConnectorsBuiltIn(file)
	if len(dotImported) != 1 || !strings.HasSuffix(dotImported[0], "/gmail") {
		t.Errorf("dot-imported = %v, want the gmail package reported: a construction this reader cannot see must be "+
			"named, not skipped", dotImported)
	}
	if len(builds) != 0 {
		t.Errorf("builds = %v, want none: there is no qualifier to attribute one to, which is the whole reason the "+
			"import itself is reported", builds)
	}
}

// googleConnectorBuild is one construction and the function it sits in.
type googleConnectorBuild struct {
	function  string
	connector string
}

// googleConnectorsBuiltIn names the Google connectors a file constructs, with
// the enclosing function of each, and separately the governed packages the file
// dot-imports.
//
// The qualifier comes from the file's own import block rather than from the
// package's name, so an aliased import is read as the connector it is. A DOT
// import has no qualifier at all — its construction reads as a bare `New(…)`
// with nothing to match on — so it is reported rather than skipped: a reader
// that cannot see a registration certifies nothing while reading exactly like
// a clean one.
//
// It walks the WHOLE file, not just function bodies. A package-scope
// initializer is a construction like any other, and one attributed to package
// scope can never be the registrar, so it fails — which is the right answer,
// since a registry built at init time decides reachability the same way.
func googleConnectorsBuiltIn(file *ast.File) (builds []googleConnectorBuild, dotImported []string) {
	byQualifier := map[string]string{}
	for _, importPath := range googleConnectorPackages {
		qualifier, dot := gatekit.ImportedAs(file, importPath)
		switch {
		case dot:
			dotImported = append(dotImported, importPath)
		case qualifier != "":
			byQualifier[qualifier] = path.Base(importPath)
		}
		// A blank import is neither: it cannot be called through at all.
	}
	if len(byQualifier) == 0 {
		return nil, dotImported
	}
	// packageScope is what a construction outside every function declaration is
	// attributed to. It is spelled as prose rather than as an identifier so that
	// no function can be named it and inherit the registrar's licence.
	const packageScope = "(package scope)"
	enclosing := packageScope
	seen := map[string]bool{}
	var walk func(ast.Node) bool
	walk = func(n ast.Node) bool {
		if fn, isFunc := n.(*ast.FuncDecl); isFunc {
			if fn.Body == nil {
				return false
			}
			before := enclosing
			enclosing = functionName(fn)
			ast.Inspect(fn.Body, walk)
			enclosing = before
			return false
		}
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "New" {
			return true
		}
		qualifier, isIdent := sel.X.(*ast.Ident)
		if !isIdent || byQualifier[qualifier.Name] == "" {
			return true
		}
		// One entry per function per connector: a registrar building the same
		// one twice is one decision, and reporting it twice reads as two.
		key := enclosing + ":" + byQualifier[qualifier.Name]
		if !seen[key] {
			seen[key] = true
			builds = append(builds, googleConnectorBuild{
				function: enclosing, connector: byQualifier[qualifier.Name],
			})
		}
		return true
	}
	ast.Inspect(file, walk)
	sort.Slice(builds, func(i, j int) bool {
		if builds[i].function != builds[j].function {
			return builds[i].function < builds[j].function
		}
		return builds[i].connector < builds[j].connector
	})
	return builds, dotImported
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
