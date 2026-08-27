// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A nil version is the request's answer, never the call site's.
//
// storekit.Patch.ApplyGuarded takes an *int64 If-Match, and a nil one drops the
// version clause: the write then locks the row and applies, which is serialized
// but is not the compare-and-set the caller's signature suggests it might be.
// The contract keeps If-Match OPTIONAL (data-model §1.3a), so that path has to
// exist — a client that sends no precondition still gets a write.
//
// What must not exist is a call site that writes the nil itself. The two spell
// the same thing and mean opposite things: one is a client declining a
// precondition, and the other is a programmer who had no version and reached for
// the argument that compiled. Nothing distinguishes them at the call site, which
// is how optimistic locking stayed opt-in on a seam whose whole purpose is to
// make it the default (issue #1505).
//
// So the pointer must come from somewhere — a request field, a variable, a
// parameter — and never from the `nil` keyword. A call site with genuinely no
// version has an honest spelling already: take the lock by name with
// storekit.LockRow and write through the witness with ApplyLocked, which is what
// ApplyGuarded(nil) does anyway and says so in the types.
//
// A prohibition rather than a census, and the difference matters here: the rule
// is about a literal that must appear nowhere, so a walk that missed a file
// would report the same green as a tree that has none. The census half is the
// sweep itself — every file in internal/ that names the seam is parsed, and a
// file naming it outside the roots fails the scope rather than being skipped.

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The seams that take an If-Match pointer. Both, because ApplyGuardedIn is the
// same door with the archive filter spelled out, and a rule that bound only the
// short one would be one refactor from meaning nothing.
var guardedApplyDoors = map[string]bool{
	"ApplyGuarded":   true,
	"ApplyGuardedIn": true,
}

// ifVersionArg is the pointer's position: (ctx, tx, table, id, ifVersion, …).
const ifVersionArg = 4

func TestNoCallSiteWritesItsOwnNilVersion(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileCallsAGuardedApply,
		// Not the extension tier: extensions/ are separate modules that cannot
		// import storekit at all, so a unit reaches a guarded write only through
		// a wrapper inside this root.
	}.Files(t)

	sites := 0
	for _, parsed := range files {
		for _, site := range guardedApplySitesIn(parsed) {
			sites++
			if site.versionIsNilLiteral {
				t.Errorf("%s: %s is handed a literal nil version.\n"+
					"\tA nil here drops the If-Match clause, so this write is last-writer-wins "+
					"against anything the row lock does not cover — and it reads as a caller who "+
					"HAD a version and declined it.\n"+
					"\tIf there is genuinely no version to compare (the wire takes no If-Match, or "+
					"the table has no version column), say so in the types: storekit.LockRow for "+
					"the lock and Patch.ApplyLocked through its witness.",
					site.key, site.door)
			}
		}
	}
	// A prohibition that swept nothing passes for the wrong reason. The seam has
	// call sites in every module that serves a client-driven update, so a zero
	// here is a broken walk rather than a clean tree.
	if sites == 0 {
		t.Fatal("no ApplyGuarded call site was found at all, so this gate judged nothing — " +
			"the walk is broken, or the seam was renamed and this rule now binds a name nobody calls")
	}
}

// guardedApplySite is one call, keyed for a message a reader can open.
type guardedApplySite struct {
	key                 string
	door                string
	versionIsNilLiteral bool
}

func fileCallsAGuardedApply(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && guardedApplyDoors[sel.Sel.Name] {
			found = true
		}
		return true
	})
	return found
}

func guardedApplySitesIn(parsed gatekit.ParsedFile) []guardedApplySite {
	// The DECLARATION is out of scope: storekit's own signature is where the
	// pointer is allowed to be nil, and the forward from ApplyGuarded into
	// ApplyGuardedIn is that same argument travelling one frame.
	if strings.Contains(filepath.ToSlash(parsed.Path), "/platform/database/storekit/") {
		return nil
	}
	var sites []guardedApplySite
	for _, decl := range parsed.File.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !guardedApplyDoors[sel.Sel.Name] || len(call.Args) <= ifVersionArg {
				return true
			}
			sites = append(sites, guardedApplySite{
				key:                 parsed.Path + ":" + fn.Name.Name,
				door:                sel.Sel.Name,
				versionIsNilLiteral: isNilIdent(call.Args[ifVersionArg]),
			})
			return true
		})
	}
	return sites
}
