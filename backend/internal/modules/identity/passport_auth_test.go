// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
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

	// Every statement in the package that FILTERS FOR LIVENESS builds on the
	// one rule.
	//
	// This used to count: exactly one `FROM passport p` in passport.go, on the
	// argument that a second occurrence is a second place to remember the rule.
	// That was the same claim while there was one moment to ask in. There are
	// two now — the call arriving with a token, and every tool ADMISSION inside
	// a run that authenticated once and then executes for its whole wall clock
	// — and a count cannot tell a second correct asking from a second rule.
	//
	// What separates them is INTENT, not the alias: the list surface selects
	// `FROM passport p` too and deliberately shows revoked rows, so aliasing
	// never meant "authenticating". A statement that asks whether a passport is
	// revoked or expired is one that decides liveness, and that is the one that
	// has to reach for agentLivenessWhere by name — which still catches what
	// the count was for, a statement quietly grown conditions of its own.
	deciders := livenessDecidingStatements(t)
	if len(deciders) < 2 {
		t.Errorf("this package holds %d statement(s) deciding a passport's liveness — the rule is asked when a call arrives AND at every admission, so fewer than two means one of those askings is gone", len(deciders))
	}
	for where, statement := range deciders {
		if !strings.Contains(statement, "agentLivenessWhere") {
			t.Errorf("%s decides a passport's liveness without building on agentLivenessWhere — a passport killed by one of two rules is admitted by the other:\n%s", where, statement)
		}
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

// livenessDecidingStatements returns the source text of every declaration in
// the PACKAGE that selects from the passport relation AND decides whether the
// row is still a credential, keyed by where it was found.
//
// The whole package rather than one file, and that is not tidiness. It used to
// read passport.go alone, which was the same claim while the rule and its two
// askings lived there; splitting the liveness rule into its own file for the
// size cap would have left a third file free to grow a statement nobody checks.
// The claim is about the package, so the reader is.
//
// Read as TEXT rather than through the parser, because what is being judged is
// what the author wrote: a declaration that concatenates agentLivenessWhere
// says so in the source, and one that spells the conditions out again says that
// instead. The parsed value would be identical either way, which is exactly the
// drift this is for.
func livenessDecidingStatements(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		for i, statement := range statementsIn(readSource(t, name)) {
			if !decidesLiveness(statement) {
				continue
			}
			out[fmt.Sprintf("%s statement %d", name, i+1)] = statement
		}
	}
	return out
}

// decidesLiveness reports whether a statement is deciding whether a passport is
// still a credential, rather than merely reading one.
//
// Reaching for the shared rule counts, and so does spelling either of its two
// own conditions by hand — which is what a new authenticator that has not been
// told about agentLivenessWhere looks like. The list surface names neither: it
// shows revoked rows on purpose, which is why the alias was never the
// discriminator.
func decidesLiveness(statement string) bool {
	for _, tell := range []string{"agentLivenessWhere", "revoked_at IS NULL", "expires_at"} {
		if strings.Contains(statement, tell) {
			return true
		}
	}
	return false
}

// statementsIn pulls the passport statements out of one file's source.
func statementsIn(source string) []string {
	var out []string
	for _, chunk := range strings.Split(source, "FROM passport p")[1:] {
		// To the end of the DECLARATION, not the end of the raw string: the
		// rule is concatenated after the closing backtick, so cutting there
		// would read every statement as carrying no rule at all. Declarations
		// in this file are separated by a blank line.
		end := strings.Index(chunk, "\n\n")
		if end < 0 {
			end = len(chunk)
		}
		out = append(out, "FROM passport p"+chunk[:end])
	}
	return out
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
