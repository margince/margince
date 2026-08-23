// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A page token a caller hands back is either one this server minted or it is
// not, and that is ONE question with one answer on the wire: the contract's
// `422 code: malformed_cursor`, which tells the caller to re-issue the request
// without the token.
//
// A module that invents its own refusal answers a different question. The two
// spellings this tree carried said `required` — of a field the caller had just
// supplied, so acting on it means sending the token again — and `invalid`, which
// names no remedy at all. Neither is reachable from the contract, so a client
// written against the contract cannot recognise either.
//
// THE RULE: a refusal about a cursor is `storekit.MalformedCursorError`.
// `platform/httperr` maps that one type to the contract's code, and mapping is
// the only place the wire code is decided.
//
// Two arms, because the site that escaped the first sweep escaped it by not
// having the shape the first arm reads. `people/dedupequeue.go` decodes base64
// rather than parsing a UUID, and it was found only when a test named the error
// it expected. A census keyed on one spelling of "parse" would have reported a
// clean tree over it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// connectorCursors resume a CONNECTOR, not a caller's page. The server mints
// these and reads its own back; no client ever supplies one, so there is no
// request to refuse and no contract code to answer with. Ratified by name
// because the distinction is what the token IS, which its shape does not say —
// both are bytes carrying JSON.
var connectorCursors = gatekit.Waive(map[string]string{
	"internal/modules/capture/backfillpager.go:backfillPageCursor":   "reads the provider page token stored on a backfill run, handed back to the provider's own API. A caller never sends it, and an unreadable one is a server-side fault the run reports, not a 422.",
	"internal/modules/capture/offlinedemo/offlinedemo.go:readCursor": "reads the demo generator's own resume state and returns no error at all — an unreadable or stale-generation cursor restarts the generator rather than refusing anything.",
})

// refusalSurfaceRoots are the trees where a cursor may be refused.
var refusalSurfaceRoots = []string{"internal/modules", "internal/compose", "../extensions"}

// cursorDecoders are the primitives a cursor is read WITH. A function calling
// one of these on a cursor is deciding whether the token is well-formed, which
// is the decision this file governs.
var cursorDecoders = []string{"Parse", "DecodeString", "Unmarshal", "DecodeCursor"}

// refusalFile is one parsed product file with its package's string constants,
// so a field named through a constant (`Field: fieldCursor`) reads the same as
// one named literally.
type refusalFile struct {
	path   string
	file   *ast.File
	consts map[string]string
}

func refusalFiles(t *testing.T) []refusalFile {
	t.Helper()
	byDir := map[string][]*ast.File{}
	paths := map[*ast.File]string{}
	fset := token.NewFileSet()
	for _, root := range refusalSurfaceRoots {
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
			parsed, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			byDir[dir] = append(byDir[dir], parsed)
			paths[parsed] = filepath.ToSlash(path)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	var out []refusalFile
	for _, files := range byDir {
		consts := stringConstants(files)
		for _, f := range files {
			out = append(out, refusalFile{path: paths[f], file: f, consts: consts})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// namesACursorField reports whether a composite literal binds a field whose
// value is the string "cursor" — the shape an error uses to say WHICH argument
// it is refusing.
func namesACursorField(lit *ast.CompositeLit, consts map[string]string) bool {
	for _, element := range lit.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || !strings.EqualFold(key.Name, "field") {
			continue
		}
		if text, resolved := stringValue(pair.Value, consts); resolved && text == "cursor" {
			return true
		}
	}
	return false
}

func literalTypeName(lit *ast.CompositeLit) string {
	switch typ := lit.Type.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		if pkg, ok := typ.X.(*ast.Ident); ok {
			return pkg.Name + "." + typ.Sel.Name
		}
		return typ.Sel.Name
	}
	return ""
}

// TestARefusalAboutACursorIsTheContractsRefusal is the first arm: nothing but
// storekit's type may name a cursor as the field it refuses.
func TestARefusalAboutACursorIsTheContractsRefusal(t *testing.T) {
	files := refusalFiles(t)
	if len(files) < 300 {
		t.Fatalf("the census read only %d files, so it covered almost nothing", len(files))
	}
	var findings []string
	for _, source := range files {
		ast.Inspect(source.file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !namesACursorField(lit, source.consts) {
				return true
			}
			if name := literalTypeName(lit); name != "MalformedCursorError" &&
				name != "storekit.MalformedCursorError" {
				findings = append(findings, source.path+": "+name)
			}
			return true
		})
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d refusal(s) name a cursor as the field they reject, but are not the "+
			"contract's refusal.\n\n"+
			"A caller cannot act on `required` for a token it just supplied, or on `invalid`, "+
			"which names no remedy — and neither code is reachable from the contract, so a client "+
			"written against it cannot recognise them. Return &storekit.MalformedCursorError{}; "+
			"httperr maps it to 422 malformed_cursor.\n\n\t%s",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// TestEveryCursorDecoderCanAnswerMalformed is the second arm: a function that
// READS a cursor must be able to give that answer, however it parses.
//
// Keyed on the decode rather than on the error's shape, because a refusal that
// names no field is invisible to the first arm — and that is exactly how the
// base64 decoder in the dedupe queue survived a sweep of the UUID-parsing ones.
func TestEveryCursorDecoderCanAnswerMalformed(t *testing.T) {
	// A ratification that stops matching is one for a function that has moved or
	// been folded in, and leaving it quietly re-exempts whatever takes its name.
	defer connectorCursors.AssertAllMatched(t)

	var findings []string
	decoders := 0
	for _, source := range refusalFiles(t) {
		for _, decl := range source.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !readsACursor(fn) {
				continue
			}
			decoders++
			if connectorCursors.Waived(t, source.path+":"+fn.Name.Name) {
				continue
			}
			if !mentionsMalformedCursor(fn.Body) {
				findings = append(findings, source.path+": "+fn.Name.Name)
			}
		}
	}
	// A census that recognises no decoder is judging nothing.
	if decoders < 10 {
		t.Fatalf("the census found only %d cursor decoders, so it is not reading the tree", decoders)
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d function(s) decide whether a cursor is well-formed without being able to "+
			"say so in the contract's words.\n\n"+
			"Answer &storekit.MalformedCursorError{} on the bad-token path, or delegate to "+
			"storekit.DecodeCursor.\n\n\t%s", len(findings), strings.Join(findings, "\n\t"))
	}
}

// readsACursor reports whether fn hands a value the code itself calls a cursor
// to a parsing primitive.
func readsACursor(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !callsADecoder(call) {
			return true
		}
		for _, arg := range call.Args {
			if namesACursor(arg) {
				found = true
			}
		}
		return true
	})
	return found
}

func callsADecoder(call *ast.CallExpr) bool {
	name := ""
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	}
	for _, decoder := range cursorDecoders {
		if name == decoder {
			return true
		}
	}
	return false
}

// namesACursor reads the expression's own vocabulary. A token the code calls a
// cursor is one, whether it arrives as `cursor`, `in.Cursor` or `*f.Cursor`.
func namesACursor(expr ast.Expr) bool {
	named := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok &&
			strings.Contains(strings.ToLower(ident.Name), "cursor") {
			named = true
		}
		return true
	})
	return named
}

func mentionsMalformedCursor(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			if n.Name == "MalformedCursorError" || n.Name == "DecodeCursor" {
				found = true
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "MalformedCursorError" || n.Sel.Name == "DecodeCursor" {
				found = true
			}
		}
		return true
	})
	return found
}
