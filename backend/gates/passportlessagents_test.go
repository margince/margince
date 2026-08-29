// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// An agent principal carrying no passport is a principal nobody can revoke.
//
// platform/auth's Admit re-asks a passport's liveness at every tool call, which
// is what makes revocation bind mid-run: a run authenticates once at start and
// then executes for its whole wall clock, so without the second asking a killed
// credential keeps acting until the run ends on its own.
//
// That check has exactly one way to be skipped, and it is not a bug: a
// principal that names NO passport holds no credential to re-ask about. Two
// production paths mint one — the product acting under a policy, derived from a
// live human at construction rather than from a long-lived token. Both are
// right, and both are also the shape somebody could copy into a third place
// where it is not, arriving as a passport-free agent that the kill switch does
// not reach.
//
// So the sites are a REGISTER. A new one fails here until whoever adds it says
// why the principal it mints answers to nobody's revocation.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// passportlessAgentPrincipals are the ratified sites that build an agent
// principal with no PassportID, keyed by "path:func".
//
// modules/identity is not on it and cannot be: AgentIdentity.Principal is the
// authentication path, and it always sets the passport it just resolved. A
// principal minted there without one would be a defect rather than an
// exception, so the census reads that package too.
var passportlessAgentPrincipals = gatekit.Waive(map[string]string{
	"internal/compose/extjobsrun.go:deriveAuthority": "an extension job TICK, not a credential: the authority is the job owner's, read live from identity as the tick starts, plus the single scope the extension's manifest declared. Nothing issues a token for it and nothing can hold one after the tick ends, so there is no revocation for the gate to re-ask about — killing it is killing the job or the owner's own authority, both of which the seat and RBAC re-read already bind",
	"internal/compose/autoapply.go:asOwnersAgent":    "the auto-apply actor, minted per decision from the record owner's live authority (EffectiveAuthority) and carrying write and nothing else. It exists for the length of one staged action; the owner losing their seat or their grants stops it at the next admission, which is the whole of what revoking it would mean",
})

