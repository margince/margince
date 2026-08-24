// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A page token a caller hands back is either one this server minted or it is
// not, and that is ONE question with one answer on the wire: the contract's
// `422 code: malformed_cursor`, which tells the caller to re-issue the request
// without the token.
//
// A module that invents its own refusal answers a different question. `required`
// tells a caller to supply a field it just supplied, so acting on it means
// sending the same token again; `invalid` and `invalid_query` name no remedy.
// None of them is reachable from the contract, so a client written against the
// contract cannot recognise any of them.
//
// THE RULE: a refusal about a cursor is `storekit.MalformedCursorError`.
// `platform/httperr` maps that one type to the contract's code, and mapping is
// the only place the wire code is decided.
//
// TWO ARMS, because a refusal and a decode are visible to different readers. The
// first reads the refusal's shape and sees nothing when the error names no
// field. The second reads the decode and sees nothing when the refusal is built
// somewhere the decode is not. A cursor read by base64 and refused without
// naming a field is invisible to either arm alone.

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
	"internal/modules/capture/gmail/gmail.go:parseCursor":          "reads the Gmail history id this connector stored on its own last sync. The provider mints it, the server persists and hands it back; a caller never sends one.",
	"internal/modules/capture/gcal/gcal.go:parseCursor":            "reads the Google Calendar sync token this connector stored on its own last sync — the provider's resume state, never request input.",
	"internal/modules/capture/graph/graph.go:parseCursor":          "reads the Microsoft Graph delta link this connector stored on its own last sync — the provider's resume state, never request input.",
	"internal/modules/capture/imap/standing.go:parseIMAPCursor":    "reads the IMAP UID watermark this connector stored on its own last sync — server-minted resume state, never request input.",
	"internal/modules/overlay/fake/adapter.go:parseCursor":         "the overlay fake incumbent's own paging offset, minted and read by the fake. It stands in for a third-party CRM's cursor, not for one of ours.",
	"internal/modules/capture/backfillpager.go:backfillPageCursor": "reads the provider page token stored on a backfill run, handed back to the provider's own API. A caller never sends it, and an unreadable one is a server-side fault the run reports, not a 422.",
})

// refusalSurfaceRoots are the trees where a cursor may be refused.
var refusalSurfaceRoots = []string{"internal/modules", "internal/compose", "../extensions"}

// cursorDecoders are the primitives a page token is read WITH. A function
// calling one of these on a token is deciding whether it is well-formed, which
// is the decision this file governs.
//
// Wider than the shapes this tree happens to use today, because a census that
// recognises only those reports a clean tree over the next one.
var cursorDecoders = []string{
	"Parse", "ParseUUID", "DecodeString", "Unmarshal", "Sscanf", "Cut", "Atoi", "DecodeCursor",
}

// namesAToken reports whether an identifier is what this tree calls a page
// token. `cur` is here because the connector decoders spell it that way; a
// census keyed only on whichever spelling its author was looking at leaves the
// rest invisible.
//
// Bare `token` is NOT: it is the ordinary word for a lexeme, so it reaches
// parsers with no page token in them. A census that cries wolf teaches its
// readers to skip the output.
func namesAToken(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "cursor") || strings.Contains(lower, "pagetoken") || lower == "cur"
}

