// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestBothAgentAuthenticationPathsExecuteTheOneLivenessQuery is a fitness
// function over this package's own source, because the property is structural:
// an agent authenticates either by bearer token or by passport id, and a
// liveness rule that binds on one and not the other is a live credential on
// whichever path was missed. Asserting that the query BUILDER contains the
// constants it concatenates would prove only that concatenation works — it
// would stay green while an entry point quietly grew a SELECT of its own,
// which is the one drift this guard exists to catch. So what is asserted is
// what the two exported entry points reach for, and that the package has
// exactly one statement selecting from passport for authentication.
func TestBothAgentAuthenticationPathsExecuteTheOneLivenessQuery(t *testing.T) {
	const source = "passport.go"
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}

	// Two claims, because they fail differently and one check cannot carry
	// both.
	//
	// A HAND-ROLLED rule: a statement that selects from the passport relation
	// and spells a liveness condition itself. That is what a new authenticator
	// written without being told about agentLivenessWhere looks like, and a
	// passport killed by one of two rules is admitted by the other.
	//
	// And an asking GOING MISSING: the rule is built on in two places, because
	// it is asked at two moments — when a call arrives with a token, and at
	// every tool admission inside a run that authenticated once and then
	// executes for its whole wall clock.
	//
	// This used to be one count of `FROM passport p` in one file, which was the
	// same claim while there was one moment to ask in and one file to ask from.
	for where, declaration := range handRolledLivenessRules(t) {
		t.Errorf("%s selects from the passport relation and spells a liveness condition of its own instead of building on agentLivenessWhere — a passport killed by one of two rules is admitted by the other:\n%s", where, declaration)
	}
	if askings := declarationsBuildingOnTheRule(t); len(askings) < 2 {
		t.Errorf("agentLivenessWhere is built on by %v — it is asked when a call arrives AND at every admission, so fewer than two means one of those askings is gone", askings)
	}
	// And the admission path REACHES the second one. The count above is over
	// declarations, which stay whether or not anybody executes them; this is
	// what fails if AdmittedAuthority stops asking. The runtime half is
	// TestRevocationReachesARunAlreadyInFlight, which revokes and drives it.
	if body := funcBodyIn(t, "authority.go", "AdmittedAuthority"); body == nil {
		t.Error("identity has no AdmittedAuthority — the admission read this rule is asked at is gone or renamed")
	} else if !mentionsIdent(body, "passportStillLiveQuery") {
		t.Error("AdmittedAuthority no longer reaches passportStillLiveQuery — the rule is still written down and nothing asks it, so a revoked passport runs to the end of its run again")
	}

	for _, entryPoint := range []string{"AuthenticateAgent", "AuthenticateAgentByID"} {
		body := funcBody(t, file, entryPoint)
		if body == nil {
			t.Fatalf("%s has no %s method — the entry point this rule guards is gone or renamed", source, entryPoint)
		}
		if !callsFunc(body, "authenticateAgentWhere") {
			t.Errorf("%s does not go through authenticateAgentWhere, so it does not carry the liveness rule the other path does", entryPoint)
		}
	}
}

// handRolledLivenessRules returns the declarations in the package that decide a
// passport's liveness with a condition of their own, keyed by where.
//
// The two filters are applied to ONE STRING LITERAL, which is what makes them
// mean anything together. RevokePassport reads `SELECT … FROM passport WHERE id
// = $1` and writes `UPDATE passport SET revoked_at = now() WHERE … AND
// revoked_at IS NULL` — one declaration carrying both halves and authenticating
// nothing, so a declaration-wide match reports the writer that performs
// revocation as a second rule for deciding it.
//
// The correct statements are not caught by this and must not be: they carry no
// condition of their own, only the concatenated agentLivenessWhere. What holds
// THEM is the count below.
func handRolledLivenessRules(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range packageSources(t) {
		for _, decl := range declarationsIn(t, name) {
			for _, statement := range gatekit.SQLStatementsOf(decl.node) {
				if !strings.Contains(statement, "FROM passport") || !spellsALivenessCondition(statement) {
					continue
				}
				if strings.Contains(decl.source, "agentLivenessWhere") {
					continue
				}
				out[name+":"+decl.name] = decl.source
			}
		}
	}
	return out
}

// declarationsBuildingOnTheRule names the declarations that concatenate the
// shared rule — the two askings, and nothing else.
func declarationsBuildingOnTheRule(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, name := range packageSources(t) {
		for _, decl := range declarationsIn(t, name) {
			if decl.name == "agentLivenessWhere" || !strings.Contains(decl.source, "agentLivenessWhere") {
				continue
			}
			out = append(out, name+":"+decl.name)
		}
	}
	sort.Strings(out)
	return out
}

// spellsALivenessCondition reports whether a statement decides for itself
// whether a passport is still a credential.
//
// PREDICATE shapes, not column names. The list surface selects revoked_at and
// expires_at as columns and shows revoked rows on purpose, so naming a column
// was never the discriminator — and a reformat of that query would otherwise
// turn it into a spurious failure here.
func spellsALivenessCondition(statement string) bool {
	for _, tell := range livenessTells {
		if strings.Contains(statement, tell) {
			return true
		}
	}
	return false
}