// TestEveryPassportlessAgentPrincipalIsRatified is the census.
func TestEveryPassportlessAgentPrincipalIsRatified(t *testing.T) {
	t.Parallel()
	defer passportlessAgentPrincipals.AssertAllMatched(t)
	found := 0
	// internal/shared is walked too: the principal TYPE lives there, and a
	// mint in the package that defines it would be the one place nobody
	// thought to look.
	for _, root := range []string{"internal/compose", "internal/modules", "internal/platform", "internal/shared", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			// The QUALIFIER this file actually spells, not the package's own
			// name. A file that aliases the import — or dot-imports it — writes
			// the same literal under a different word, and a reader looking for
			// `principal.` alone would answer "no agent principal here" for the
			// one file where somebody had a reason to spell it differently.
			qualifier, dotImported := gatekit.ImportedAs(file, principalImportPath)
			if qualifier == "" && !dotImported {
				return nil
			}
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || fn.Body == nil {
					continue
				}
				for _, mint := range agentPrincipalMints(fn, qualifier, dotImported) {
					found++
					if mint {
						continue
					}
					if passportlessAgentPrincipals.Waived(t, path+":"+fn.Name.Name) {
						continue
					}
					t.Errorf("%s: %s builds an agent principal naming no passport — a zero value counts as none, and a computed Type counts as unreadable. Admit cannot re-ask a credential nobody named, so revoking whatever authorized this run stops nothing until the run ends on its own. Carry the passport, or ratify it in passportlessAgentPrincipals with what killing this principal would even mean",
						path, fn.Name.Name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A census that judged nothing certifies nothing, and this one goes quiet
	// the moment the literal stops being written as a literal.
	if found < 3 {
		t.Fatalf("this census found %d agent-principal construction(s) and expects at least 3 — the reader has stopped seeing them rather than the tree having lost them", found)
	}
}

// agentPrincipalMints reports, for each principal.Principal literal in fn that
// sets Type to an agent, whether it also names a PassportID.
//
// The TYPE is what makes a literal in scope: a principal built for a human, a
// connector or the system holds no passport by construction and is governed by
// something else entirely. A literal that computes its Type rather than naming
// it is not read at all — that shape does not exist in this tree, and reading
// it would mean evaluating the expression rather than reading it.
// principalImportPath is the package the type and the kind both come from.
const principalImportPath = "github.com/margince/margince/backend/internal/shared/kernel/principal"

func agentPrincipalMints(fn *ast.FuncDecl, qualifier string, dotImported bool) []bool {
	var out []bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !isPrincipalType(lit.Type, qualifier, dotImported) {
			return true
		}
		agent, passport, unreadable := false, false, false
		for _, element := range lit.Elts {
			pair, isPair := element.(*ast.KeyValueExpr)
			if !isPair {
				continue
			}
			key, isIdent := pair.Key.(*ast.Ident)
			if !isIdent {
				continue
			}
			switch key.Name {
			case "Type":
				agent = agent || namesAgentPrincipal(pair.Value, qualifier, dotImported)
				// A Type this reader cannot judge is not a pass. Computed, it
				// could be the agent kind, and skipping the literal would wave
				// through the one shape this census exists for.
				unreadable = unreadable || !isPlainSelector(pair.Value, dotImported)
			case "PassportID":
				// The VALUE, not the key. `PassportID: ids.UUID{}` names the
				// field and no credential — which is exactly the principal
				// nobody can revoke, wearing the shape of one that can — and a
				// value this reader cannot judge is not a pass either, for the
				// same reason a computed Type is not.
				named, readable := namesSomePassport(pair.Value)
				passport = passport || named
				unreadable = unreadable || !readable
			}
		}
		if agent || unreadable {
			out = append(out, passport && !unreadable)
		}
		return true
	})
	return out
}

func isPrincipalType(expr ast.Expr, qualifier string, dotImported bool) bool {
	if dotImported {
		ident, isIdent := expr.(*ast.Ident)
		return isIdent && ident.Name == "Principal"
	}
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "Principal" {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == qualifier
}

func namesAgentPrincipal(expr ast.Expr, qualifier string, dotImported bool) bool {
	if dotImported {
		ident, isIdent := expr.(*ast.Ident)
		return isIdent && ident.Name == "PrincipalAgent"
	}
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "PrincipalAgent" {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == qualifier
}

// isPlainSelector reports whether an expression NAMES the kind rather than
// standing in for it.
//
// A selector qualified by the principal import — `principal.PrincipalAgent`,
// under whatever name this file gave it — is the readable form, and a dot
// import makes a bare identifier readable too. Anything else is not: a bare
// identifier under an ordinary import can be a package-level var holding the
// agent kind, which reads as "not an agent" to a matcher looking for the
// constant, and deciding a site is not an agent is exactly the pass that hides
// it.
func isPlainSelector(expr ast.Expr, dotImported bool) bool {
	if dotImported {
		// Under a dot import the readable form is the CONSTANT'S OWN NAME and
		// nothing else. Accepting any bare identifier would make a computed
		// `Type: agentKind` readable here while the qualified path refuses
		// exactly that shape — the two sides disagreeing about the same
		// question, with the dot-import side the permissive one.
		ident, isIdent := expr.(*ast.Ident)
		return isIdent && ident.Name == "PrincipalAgent"
	}
	_, isSelector := expr.(*ast.SelectorExpr)
	return isSelector
}

// namesSomePassport answers two things about a PassportID value: whether it
// names a credential, and whether this reader could tell.
//
// `ids.UUID{}` is the ZERO value and names none — the field is set, the
// census's question is unanswered, and the site is the very one it exists to
// find. A FIELD READ (`a.PassportID.UUID`) is taken as naming one, because that
// is how an authenticated identity hands its own credential on.
//
// Everything else is UNREADABLE rather than assumed: a bare identifier or a
// call can hold a zero id just as easily as a real one, which is the same
// indirection the Type side already refuses to guess at. Assuming a passport
// there is the under-recognition this census may not have — the site passes and
// nothing says it was never judged.
func namesSomePassport(expr ast.Expr) (names, readable bool) {
	switch value := expr.(type) {
	case *ast.CompositeLit:
		return len(value.Elts) > 0, true
	case *ast.SelectorExpr:
		return true, true
	case *ast.UnaryExpr:
		return namesSomePassport(value.X)
	}
	return false, false
}