// refusalFile is one parsed product file with its package's string constants,
// so a field named through a constant (`Field: fieldCursor`) reads the same as
// one named literally.
type refusalFile struct {
	path       string
	file       *ast.File
	consts     map[string]string
	constNames map[string]bool
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
		names := declaredConstNames(files)
		for _, f := range files {
			out = append(out, refusalFile{path: paths[f], file: f, consts: consts, constNames: names})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// fieldVerdict is what a composite literal's `Field:` says about the argument it
// refuses.
type fieldVerdict int

const (
	fieldIsSomethingElse fieldVerdict = iota
	fieldIsACursor
	fieldIsUnreadable
)

// cursorFieldVerdict reads a composite literal's `Field:` — the shape an error
// uses to say WHICH argument it is refusing.
//
// A value that will not resolve is reported rather than passed over. The field
// is named through a constant in this tree, and a constant the resolver cannot
// fold (bound twice in one package, or built from something it does not follow)
// would silently read as "not a cursor" — a false pass in the one place this
// census is trusted to look.
func cursorFieldVerdict(lit *ast.CompositeLit, consts map[string]string, constNames map[string]bool) fieldVerdict {
	for _, element := range lit.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || !strings.EqualFold(key.Name, "field") {
			continue
		}
		text, resolved := stringValue(pair.Value, consts)
		switch {
		case resolved && text == "cursor":
			return fieldIsACursor
		case resolved:
			return fieldIsSomethingElse
		case namesADeclaredConst(pair.Value, constNames):
			return fieldIsUnreadable
		}
	}
	return fieldIsSomethingElse
}

// namesADeclaredConst reports whether a Field: value is an identifier the
// package declares as a constant — one the resolver was meant to fold.
//
// A parameter or local carrying a field name at runtime (`Field: field`, in the
// search refusals) is not something any census could read, and reporting it
// would ask an author to change correct code.
func namesADeclaredConst(value ast.Expr, constNames map[string]bool) bool {
	ident, named := value.(*ast.Ident)
	return named && constNames[ident.Name]
}

// declaredConstNames are the names a package binds with `const`, whether or not
// the resolver could fold their values.
func declaredConstNames(files []*ast.File) map[string]bool {
	names := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range value.Names {
						names[name.Name] = true
					}
				}
			}
		}
	}
	return names
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
	var findings, unreadable []string
	for _, source := range files {
		ast.Inspect(source.file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch cursorFieldVerdict(lit, source.consts, source.constNames) {
			case fieldIsUnreadable:
				unreadable = append(unreadable, source.path+": "+literalTypeName(lit))
			case fieldIsACursor:
				if name := literalTypeName(lit); name != "MalformedCursorError" &&
					name != "storekit.MalformedCursorError" {
					findings = append(findings, source.path+": "+name)
				}
			case fieldIsSomethingElse:
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
	if len(unreadable) > 0 {
		sort.Strings(unreadable)
		t.Errorf("%d refusal(s) name their field through a constant this census cannot resolve, "+
			"so it cannot tell whether they are about a cursor.\n\n"+
			"An unreadable field reads as \"not a cursor\" and passes for the wrong reason. Bind the "+
			"constant once per package, or name the field literally.\n\n\t%s",
			len(unreadable), strings.Join(unreadable, "\n\t"))
	}
}

// TestEveryCursorDecoderCanAnswerMalformed is the second arm: a function that
// READS a cursor must be able to give that answer, however it parses.
//
// Keyed on the decode rather than on the error's shape, because a refusal that
// names no field is invisible to the first arm.
func TestEveryCursorDecoderCanAnswerMalformed(t *testing.T) {
	// A ratification that stops matching is one for a function that has moved or
	// been folded in, and leaving it quietly re-exempts whatever takes its name.
	defer connectorCursors.AssertAllMatched(t)

	var findings []string
	branches := 0
	for _, source := range refusalFiles(t) {
		for _, decl := range source.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			refusals := cursorRefusals(fn)
			if len(refusals) == 0 {
				continue
			}
			if connectorCursors.Waived(t, source.path+":"+fn.Name.Name) {
				continue
			}
			for _, refusal := range refusals {
				branches++
				if !refusal.answersTheContract() {
					findings = append(findings, source.path+": "+fn.Name.Name)
				}
			}
		}
	}
	// A census that recognises no refusal branch is judging nothing.
	if branches < 15 {
		t.Fatalf("the census found only %d cursor refusal branches, so it is not reading the tree", branches)
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d failure branch(es) refuse a page token without saying so in the contract's "+
			"words.\n\n"+
			"Return &storekit.MalformedCursorError{} from the branch itself, or delegate the decode "+
			"to storekit.DecodeCursor and hand its error on.\n\n\t%s",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// cursorRefusals returns, for each place fn decodes a page token, the error
// expressions its IMMEDIATE failure branch returns — and whether that decode
// delegated to storekit.
//
// The branch, not the function. A compound decoder that answers the contract on
// one failure and a module error on another passes any check asking only whether
// the symbol appears somewhere in the body, while the second failure still
// reaches a client as the wrong code. What a caller receives is decided by the
// branch it lands in.
//
// A decode with no failure branch refuses nothing, so there is nothing here to
// misclassify and it yields no entry.
func cursorRefusals(fn *ast.FuncDecl) []cursorRefusal {
	carries := tokenCarriers(fn)
	var found []cursorRefusal
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range block.List {
			// `x, err := decode(cursor)` followed by `if err != nil { ... }`.
			assign, isAssign := stmt.(*ast.AssignStmt)
			if !isAssign || i+1 >= len(block.List) {
				continue
			}
			delegated, decodes := decodesAToken(assign.Rhs, carries)
			if !decodes {
				continue
			}
			guard, isGuard := block.List[i+1].(*ast.IfStmt)
			if !isGuard || !guardsAnError(guard.Cond) {
				continue
			}
			if returns := returnedErrors(guard.Body); refuses(returns) {
				found = append(found, cursorRefusal{returns: returns, delegated: delegated})
			}
		}
		return true
	})
	// `if x, err := decode(cursor); err != nil { ... }` — the same decision with
	// the assignment inside the guard.
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		guard, ok := node.(*ast.IfStmt)
		if !ok || guard.Init == nil || !guardsAnError(guard.Cond) {
			return true
		}
		assign, isAssign := guard.Init.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		if delegated, decodes := decodesAToken(assign.Rhs, carries); decodes {
			if returns := returnedErrors(guard.Body); refuses(returns) {
				found = append(found, cursorRefusal{returns: returns, delegated: delegated})
			}
		}
		return true
	})
	return found
}

