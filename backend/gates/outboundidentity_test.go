// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// A remote operator sees one name for this product and decides about it: blocks
// it, rate-limits it, allow-lists it, or writes a robots.txt group naming it.
//
// That makes the name an interface with people outside this codebase, and it is
// the only interface here whose other side cannot be asked what it meant. A
// second spelling means a decision an operator made is silently not in force
// for the calls their rule did not name — and they have no way to discover that
// except by the traffic they thought they had stopped still arriving.
//
// THE RULE: an outbound User-Agent comes from an `outbound.Agent`, never from a
// literal at the call site.
//
// This does NOT ask every caller to send the SAME token. A crawler, a geocoder
// and a webhook delivery are different things to be allowed or refused
// separately, and collapsing them would take away a distinction operators use.
// What it asks is that each token exist once, in one place, in one format.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// identitySurfaceRoots are the trees that can make an outbound call.
var identitySurfaceRoots = []string{
	"internal", "cmd", "tools", "../extensions", "../cli", "../desktop", "../fixtures",
}

func TestNoOutboundIdentityIsWrittenAtItsCallSite(t *testing.T) {
	t.Parallel()
	var findings []string
	headerWrites := 0
	// Gathered per PACKAGE, not per file. A constant is package-scoped, so
	// `const ua = "…"` in one file and the write in another is the same alias
	// the single-file map could not see — and splitting the two across files is
	// no harder than putting them side by side.
	byDir := map[string][]*ast.File{}
	paths := map[*ast.File]string{}
	fset := token.NewFileSet()
	for _, root := range identitySurfaceRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "testdata" || name == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			byDir[dir] = append(byDir[dir], file)
			paths[file] = filepath.ToSlash(path)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	for _, files := range byDir {
		// The header NAME and its VALUE can both be constants — one line up, or
		// one file over.
		consts := stringConstants(files)
		for _, file := range files {
			path := paths[file]
			ast.Inspect(file, func(node ast.Node) bool {
				switch written := node.(type) {
				case *ast.CallExpr:
					// Header().Set(name, value) and Header().Add(name, value).
					if len(written.Args) != 2 || !writesAHeader(written) {
						return true
					}
					name, isString := gatekit.StringExpr(written.Args[0], consts, gatekit.FoldStrict)
					if !isString || !strings.EqualFold(name, "User-Agent") {
						return true
					}
					headerWrites++
					if _, literal := gatekit.StringExpr(written.Args[1], consts, gatekit.FoldStrict); literal {
						findings = append(findings, path)
					}
				case *ast.CompositeLit:
					// http.Header{"User-Agent": {"x"}} — the whole map written
					// at once, which no assignment to an index appears on.
					for _, element := range written.Elts {
						pair, isPair := element.(*ast.KeyValueExpr)
						if !isPair {
							continue
						}
						name, isString := gatekit.StringExpr(pair.Key, consts, gatekit.FoldStrict)
						if !isString || !strings.EqualFold(name, "User-Agent") {
							continue
						}
						headerWrites++
						if holdsALiteral(pair.Value, consts) {
							findings = append(findings, path)
						}
					}
				case *ast.AssignStmt:
					// req.Header["User-Agent"] = []string{...}, which reaches the
					// same map by a route no method call appears on.
					for i, target := range written.Lhs {
						index, isIndex := target.(*ast.IndexExpr)
						if !isIndex {
							continue
						}
						name, isString := gatekit.StringExpr(index.Index, consts, gatekit.FoldStrict)
						if !isString || !strings.EqualFold(name, "User-Agent") {
							continue
						}
						headerWrites++
						if i < len(written.Rhs) && holdsALiteral(written.Rhs[i], consts) {
							findings = append(findings, path)
						}
					}
				}
				return true
			})
		}
	}
	// A census that finds no outbound identity at all is judging nothing.
	if headerWrites < 3 {
		t.Fatalf("the census found only %d User-Agent writes, so it is not reading the tree", headerWrites)
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d call site(s) write an outbound User-Agent as a literal.\n\n"+
			"The token is an interface with a remote operator, and the one interface here whose "+
			"other side cannot be asked what it meant: a second spelling means a rule they wrote is "+
			"not in force for the calls it did not name, and the only way they find out is the "+
			"traffic they thought they had stopped. Declare it as an outbound.Agent and send "+
			"Header().\n\n\t%s", len(findings), strings.Join(findings, "\n\t"))
	}
}

// writesAHeader matches both ways a header is written through the map's own
// methods. `Add` merges rather than replaces, which is a different operation and
// exactly the same disclosure.
func writesAHeader(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && (selector.Sel.Name == "Set" || selector.Sel.Name == "Add")
}

// holdsALiteral reports whether a value assigned to the header map is written
// out at the call site, including inside the []string a direct assignment takes.
func holdsALiteral(value ast.Expr, consts map[string]string) bool {
	if _, literal := gatekit.StringExpr(value, consts, gatekit.FoldStrict); literal {
		return true
	}
	composite, ok := value.(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, element := range composite.Elts {
		if _, literal := gatekit.StringExpr(element, consts, gatekit.FoldStrict); literal {
			return true
		}
	}
	return false
}
