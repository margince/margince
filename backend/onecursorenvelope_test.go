// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// A keyset cursor travels in one envelope, storekit's.
//
// Four modules wrote their own — the dedupe queue, the lead work queue, the
// search ranker and the overlay mirror — and the KEYSETS differing is exactly
// why they did. A dedupe queue pages by confidence, a lead queue by SLA band, a
// search by relevance score, a mirror by external id. Those are four genuine
// positions. The envelope around them was not four things: base64url, a
// payload, and one refusal for "the client sent a token we did not mint" —
// written four times, and the search ranker's payload was a `|`-delimited
// string rather than JSON, so an entity type containing a pipe would have taken
// the cursor apart at the wrong place.
//
// THE SUBJECT IS THE PAIRING, not base64 on its own. This tree base64s a JWT
// segment, a signed connector state and a key-vault token, and none of those is
// a cursor; a census that reported them would be waived away in a week. What
// says "this function is decoding a client's continuation token" is base64
// decoding IN THE SAME FUNCTION as a MalformedCursorError — the error is the
// tell, because it is the one refusal that means "not a token we minted".
//
// WHAT IT CANNOT SEE. A module that decoded a cursor and returned some other
// error would be outside the net — but that is the defect the error type exists
// to prevent and httperr's own gates already judge, so it is covered elsewhere
// rather than not at all. Nor can it see an envelope built without base64.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// cursorEnvelopeScope claims the envelope lives in storekit and nowhere else.
// Nothing is exempt: every module that pages a store already imports storekit
// for ClampLimit and Page, so every one of them can reach DecodeOpaque.
var cursorEnvelopeScope = gatekit.Scope{
	Roots:   []string{"internal/platform/database/storekit"},
	Subject: unwrapsACursorItself,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestTheCursorEnvelopeIsSpelledOnce(t *testing.T) {
	inside := cursorEnvelopeScope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("a cursor envelope is opened in %d files inside the package that owns it:\n\t%s\n\n"+
			"One envelope, so a token that is not ours is refused the same way everywhere and a payload "+
			"cannot be taken apart at the wrong character. Call storekit.DecodeOpaque with your own "+
			"position type, and keep the check that the position is a POSITION where the knowledge is",
			len(inside), strings.Join(where, "\n\t"))
	}
}

// unwrapsACursorItself reports whether some function both base64-decodes and
// raises a malformed-cursor refusal — the pairing that means it is opening a
// client's continuation token rather than reading a JWT or a signed state.
func unwrapsACursorItself(_ string, file *ast.File) bool {
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if decodesBase64(fn.Body) && raisesMalformedCursor(fn.Body) {
			found = true
			break
		}
	}
	return found
}

// decodesBase64 matches `<anything>.DecodeString(…)`. The receiver is not
// pinned to a named encoding: RawURLEncoding is what this tree uses, and a
// module that reached for StdEncoding would be rolling the same envelope
// slightly differently, which is the thing rather than an exception to it.
func decodesBase64(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.SelectorExpr); ok && fn.Sel.Name == "DecodeString" {
			found = true
		}
		return !found
	})
	return found
}

// raisesMalformedCursor matches the composite literal `MalformedCursorError{}`,
// however the package it lives in is named locally.
func raisesMalformedCursor(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch named := lit.Type.(type) {
		case *ast.SelectorExpr:
			found = named.Sel.Name == "MalformedCursorError"
		case *ast.Ident:
			found = named.Name == "MalformedCursorError"
		}
		return !found
	})
	return found
}

// TestTheCursorEnvelopeCensusStillSeesItsSubject is the vacuity check: a census
// that has stopped matching passes by finding nothing, which is the same word
// it prints over a clean tree.
func TestTheCursorEnvelopeCensusStillSeesItsSubject(t *testing.T) {
	subjects := map[string]string{
		"a hand-rolled envelope, as all four were written": "" +
			"package p\nfunc f(token string) error {\n\traw, err := base64.RawURLEncoding.DecodeString(token)\n" +
			"\tif err != nil {\n\t\treturn &storekit.MalformedCursorError{}\n\t}\n\t_ = raw\n\treturn nil\n}",
		"the same, through a differently named import": "" +
			"package p\nfunc f(token string) error {\n\traw, err := b64.StdEncoding.DecodeString(token)\n" +
			"\tif err != nil {\n\t\treturn &kit.MalformedCursorError{}\n\t}\n\t_ = raw\n\treturn nil\n}",
	}
	for name, body := range subjects {
		if !unwrapsACursorItself("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"a JWT segment, which is base64 and not a cursor": "" +
			"package p\nfunc f(seg string) error {\n\t_, err := base64.RawURLEncoding.DecodeString(seg)\n" +
			"\treturn err\n}",
		"a module VALIDATING a decoded position, which is where that check belongs": "" +
			"package p\nfunc f(c cursor) error {\n\tif c.ID.IsZero() {\n\t\treturn &storekit.MalformedCursorError{}\n\t}\n\treturn nil\n}",
		"the two in one FILE but not one function, which is two unrelated jobs": "" +
			"package p\nfunc a(seg string) { _, _ = base64.RawURLEncoding.DecodeString(seg) }\n" +
			"func b(c cursor) error { return &storekit.MalformedCursorError{} }",
	}
	for name, body := range nearMisses {
		if unwrapsACursorItself("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census claims %s opens a cursor envelope", name)
		}
	}
}
