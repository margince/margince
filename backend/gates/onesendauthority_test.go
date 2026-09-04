// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates_test

// The composer's preview and the send's door are one authority.
//
// activities.Handlers takes two consent seams — WithConsent for the
// default-deny door the send passes through, and WithSendPreview for the
// question a composer asks before writing. They are two readings of the same
// engine, and the whole value of the preview is that it cannot disagree with
// the send about an unchanged record.
//
// Two gates built here would be two answers. Not immediately: they would agree
// for as long as nothing about their construction differed, and then one would
// gain an injected reader the other did not — the installation country, say,
// which decides the jurisdiction windows — and the preview would start
// promising sends the engine then refuses. That failure arrives at a rep's
// keyboard, in the moment they were being careful.
//
// So this holds what the doc comment on WithSendPreview claims: newActivitiesHandlers
// passes ONE value to both.

import (
	"go/ast"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestOneAuthorityServesBothConsentSeams reads the composition root and
// requires that WithConsent and WithSendPreview receive the same identifier.
//
// The subject is derived from the source rather than named: any function that
// calls both seams is checked, so a second composition root added tomorrow is
// checked the day it appears rather than the day somebody remembers this test.
func TestOneAuthorityServesBothConsentSeams(t *testing.T) {
	// The SUBJECT is a file that wires either seam, asked the same way inside
	// the declared root and outside it — so a second composition root added in
	// another package fails the sweep rather than going unchecked.
	scope := gatekit.Scope{
		Roots: []string{"internal/compose"},
		Subject: func(_ string, file *ast.File) bool {
			return wiresEitherSeam(file)
		},
	}
	checked := 0
	for _, parsed := range scope.Files(t) {
		for _, decl := range parsed.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			consent, preview, found := seamArguments(fn)
			if !found {
				continue
			}
			checked++
			if consent != preview {
				t.Errorf("%s: %s passes %q to WithConsent and %q to WithSendPreview — "+
					"the composer's preview and the send's door must be ONE authority, or they "+
					"will eventually answer differently about the same message",
					parsed.Path, fn.Name.Name, consent, preview)
			}
		}
	}
	if checked == 0 {
		// Under-recognition is the one way this gate must not break: it would
		// read a tree where nothing wires the seams, report PASS, and there
		// would be no failing assertion to notice.
		t.Fatal("no function wires both WithConsent and WithSendPreview — either the seams were " +
			"renamed and this gate now checks nothing, or the preview lost its authority")
	}
}

// wiresEitherSeam is deliberately an OR. A file that wired only WithConsent
// would be a send path with no preview behind it, which is a different defect
// and not this gate's — but it still has to be SEEN, because a file that wires
// one seam is exactly where the second one belongs.
func wiresEitherSeam(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "WithConsent" || sel.Sel.Name == "WithSendPreview" {
			found = true
		}
		return true
	})
	return found
}

// seamArguments finds the identifier each seam is called with, in a chain like
// NewHandlers(db).WithConsent(gate).WithSendPreview(gate).
func seamArguments(fn *ast.FuncDecl) (consent, preview string, found bool) {
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		arg, ok := call.Args[0].(*ast.Ident)
		if !ok {
			// A non-identifier argument is a construction inline at the call
			// site, which cannot be the same value as anything else by
			// definition. Named so the message says so rather than reporting
			// an empty string.
			arg = ast.NewIdent("<a value constructed inline>")
		}
		switch sel.Sel.Name {
		case "WithConsent":
			consent = arg.Name
		case "WithSendPreview":
			preview = arg.Name
		}
		return true
	})
	return consent, preview, consent != "" && preview != ""
}
