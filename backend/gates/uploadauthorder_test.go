// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// An upload route refuses an unauthorized caller BEFORE it reads the body.
//
// The gate is about ORDER, and order is the whole of it: every one of these
// routes was already authorized, and every one of them authorized too late.
// The store is where the grant decides, the store runs after the handler, and
// the handler's first act was to parse a multipart body — so a session holding
// no grant at all sent one request, made this server take a whole file apart
// and spill it to disk, and got a 403. Repeatable, and free to the sender: the
// attacker spends a request and the server spends a file.
//
// What the early check is NOT is the gate. It is deliberately allowed to be
// coarser than the store's — the attachment upload cannot even know which
// object type the body names until it has parsed it, so it asks whether the
// caller may write ANYTHING — and the store still asks the exact question
// afterwards, because the store is the gate every transport passes.
//
// Derived from the tree rather than from a list of routes: the subject is any
// function that parses a multipart body, so a seventh upload route is in the
// obligation set the moment it is written, which is the only way this stays
// true. A parse whose caller authorizes instead is waived BY NAME below, with
// the caller named, because that shape is correct and unprovable from inside
// one function.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The two ways a handler takes a multipart body apart. Both are the cost this
// gate is about: ParseMultipartForm reads the whole body, and MultipartReader
// hands back a stream the caller then reads.
var multipartParsers = []string{"ParseMultipartForm", "MultipartReader"}

// gatekit:fixture the parses whose authorization is their CALLER's, not their
// own — a waiver, and each entry names the caller that holds the grant so the
// claim can be checked rather than taken.
//
// This is the one shape the rule cannot see: a helper that only decodes is
// right to hold no grant, and whether its caller holds one is a fact about a
// different function. An entry here is a claim about that caller, so it goes
// stale the way any claim does — which is why an entry no parse reaches is
// reported below rather than left to sit.
var uploadParseAuthorizedByCaller = map[string]string{
	"decodeCompanyLogoUpload": "its only caller, uploadCompanyMark, reads the anchor company " +
		"first, and GetAnchorCompany requires company.read",
	"readImportUpload": "its only caller, stageImportSource, requires import_run.create before it calls this",
}

func TestEveryMultipartParseRefusesBeforeItReads(t *testing.T) {
	t.Parallel()
	parses := 0
	waived := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// path != "." because the root's own name is dotted, and skipping
			// it would take the whole tree — reporting nothing, for the most
			// reassuring possible reason.
			if path != "." && skipUploadAuthDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("parsing %s: %v", path, perr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			parse := firstMultipartParse(fn.Body)
			if parse == token.NoPos {
				continue
			}
			parses++
			if why, ok := uploadParseAuthorizedByCaller[fn.Name.Name]; ok {
				waived[fn.Name.Name] = true
				t.Logf("%s: %s takes its authorization from its caller — %s", path, fn.Name.Name, why)
				continue
			}
			if authorizesBefore(fn.Body, parse) {
				continue
			}
			t.Errorf("%s: %s parses a multipart body before it refuses anybody. "+
				"A caller with no grant then spends one request and makes this server "+
				"spend a whole file — parsed, spilled and thrown away — for a 403 it was "+
				"always going to get. Ask the grant first: the exact one if the object is "+
				"known here, auth.RequireAnyObject if the body is what names it. The "+
				"store's own check stays either way. If the grant genuinely belongs to "+
				"this function's caller, add it to uploadParseAuthorizedByCaller with the "+
				"caller named.", path, fn.Name.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if parses == 0 {
		t.Fatal("no multipart parse found anywhere, so this gate has stopped reading the code it derives from")
	}
	for name, why := range uploadParseAuthorizedByCaller {
		if !waived[name] {
			t.Errorf("uploadParseAuthorizedByCaller waives %s (%s) and no multipart parse is in it any more — "+
				"a stale waiver is a hole nobody can see", name, why)
		}
	}
}

// firstMultipartParse is where in this function the body starts being read, or
// NoPos if it never is.
func firstMultipartParse(body *ast.BlockStmt) token.Pos {
	at := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		for _, name := range multipartParsers {
			if selector.Sel.Name == name && (at == token.NoPos || call.Pos() < at) {
				at = call.Pos()
			}
		}
		return true
	})
	return at
}

// authorizesBefore reports whether this function calls into platform/auth
// ahead of the parse. Any of that package's gates counts: which one is the
// right question is the handler's judgement and the store's to re-ask, while
// "asked nobody" is the failure with a shape.
func authorizesBefore(body *ast.BlockStmt, parse token.Pos) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == "auth" && call.Pos() < parse {
			found = true
		}
		return !found
	})
	return found
}

func skipUploadAuthDir(name string) bool {
	return name == "node_modules" || name == "build" || name == "testdata" ||
		strings.HasPrefix(name, ".")
}