// livenessTells are the ways a statement says it is deciding whether a passport
// is still a credential.
//
// EVERY condition the shared rule carries, and the expiry BOUND SPELLED BOTH
// WAYS. A hand-rolled rule keeping only the still-live half — `expires_at <
// now()` inverted, or the rotation and grant halves folded into the shared
// string while the expiry is written out — would otherwise select from passport
// and evade the check, which is the whole of what "a passport killed by one of
// two rules is admitted by the other" means.
//
// PREDICATE shapes, not column names: the list surface selects revoked_at and
// expires_at as columns and shows revoked rows on purpose, so naming a column
// was never the discriminator — and a reformat of that query would otherwise
// become a spurious failure here.
var livenessTells = []string{
	"revoked_at IS NULL",
	"revoked_at IS NOT NULL",
	"< p.expires_at",
	"> p.expires_at",
	"p.expires_at >",
	"p.expires_at <",
	"expires_at > now()",
	"expires_at < now()",
	"now() < p.expires_at",
	"now() > p.expires_at",
	"must_change_password",
	"client_id IS NOT NULL",
}

// declaration is one top-level declaration: its name, its source text, and the
// node itself for the readers that want the syntax rather than the words.
type declaration struct {
	name   string
	source string
	node   ast.Decl
}

// declarationsIn returns the top-level declarations of one file, bounded by the
// PARSER's idea of where each ends rather than by a blank line — the rule is
// concatenated after a closing backtick, and SQL formatted with a blank line
// between clauses would cut a declaration before the part saying which rule it
// builds on.
func declarationsIn(t *testing.T, name string) []declaration {
	t.Helper()
	source := readSource(t, name)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	var out []declaration
	for _, decl := range file.Decls {
		start, end := fset.Position(decl.Pos()).Offset, fset.Position(decl.End()).Offset
		if start < 0 || end > len(source) || start >= end {
			continue
		}
		out = append(out, declaration{name: declarationLabel(decl), source: source[start:end], node: decl})
	}
	return out
}

// packageSources names this package's own non-test files. The whole package,
// because the claim is about the package: reading one file would leave a third
// free to grow a rule nobody checks, which is how the liveness rule moving to
// its own file for the size cap would have widened the hole.
func packageSources(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			out = append(out, name)
		}
	}
	return out
}

// funcBodyIn finds one function's body in a named file of this package.
func funcBodyIn(t *testing.T, file, name string) *ast.BlockStmt {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	return funcBody(t, parsed, name)
}

// mentionsIdent reports whether a body names an identifier anywhere.
func mentionsIdent(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// Every condition the liveness rule EXISTS for is in it. The shared string is
// what makes both askings agree; this is what stops them agreeing on less.
func TestTheLivenessRuleStillAsksEverythingItIsFor(t *testing.T) {
	t.Parallel()
	for _, condition := range []string{
		"p.revoked_at IS NULL",         // the operator's kill switch
		"now() < p.expires_at",         // the credential's own lifetime
		"must_change_password = false", // a human who owes a rotation grants nothing
		"g.revoked_at IS NULL",         // the OAuth grant behind it
		"c.client_id IS NOT NULL",      // and the client that grant names
	} {
		if !strings.Contains(agentLivenessWhere, condition) {
			t.Errorf("the liveness rule no longer asks %q — a passport this condition exists to stop is now admitted, on both paths at once", condition)
		}
	}
	// The re-check is about ONE passport and the human it still answers to.
	// Without the second half, a passport re-granted to somebody else keeps
	// admitting calls bounded by the authority of the human it left.
	if !strings.Contains(passportStillLiveQuery, "p.on_behalf_of = $2") {
		t.Error("the admission re-check does not compare the granting human — a re-granted passport still acts on the old human's authority")
	}
}

// The two predicates differ in NOTHING but the column that names the passport:
// any other difference is a second place the rules can rot apart.
func TestTheTwoAgentPredicatesDifferOnlyInWhichColumnNamesThePassport(t *testing.T) {
	byToken := strings.Replace(agentAuthQuery(agentByHashPredicate), agentByHashPredicate, "<predicate>", 1)
	byID := strings.Replace(agentAuthQuery(agentByIDPredicate), agentByIDPredicate, "<predicate>", 1)
	if byToken != byID {
		t.Errorf("the two authentication paths differ beyond their predicate:\n%s\n---\n%s", byToken, byID)
	}
}

// A locally minted passport answers to no OAuth grant, so the liveness rule
// must be a condition on the joined rows and never a requirement that they
// exist — an inner join, or dropping the IS NULL arm, would take the whole A1
// surface down with it.
func TestTheLivenessRuleExemptsLocallyMintedPassports(t *testing.T) {
	query := agentAuthQuery(agentByHashPredicate)
	if strings.Count(query, "LEFT JOIN") != 2 {
		t.Errorf("the connection joins are not both LEFT JOINs, so a passport with no grant cannot match:\n%s", query)
	}
	if !strings.Contains(query, "p.oauth_grant_id IS NULL") {
		t.Errorf("the liveness predicate has no exemption for a passport that answers to no grant:\n%s", query)
	}
}

// readSource reads one file of this package. The test runs in the package
// directory, so the plain name resolves.
func readSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(raw)
}

// funcBody finds a method or function by name, whatever it is declared on.
func funcBody(t *testing.T, file *ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn.Body
		}
	}
	return nil
}

// callsFunc reports whether body calls name anywhere inside it, including as a
// method on a receiver.
func callsFunc(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			found = found || fn.Name == name
		case *ast.SelectorExpr:
			found = found || fn.Sel.Name == name
		}
		return !found
	})
	return found
}

// declarationLabel names a declaration for a finding.
func declarationLabel(decl ast.Decl) string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		return typed.Name.Name
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok && len(value.Names) > 0 {
				return value.Names[0].Name
			}
		}
	}
	return "an unnamed declaration"
}