// cursorRefusal is one failure branch: what it returns, and whether the decode
// it guards came from storekit.
type cursorRefusal struct {
	returns   []ast.Expr
	delegated bool
}

// answersTheContract reports whether this branch gives the caller the contract's
// refusal.
//
// A branch guarding storekit.DecodeCursor may simply hand the error on: that
// error already IS the contract's, and re-wrapping it would be the second
// spelling this file exists to prevent. Every other branch has to say so itself.
func (r cursorRefusal) answersTheContract() bool {
	for _, returned := range r.returns {
		if r.delegated && mentionsAnError(returned) {
			return true
		}
		if buildsMalformedCursor(returned) {
			return true
		}
	}
	return false
}

// decodesAToken reports whether an assignment's right-hand side reads a page
// token, and whether it did so through storekit.
//
// A call named DecodeCursor is a cursor decode whatever its argument is called —
// the function name is the evidence, and requiring the argument to be named too
// would drop the sites that spell it `token`. A general-purpose primitive
// (`Atoi`, `Cut`, `Parse`) needs the argument to say what it is reading.
func decodesAToken(rhs []ast.Expr, carries map[string]bool) (delegated, decodes bool) {
	for _, expr := range rhs {
		call, ok := expr.(*ast.CallExpr)
		if !ok || !callsADecoder(call) {
			continue
		}
		if decoderName(call) == "DecodeCursor" {
			return true, true
		}
		for _, arg := range call.Args {
			if namesACursor(arg) || carriesAToken(arg, carries) {
				decodes = true
			}
		}
	}
	return delegated, decodes
}

// tokenCarriers are the locals a function has copied a page token into, to a
// fixed point.
//
// Without this, one assignment launders the token out of the census: `t :=
// in.Cursor`, and every later mention is of `t`, which no vocabulary of names
// recognises. A rule a rename defeats is not a rule.
func tokenCarriers(fn *ast.FuncDecl) map[string]bool {
	carries := map[string]bool{}
	for {
		learned := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, target := range assign.Lhs {
				name, isIdent := target.(*ast.Ident)
				if !isIdent || carries[name.Name] || i >= len(assign.Rhs) {
					continue
				}
				if namesACursor(assign.Rhs[i]) || carriesAToken(assign.Rhs[i], carries) {
					carries[name.Name] = true
					learned = true
				}
			}
			return true
		})
		if !learned {
			return carries
		}
	}
}

func carriesAToken(expr ast.Expr, carries map[string]bool) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && carries[ident.Name] {
			found = true
		}
		return true
	})
	return found
}

func decoderName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func guardsAnError(cond ast.Expr) bool {
	binary, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	return mentionsAnError(binary.X) || mentionsAnError(binary.Y)
}

func mentionsAnError(expr ast.Expr) bool {
	named := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && strings.Contains(strings.ToLower(ident.Name), "err") {
			named = true
		}
		return true
	})
	return named
}

// refuses reports whether a branch hands an ERROR back. A branch that recovers
// instead — returning a zero value, or the token unchanged — declines nothing,
// so there is no code for it to get wrong. `splitCappedCursor` reads that way on
// purpose: a cursor without the count prefix is the start of paging.
func refuses(returns []ast.Expr) bool {
	for _, returned := range returns {
		if looksLikeAnError(returned) {
			return true
		}
	}
	return false
}

func looksLikeAnError(expr ast.Expr) bool {
	switch node := expr.(type) {
	case *ast.Ident:
		return strings.Contains(strings.ToLower(node.Name), "err")
	case *ast.UnaryExpr:
		return looksLikeAnError(node.X)
	case *ast.CompositeLit:
		return strings.HasSuffix(literalTypeName(node), "Error")
	case *ast.CallExpr:
		name := decoderName(node)
		return name == "New" || name == "Errorf" || strings.Contains(strings.ToLower(name), "err")
	case *ast.SelectorExpr:
		return strings.Contains(strings.ToLower(node.Sel.Name), "err")
	}
	return false
}

func returnedErrors(body *ast.BlockStmt) []ast.Expr {
	var out []ast.Expr
	ast.Inspect(body, func(node ast.Node) bool {
		if ret, ok := node.(*ast.ReturnStmt); ok {
			out = append(out, ret.Results...)
		}
		return true
	})
	return out
}

func buildsMalformedCursor(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			if n.Name == "MalformedCursorError" {
				found = true
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "MalformedCursorError" {
				found = true
			}
		}
		return true
	})
	return found
}

func callsADecoder(call *ast.CallExpr) bool {
	name := decoderName(call)
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
		if ident, ok := node.(*ast.Ident); ok && namesAToken(ident.Name) {
			named = true
		}
		return true
	})
	return named
}
