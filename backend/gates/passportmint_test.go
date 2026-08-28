// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// A passport is minted for the session user, and for nobody else.
//
// This is what "no rep is ever acted for by a credential they did not mint"
// rests on, and it rests on a SHAPE rather than on a check: the one production
// INSERT binds on_behalf_of and granted_by to the same placeholder, filled from
// the authenticated session. There is no admin-mint path, so there is nothing
// to authorize wrongly.
//
// Nothing held that. A second INSERT taking a user id from anywhere else —
// a request body, a loop over colleagues, an "issue on behalf of" convenience —
// would compile, pass every existing test, and quietly turn standing authority
// into something one person can create for another. The whole overnight-agent
// feature is built on the property, so it is gated here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// passportOwner is the module that may write the passport table. The census in
// tableownership_test.go says the same thing about the table; this says it
// about the STATEMENT, which is the half that carries the invariant.
const passportOwner = "internal/modules/identity"

// TestEveryPassportMintTakesItsUserFromTheSession holds the other half of the
// invariant: WHERE the user id comes from.
//
// The statement check below proves on_behalf_of and granted_by are the same
// value. It cannot prove that value is the caller's own — Identity is a struct
// with exported fields, so a new admin endpoint could construct one naming
// anybody, hand it to IssuePassport, and mint a passport for a colleague
// through an INSERT this gate reads as correct.
//
// So every call site must pass an identity it READ from the request context,
// never one it built. identityFrom is the only way to obtain one honestly.
func TestEveryPassportMintTakesItsUserFromTheSession(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	calls := 0
	roots := []string{"internal", "cmd"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}
			parsed, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				return parseErr
			}
			ast.Inspect(parsed, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, identityArg, ok := mintCall(call)
				if !ok {
					return true
				}
				calls++
				// The identity argument must be a plain identifier — a variable
				// the function obtained. A composite literal is somebody
				// building an identity, which is exactly the shape that mints
				// for a person who did not ask.
				if _, plain := identityArg.(*ast.Ident); !plain {
					t.Errorf("%s calls %s with a constructed identity (%s) — "+
						"the user must come from the session, or the mint acts for "+
						"somebody who never asked",
						fset.Position(call.Pos()), name, exprText(fset, identityArg))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if calls == 0 {
		t.Fatal("no call to IssuePassport found under internal/ or cmd/ — this gate " +
			"read a smaller tree than it thinks, and would pass over a new mint caller")
	}
}

func TestOnlyIdentityMintsAPassportAndOnlyForTheSessionUser(t *testing.T) {
	t.Parallel()
	var inserts []string
	roots := []string{"internal", "cmd"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			// Test files and the integration harness are excluded on purpose:
			// a harness seeding a passport row is not a production mint path,
			// and holding it to this shape would only teach the next author to
			// move their new mint into a file the gate does not read.
			if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/integration/") {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			// Statements, not lines: a mint spelled across a multi-line raw
			// string is exactly the shape a line scan would miss.
			for _, stmt := range sqlStringsIn(t, path, body) {
				if !strings.Contains(strings.ToUpper(stmt), "INSERT INTO PASSPORT") {
					continue
				}
				inserts = append(inserts, path)
				if !strings.HasPrefix(filepath.ToSlash(path), passportOwner) {
					t.Errorf("%s mints a passport, but only %s may — a second mint "+
						"path is how a credential starts being created for somebody "+
						"other than the person it acts as", path, passportOwner)
					continue
				}
				if !mintsForOneUser(stmt) {
					t.Errorf("%s mints a passport whose on_behalf_of and granted_by "+
						"are not the same value: a rep must only ever be acted for "+
						"by a credential they minted themselves.\n\n%s", path, stmt)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// Under-recognition is the one way this gate must not break: a scan that
	// found no mint at all would report PASS having examined nothing.
	if len(inserts) == 0 {
		t.Fatal("no INSERT INTO passport found under internal/ or cmd/ — this gate " +
			"read a smaller tree than it thinks, and would pass over a new mint path")
	}
}

// mintsForOneUser reports whether the statement's on_behalf_of and granted_by
// columns are filled from the SAME placeholder.
//
// Two different placeholders is the defect, whatever fills them: it is the
// spelling that allows a caller to name somebody other than themselves.
func mintsForOneUser(stmt string) bool {
	columns, values, ok := insertColumnsAndValues(stmt)
	if !ok {
		return false
	}
	onBehalfOf, grantedBy := "", ""
	for i, col := range columns {
		if i >= len(values) {
			return false
		}
		switch col {
		case "on_behalf_of":
			onBehalfOf = values[i]
		case "granted_by":
			grantedBy = values[i]
		}
	}
	return onBehalfOf != "" && onBehalfOf == grantedBy
}

// insertColumnsAndValues splits the column list and the VALUES list of a single
// INSERT, in order.
//
// Both lists are read by MATCHING parentheses rather than by finding the next
// one. A value list carrying a call — `now() + $5::interval` is in the real
// statement — closes a paren that is not the list's, and a scan taking the
// first `)` truncates the list and reports the correct mint as malformed.
func insertColumnsAndValues(stmt string) (columns, values []string, ok bool) {
	cols, after, ok := parenGroup(stmt)
	if !ok {
		return nil, nil, false
	}
	valuesAt := strings.Index(strings.ToUpper(after), "VALUES")
	if valuesAt < 0 {
		return nil, nil, false
	}
	vals, _, ok := parenGroup(after[valuesAt+len("VALUES"):])
	if !ok {
		return nil, nil, false
	}
	return splitTopLevel(cols), splitTopLevel(vals), true
}

// parenGroup returns the contents of the first balanced parenthesised group and
// whatever follows it.
func parenGroup(s string) (inside, rest string, ok bool) {
	open := strings.Index(s, "(")
	if open < 0 {
		return "", "", false
	}
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], s[i+1:], true
			}
		}
	}
	return "", "", false
}

// splitTopLevel splits on commas that are not inside a nested call.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// sqlStringsIn returns every string literal in the file, so the scan reads
// whole statements rather than lines.
func sqlStringsIn(t *testing.T, path string, body []byte) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, body, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var out []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			out = append(out, gatekit.TextOf(lit))
		}
		return true
	})
	return out
}

// mintNames is every function that mints a passport, by name, with the
// position of its Identity argument.
//
// It is a LIST because the mint is reachable by more than one spelling: the
// session path calls IssuePassport, and a caller with its own half of the same
// fact to commit calls IssuePassportTx so the two land in one transaction. A
// gate matching one name proves nothing about the other, and the one it misses
// is the one a future caller reaches for — which is how a mint for a person who
// never asked would get past a green gate.
//
// Held by: TestOnlyIdentityMintsAPassportAndOnlyForTheSessionUser (backend/gates/passportmint_test.go)
var mintNames = map[string]int{
	// IssuePassport(ctx, id, in)
	"IssuePassport": 1,
	// IssuePassportTx(ctx, tx, id, in) — the transaction displaces the identity.
	"IssuePassportTx": 2,
}

// mintCall reports whether a call is a passport mint, and which argument
// carries the identity.
//
// It matches a bare identifier as well as a selector: IssuePassportTx is a
// package-level function called unqualified from inside identity, so a matcher
// that only understood `x.IssuePassport(...)` would not see the call that
// actually mints there.
func mintCall(call *ast.CallExpr) (name string, identityArg ast.Expr, ok bool) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	case *ast.Ident:
		name = fun.Name
	default:
		return "", nil, false
	}
	at, known := mintNames[name]
	if !known || len(call.Args) <= at {
		return "", nil, false
	}
	return name, call.Args[at], true
}
