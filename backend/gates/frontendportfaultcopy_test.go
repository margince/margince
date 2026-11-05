// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A port error that answers a reader with its own code through MessageFault
// needs catalog copy on the screen. The copy is keyed by that code
// (PROBLEM_CODE_COPY in problemcodecopy.ts). Without it the reader gets the
// server's English sentence in any language. The ports are the subject because their errors reach several
// surfaces and are written away from the screens. The codes are read from the
// MessageFault methods, so no list is kept here.

import (
	"go/ast"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const portsDir = "internal/shared/ports/"

func TestEveryPortFaultCodeHasClientCopy(t *testing.T) {
	t.Parallel()
	copied := problemCodeCopy(t)
	sent := portFaultCodes(t)
	for _, code := range sortedKeys(sent) {
		if _, ok := copied[code]; !ok {
			t.Errorf("%s answers MessageFault code %q and %s maps no copy to it, so the reader gets the "+
				"server's English detail whatever language they set; add the entry and its en, de and vi messages",
				sent[code], code, problemCopyOwner)
		}
	}
}

// portFaultCodes reads the first result of every MessageFault method under the
// ports, keyed by code, valued by one file that answers it. A code it cannot
// fold to a string fails the test, because an unread code escapes the census.
func portFaultCodes(t *testing.T) map[string]string {
	t.Helper()
	files := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(filePath string, file *ast.File) bool {
			return strings.HasPrefix(filePath, portsDir) && len(messageFaultMethods(file)) > 0
		},
	}.Files(t)
	codes := map[string]string{}
	consts := map[string]map[string]string{}
	for _, parsed := range files {
		dir := path.Dir(parsed.Path)
		if consts[dir] == nil {
			consts[dir] = gatekit.PackageStringConstants(t, filepath.FromSlash(dir))
		}
		for _, method := range messageFaultMethods(parsed.File) {
			ast.Inspect(method.Body, func(node ast.Node) bool {
				ret, ok := node.(*ast.ReturnStmt)
				if !ok || len(ret.Results) == 0 {
					return true
				}
				code, readable := gatekit.StringExpr(ret.Results[0], consts[dir], gatekit.FoldStrict)
				if !readable {
					t.Errorf("%s: a MessageFault code cannot be read; return a literal or a package constant", parsed.Path)
					return true
				}
				codes[code] = parsed.Path
				return true
			})
		}
	}
	// A port error answers a reader today, so an empty read means the walk went
	// blind rather than that the tree is clean.
	if len(codes) == 0 {
		t.Fatal("read no MessageFault codes under internal/shared/ports: the walk has gone blind")
	}
	return codes
}

func messageFaultMethods(file *ast.File) []*ast.FuncDecl {
	var methods []*ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Name.Name == "MessageFault" && fn.Body != nil {
			methods = append(methods, fn)
		}
	}
	return methods
}
