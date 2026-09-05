// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The audience gate is spelled twice, and the integration suite holds the pair
// arm by arm — for the arms somebody thought to list.
//
// That is the hole this file closes. A NEW arm added to one spelling has no case
// over there, so a hand-enumerated parity suite passes while the two disagree
// about a shape nobody wrote down. This asks the question the enumeration
// cannot: do both SQL texts draw on the same set of relations?
//
// It reads the rendered SQL rather than a list kept beside it, because a list is
// the second copy of the subject that AGENTS.md warns a gate not to become. Add
// a table to one side and this fails naming it, whichever side grew it.
//
// The two are not identical queries and are not meant to be — the existential
// resolves subjects the read clause receives ready-made — so what the existential
// may hold BEYOND the shared arms is named here, each with the reason it is
// resolution rather than an arm of its own. That list is the honest cost of the
// second spelling and it is meant to stay short.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// resolutionOnlyRelations are the relations the existential twin reads that are
// not arms — they resolve, per row, a value the read clause is simply HANDED by
// the principal. Each is a claim, not an exemption: if one of these ever starts
// deciding who may read, it belongs in the read clause too and this list is
// where the next author is told to look.
var resolutionOnlyRelations = gatekit.Waive(map[string]string{
	// The read clause is given the caller's own uuid; the existential has to
	// ask which uuids exist at all to match the captured_by suffix arm.
	"app_user": "resolves the captured_by suffix arm against every user, where the read clause is handed one",
	// The read clause is given p.TeamIDs, already filtered to live teams by
	// identity's loadGrants; the existential has to do that filtering itself.
	"team_membership": "resolves a selected team to its members, which the read clause receives as p.TeamIDs",
	"team":            "carries loadGrants' own archived_at filter into the team resolution above",
	// The existential is a standalone query and names its own subject; the read
	// clause is a fragment composed into somebody else's FROM.
	"activity": "the existential is a whole query and names the row it is about",
})

// sqlOf reads a function's query out of its own source.
//
// The query stays INLINE in the function it runs in, where the restricted-reader
// census can see it: that gate attributes a reader by walking function bodies for
// the activity table, and hoisting the text to a package-level constant took the
// reader out of its view entirely — it reported PASS with no waiver and no
// finding, which is the shape AGENTS.md calls a census that has already failed.
// So the text is not moved to suit this test; this test goes and reads it, which
// is also what keeps there being exactly one copy of it.
func sqlOf(t *testing.T, file, fn string) string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	var sql strings.Builder
	for _, decl := range parsed.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if !ok || d.Name.Name != fn {
			continue
		}
		ast.Inspect(d, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				sql.WriteString(lit.Value)
				sql.WriteString("\n")
			}
			return true
		})
	}
	if sql.Len() == 0 {
		t.Fatalf("found no query in %s:%s — the extraction is broken, so every comparison is vacuous", file, fn)
	}
	return sql.String()
}

// relationsIn collects the relations a SQL text reads, from its own FROM and
// JOIN clauses. Derived from the text so neither spelling can grow a source this
// test does not see.
func relationsIn(sql string) map[string]bool {
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)`).FindAllStringSubmatch(sql, -1) {
		out[strings.ToLower(m[1])] = true
	}
	return out
}

// TestBothSpellingsOfTheAudienceGateDrawOnTheSameArms is the fitness function
// behind the hand-enumerated parity cases: it fails when either spelling grows a
// relation the other lacks, without anybody having to think of the case.
func TestBothSpellingsOfTheAudienceGateDrawOnTheSameArms(t *testing.T) {
	// A fixture principal, only so the clause renders: what is under test is
	// which relations the SQL names, which no principal changes.
	n := 0
	arg := func(any) int { n++; return n }
	readSide := relationsIn(activityAudienceArm(principal.Principal{
		Type: principal.PrincipalHuman, UserID: ids.NewV7(),
	}, "a", arg))
	if len(readSide) == 0 {
		t.Fatal("the read clause names no relation at all — the extraction is broken, so every comparison below is vacuous")
	}
	existential := relationsIn(sqlOf(t, "audienceorphan.go", "ActivityHasAReaderTx"))

	var missing, extra []string
	for rel := range readSide {
		if !existential[rel] {
			missing = append(missing, rel)
		}
	}
	for rel := range existential {
		if readSide[rel] {
			continue
		}
		if !resolutionOnlyRelations.Waived(t, rel) {
			extra = append(extra, rel)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("the read clause admits readers through %v and the orphan refusal does not look there — "+
			"a legal audience write will be refused as leaving nobody", missing)
	}
	// A named relation that no longer appears is ratification of SQL that is
	// gone, which is how a waiver list turns into a list of stale claims.
	resolutionOnlyRelations.AssertAllMatched(t)

	if len(extra) > 0 {
		t.Errorf("the orphan refusal reads %v, which no longer admits anybody through the read clause — "+
			"it will report a reader for a row nobody can open, which is the orphan admitted silently. "+
			"If one of these resolves a value rather than deciding a reader, name it in resolutionOnlyRelations with that reason", extra)
	}
}
