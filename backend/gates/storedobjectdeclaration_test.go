// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A writer that puts bytes into the object store declares them first, or says why its key needs no declaration.
//
// A put and the row that owns it are two writes to two systems that cannot be made
// atomic. An erasure walks ROWS, so bytes nothing references are bytes no erasure
// reaches — and nothing fails, because an orphan looks exactly like an object in use
// until somebody counts the bucket.
//
// Derived from the CALL SHAPE rather than from a list, because a list is only as
// complete as whoever last read the tree: walking blobstore.Store.Put's
// five-argument shape finds every writer, and rules out the secret vault's unrelated
// three-argument Put by arity.
//
// ORDER, not merely presence. A declaration after the put leaves exactly the window
// the ledger exists to close, so the positions are compared rather than counted.
func TestEveryStoredObjectWriterDeclaresItsIntent(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	puts := map[string]int{}
	declaring := map[string]int{}
	helperCalls := map[string]map[string]int{}
	for _, file := range goSourceFiles(t, root) {
		rel := strings.TrimPrefix(strings.TrimPrefix(file, root), "/")
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/platform/blobstore/") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		for _, fn := range functionsIn(parsed) {
			subject := rel + ":" + fn.Name.Name
			if at := firstCall(fn, putsBytes); at > 0 {
				puts[subject] = at
			}
			if at := firstCall(fn, declaresIntent); at > 0 {
				declaring[subject] = at
			}
			for _, helper := range putHelpers.Subjects() {
				name := strings.SplitN(helper, ":", 2)[1]
				if at := firstCall(fn, callsNamed(name)); at > 0 {
					if helperCalls[helper] == nil {
						helperCalls[helper] = map[string]int{}
					}
					helperCalls[helper][subject] = at
				}
			}
		}
	}
	// Under-recognition is the one way this gate must not break: a walk that finds no
	// put at all reports PASS, and the hazard it exists for is invisible either way.
	if len(puts) < 5 {
		t.Fatalf("found %d object-store writers, fewer than the five this gate was written against: "+
			"the call-shape walk has drifted", len(puts))
	}
	for put, putAt := range puts {
		if at, ok := declaring[put]; ok && at < putAt {
			continue
		}
		if at, ok := declaring[put]; ok {
			t.Errorf("%s declares the key provisional at offset %d, AFTER its put at %d — a "+
				"declaration that lands after the bytes leaves exactly the window the ledger closes",
				put, at, putAt)
			continue
		}
		if putHelpers.Waived(t, put) {
			// The helper takes a bucket and bytes and no database handle, so its
			// CALLERS are held to it — every one found by the same walk, never a list
			// here that a new caller could be absent from.
			callers := helperCalls[put]
			if len(callers) == 0 {
				t.Errorf("%s is declared a put helper and nothing calls it: the exemption answers "+
					"for nobody, so a put reaches the bucket with no declaration anywhere", put)
			}
			for caller, callAt := range callers {
				at, ok := declaring[caller]
				if !ok || at > callAt {
					t.Errorf("%s calls %s without declaring the key provisional first: the helper "+
						"is exempt only because its callers do it", caller, put)
				}
			}
			continue
		}
		// Asked here, where the offence is settled: a put that declares nothing and
		// has no caller standing in for it.
		if putsNoNewObject.Waived(t, put) {
			continue
		}
		t.Errorf("%s puts bytes into the object store without declaring the key provisional first — "+
			"a failure between the put and the row that owns them leaves bytes no erasure can reach, "+
			"and nothing reports it. Record the intent, or declare the writer in putsNoNewObject with "+
			"the reason its key needs none", put)
	}
	putsNoNewObject.AssertAllMatched(t)
	putHelpers.AssertAllMatched(t)
}

// putHelpers: a function that puts bytes for whoever calls it, and cannot declare.
//
// PutLogo takes a bucket, a key base and bytes — no database handle reaches it. Its
// callers are DISCOVERED rather than listed, so a new one joins the obligation by
// existing; what is declared here is only that the helper itself is not the writer.
var putHelpers = gatekit.Waive(map[string]string{
	"internal/modules/contacts/companylogostore.go:PutLogo": "takes a bucket, a key base and bytes, so the declaration belongs to whoever mints the base. Its callers are discovered rather than listed, and each is held to declaring before its call",
})

// putsNoNewObject: a put that cannot orphan anything, each with its reason.
//
// Both entries write to a key something ALREADY references, which is the one shape an
// intent would make worse rather than better — the ledger would offer the reaper a
// key a live row still names.
var putsNoNewObject = gatekit.Waive(map[string]string{
	"internal/modules/contacts/handlers_companylogo.go:writeBackTrimmedLogo": "writes the trimmed crop back to the SAME key it read, which CompanyLogoKey resolved off the company row — so the object is already referenced and an intent would declare a live mark provisional. The crop is an optimisation of bytes that are already the record's",
	"internal/modules/privacy/suppressionjournal.go:ExportSuppressions":      "the object IS the record. journalSuppressed reads it back by a key computed from (kind, hash), and no row carries that key or ever will — so an intent would be a key the sweep finds unreferenced one grace period later and deletes the live journal",
})

// recordIntentWrapper matches a module's own named wrapper around the ledger.
var recordIntentWrapper = regexp.MustCompile(`^[Rr]ecord\w*Intent$`)

// putsBytes matches the object store's five-argument Put.
//
// The ARITY tells it apart from the secret vault's own Put, which takes three: a
// selector-name match alone counts a store the ledger has nothing to do with.
func putsBytes(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Put" && len(call.Args) == 5
}

// declaresIntent matches a call that records an intent on the ledger.
//
// The RECEIVER is what makes it the ledger's: `Record` alone is a common method name,
// and accepting every one of them would let a writer that records something else
// entirely pass this gate. Either the receiver names the ledger, or the call is a
// module's own Record…Intent wrapper, whose body is held to the same test under its
// own name.
func declaresIntent(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if recordIntentWrapper.MatchString(sel.Sel.Name) {
		return true
	}
	if sel.Sel.Name != "Record" {
		return false
	}
	receiver := receiverText(sel.X)
	return strings.Contains(receiver, "storedobjects") || strings.Contains(receiver, "Ledger") ||
		strings.Contains(receiver, "ledger") || strings.Contains(receiver, "Intents")
}

// callsNamed matches a call to one named function, however it is qualified.
func callsNamed(name string) func(*ast.CallExpr) bool {
	return func(call *ast.CallExpr) bool {
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			return fun.Sel.Name == name
		case *ast.Ident:
			return fun.Name == name
		}
		return false
	}
}

// receiverText renders a call's receiver expression well enough to recognise the
// ledger in it: an identifier, a field, or the constructor called inline.
func receiverText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return receiverText(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		return receiverText(e.Fun)
	}
	return ""
}

// firstCall answers the source position of the first matching call in fn, or -1.
func firstCall(fn *ast.FuncDecl, matches func(*ast.CallExpr) bool) int {
	found := -1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found >= 0 {
			return found < 0
		}
		if matches(call) {
			found = int(call.Pos())
		}
		return true
	})
	return found
}
