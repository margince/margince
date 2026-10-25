// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every function in a module that inserts an employment edge asks whether the
// contact is a workspace seat first. A colleague's employer is the workspace,
// so an edge inferred from their mail at another company is false. The
// subjects are found in the code: a function holding an INSERT INTO
// relationship and the 'employment' kind.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// seatEmploymentGuards are the contacts functions that read the seat set under
// its lock (contacts/seatguard.go).
var seatEmploymentGuards = []string{"contactIsSeatTx", "seatContactsTx"}

// humanAssertedEmployment are the inserts a human or an agent asks for by name.
var humanAssertedEmployment = gatekit.Waive(map[string]string{
	"internal/modules/contacts/relationshipinsertsql.go:insertRelationshipRow": "the generic relationship create stores what a human or an agent asserted about a contact; refusing a seat there would overrule a human's own record",
})

var (
	relationshipInsert = regexp.MustCompile(`(?i)INSERT\s+INTO\s+relationship\s*\(`)
	employmentLiteral  = regexp.MustCompile(`'employment'`)
)

func TestEveryInferredEmploymentInsertAsksWhetherTheContactIsASeat(t *testing.T) {
	t.Parallel()
	defer humanAssertedEmployment.AssertAllMatched(t)
	declared := map[string]bool{}
	var subjects, findings []string
	for _, path := range handWrittenGoSources(t) {
		slash := filepath.ToSlash(path)
		if !strings.HasPrefix(slash, "internal/modules/") || strings.HasSuffix(slash, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if slash == "internal/modules/contacts/seatguard.go" && slices.Contains(seatEmploymentGuards, fn.Name.Name) {
				declared[fn.Name.Name] = true
			}
			if !insertsEmployment(fn) {
				continue
			}
			subject := slash + ":" + fn.Name.Name
			subjects = append(subjects, subject)
			if !callsAnyOf(fn, seatEmploymentGuards) && !humanAssertedEmployment.Waived(t, subject) {
				findings = append(findings, subject)
			}
		}
	}
	for _, guard := range seatEmploymentGuards {
		if !declared[guard] {
			t.Fatalf("contacts/seatguard.go no longer declares %s, so this gate asks for a guard that is not there", guard)
		}
	}
	// Four today: capture, provider claim, domain triage and the human create.
	if len(subjects) < 4 {
		t.Fatalf("found %d employment inserts (%v), want at least 4: the walk missed writers that exist", len(subjects), subjects)
	}
	if len(findings) > 0 {
		t.Errorf("these functions insert an employment edge without asking whether the contact is a workspace seat:\n  %s\n"+
			"Call contactIsSeatTx (one contact) or seatContactsTx (many) before the insert, so a colleague "+
			"is never filed as an employee of the company on their mail.", strings.Join(findings, "\n  "))
	}
}

// insertsEmployment reports whether the function holds both halves of an
// employment insert, in one literal or in two.
func insertsEmployment(fn *ast.FuncDecl) bool {
	var insert, employment bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text := gatekit.TextOf(lit)
		insert = insert || relationshipInsert.MatchString(text)
		employment = employment || employmentLiteral.MatchString(text)
		return true
	})
	return insert && employment
}

func callsAnyOf(fn *ast.FuncDecl, names []string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok && slices.Contains(names, ident.Name) {
				found = true
			}
		}
		return !found
	})
	return found
}
