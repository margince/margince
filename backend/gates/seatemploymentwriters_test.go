// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every function in a module that infers an employment edge asks whether the
// contact is a workspace seat first. A colleague's employer is the workspace,
// so an edge inferred from their mail elsewhere is false.
//
// A subject records the provenance of an inferred edge, by calling
// auditCapturedEmployment. Keying on that rather than on the insert's text
// survives a statement a helper assembles.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// seatEmploymentGuards are the contacts functions that read the seat set under
// its lock (contacts/seatguard.go).
var seatEmploymentGuards = []string{"contactIsSeatTx", "seatContactsTx"}

// capturedEmploymentAudit is the helper an inferred employment write records
// its provenance through, and its callers are this census.
//
// The generic relationship create needs no waiver now. It stores what a human
// asserted and records no inferred provenance, so it is not a subject. That
// waiver was how an inferred caller of it inherited an exemption.
const capturedEmploymentAudit = "auditCapturedEmployment"

func TestEveryInferredEmploymentInsertAsksWhetherTheContactIsASeat(t *testing.T) {
	t.Parallel()
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
			if !writesInferredEmployment(fn) {
				continue
			}
			subject := slash + ":" + fn.Name.Name
			subjects = append(subjects, subject)
			if !callsAnyOf(fn, seatEmploymentGuards) {
				findings = append(findings, subject)
			}
		}
	}
	for _, guard := range seatEmploymentGuards {
		if !declared[guard] {
			t.Fatalf("contacts/seatguard.go no longer declares %s, so this gate asks for a guard that is not there", guard)
		}
	}
	// Three today: capture, provider claim and domain triage. The human create
	// is no longer among them, because it records no inferred provenance.
	//
	// A floor cannot notice a subject that stopped matching. The tests below
	// hold the matcher against planted sources instead.
	if len(subjects) < 3 {
		t.Fatalf("found %d inferred employment writers (%v), want at least 3: the walk missed writers that exist", len(subjects), subjects)
	}
	if len(findings) > 0 {
		t.Errorf("these functions insert an employment edge without asking whether the contact is a workspace seat:\n  %s\n"+
			"Call contactIsSeatTx (one contact) or seatContactsTx (many) before the insert, so a colleague "+
			"is never filed as an employee of the company on their mail.", strings.Join(findings, "\n  "))
	}
}

// insertsEmployment reports whether the function holds both halves of an
// employment insert, in one literal or in two.
// writesInferredEmployment reports whether fn records the provenance of an
// inferred employment edge. It reads the call and not the statement, so a
// writer whose SQL comes from a helper or a constant stays in the census.
func writesInferredEmployment(fn *ast.FuncDecl) bool {
	return callsAnyOf(fn, []string{capturedEmploymentAudit})
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

// The matcher's own evidence. A census keyed on an insert's text loses a
// writer whose statement moved into a helper. The floor goes on passing,
// because the writers that remain still clear it.
func TestTheSeatCensusSeesAWriterWhoseStatementCameFromAHelper(t *testing.T) {
	t.Parallel()
	// No INSERT and no 'employment' anywhere in the body: the statement is a
	// constant and the kind rides an argument.
	fn := parseOneFunc(t, `package p
func plantFromAHelper(ctx context.Context, tx pgx.Tx) error {
	edgeID, err := insertRelationshipRow(ctx, tx, employmentInsertSQL, employmentKind)
	if err != nil {
		return err
	}
	return auditCapturedEmployment(ctx, tx, edgeID, contactID, companyID, relationshipOriginCapture)
}`)
	if !writesInferredEmployment(fn) {
		t.Error("a writer whose statement came from a helper left the census, so nothing would ask it for a seat check")
	}
}

func TestTheSeatCensusSeesAnInferredCallerOfTheGenericWriter(t *testing.T) {
	t.Parallel()
	// The shape the old waiver exempted: an inferred path reaching the generic
	// create. It is a subject because it records inferred provenance.
	fn := parseOneFunc(t, `package p
func inferEmploymentFromMail(ctx context.Context, tx pgx.Tx) error {
	edge, err := CreateRelationshipTx(ctx, tx, in)
	if err != nil {
		return err
	}
	return auditCapturedEmployment(ctx, tx, edge, contactID, companyID, relationshipOriginCapture)
}`)
	if !writesInferredEmployment(fn) {
		t.Error("an inferred caller of the generic writer left the census, which is how the old waiver spread")
	}
}

// A human-asserted write records no inferred provenance, so it is not a
// subject. Without this the two above would pass on a matcher that said yes to
// everything.
func TestTheSeatCensusLeavesAHumanAssertedWriteAlone(t *testing.T) {
	t.Parallel()
	fn := parseOneFunc(t, `package p
func createRelationshipByHand(ctx context.Context, tx pgx.Tx) error {
	_, err := insertRelationshipRow(ctx, tx, "INSERT INTO relationship (kind) VALUES ('employment')", nil)
	return err
}`)
	if writesInferredEmployment(fn) {
		t.Error("a human-asserted create reads as inferred, so the census would demand a seat check that would overrule a human")
	}
}

// parseOneFunc returns the function declaration src carries.
func parseOneFunc(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", src, 0)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatal("the planted source declares no function")
	return nil
}
