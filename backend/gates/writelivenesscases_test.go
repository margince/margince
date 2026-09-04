// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// What the liveness census judges, and what it credits, driven with SYNTHETIC
// source rather than the tree — the same reason updateguardcases_test.go gives
// for its own. That census is meant to pass, so a reader proven only by "the
// tree is clean" is one that keeps passing after it stops working: a write it
// stops recognising produces no finding, only a smaller silence.
//
// Two halves fail differently and both are planted here. A write the SUBJECT
// test stops seeing leaves the tree unjudged; a marker the CREDIT test starts
// seeing everywhere waives the tree without a waiver. The first is the one that
// goes quiet, so most cases below are its.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// retirableForCases is the table set the cases are judged against, written out
// rather than derived: a case that shared the census's own derivation would pass
// whenever that derivation broke.
var retirableForCases = map[string]bool{"organization": true, "activity": true}

// livenessCase is one synthetic function and the two verdicts it is owed.
type livenessCase struct {
	name   string
	source string
	// subject says whether this function writes one retirable row by id at all;
	// stated says whether it answers the liveness question. A function that is
	// not a subject is never asked the second.
	subject bool
	stated  bool
	// answers is the per-TABLE verdict on the in-statement half: which tables
	// this function refuses an archived row of in the text of the write itself.
	// A Go marker covers the frame and appears here as nothing, which is what
	// the two-writes case below is for.
	answers map[string]bool
}

// allAnswered reports whether every table this function writes refuses an
// archived row in its own statement.
func allAnswered(written map[string]bool) bool {
	for _, answered := range written {
		if !answered {
			return false
		}
	}
	return len(written) > 0
}

var livenessCases = []livenessCase{
	{
		name: "a by-id update of a retirable table, answering nothing",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1`" + `) }`,
		subject: true,
	}, {
		name: "the same write, refusing an archived row in its own predicate",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1 AND archived_at IS NULL`" + `) }`,
		subject: true, stated: true, answers: map[string]bool{"organization": true},
	}, {
		// The restore arm's predicate is the OPPOSITE claim, and crediting it
		// would hand every un-archive path a free pass.
		name: "archived_at IS NOT NULL, which is a reach for a retired row rather than a refusal",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE organization SET archived_at = NULL WHERE id = $1 AND archived_at IS NOT NULL`" + `) }`,
		subject: true,
	}, {
		name: "refused through the live probe",
		source: `package p
func write(tx T) {
	auth.EnsureWritableLive(ctx, tx, "organization", id)
	tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1`" + `)
}`,
		subject: true, stated: true,
	}, {
		// The declaration half. It must be credited or the pair is useless —
		// a retraction would have to be waived, and a waiver is not a spelling
		// a call site can be read for.
		name: "declared as a retraction",
		source: `package p
func write(tx T) {
	auth.EnsureRetractable(ctx, tx, "organization", id)
	tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1`" + `)
}`,
		subject: true, stated: true,
	}, {
		name: "declared through the lock filter",
		source: `package p
func write(tx T) {
	storekit.LockRow(ctx, tx, "organization", id, storekit.IncludeArchived)
	tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1`" + `)
}`,
		subject: true, stated: true,
	}, {
		// The unqualified spelling: identity calls its own LiveMemberSQL with
		// no package name in front of it, so a walk that read only qualified
		// selectors judged seven login-path writes as answering nothing.
		name: "refused through a same-package helper called without a qualifier",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE activity SET body = NULL WHERE id = $1 AND `" + ` + LiveMemberSQL("")) }`,
		subject: true, stated: true,
	}, {
		// Out of scope BY CONSTRUCTION, and the boundary has to hold: a cascade
		// over a parent's children takes its liveness from the parent that
		// chose them, and pulling it in would waive most of the privacy module.
		name: "a set-based write, which is not a by-id write",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE activity SET archived_at = now() WHERE organization_id = $1`" + `) }`,
	}, {
		name: "an insert, which creates a row rather than reaching a standing one",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`INSERT INTO activity (id, subject) VALUES ($1, $2)`" + `) }`,
	}, {
		name: "a by-id delete, which reaches a standing row exactly as an update does",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`DELETE FROM activity WHERE id = $1`" + `) }`,
		subject: true,
	}, {
		name: "a by-id write under a table alias",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE activity a SET subject = $2 WHERE a.id = $1`" + `) }`,
		subject: true,
	}, {
		// A table with no archived_at has no archived row to refuse, and asking
		// it the question would produce a waiver that says nothing.
		name: "a by-id write of a table that cannot be retired",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE audit_log SET action = $2 WHERE id = $1`" + `) }`,
	}, {
		// The shape the tree already writes: the organization column writers
		// hold their statements in package-level tables, so a reader of body
		// literals alone judged none of them.
		name: "held in a package-level table the function indexes",
		source: `package p
var held = map[string]string{"legal_name": ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1`" + `}
func write(tx T, column string) { tx.Exec(ctx, held[column]) }`,
		subject: true,
	}, {
		// CodeRabbit's case, and the reason the in-statement half is attributed
		// per table: a function that guards one write and leaves its sibling
		// bare answers for one table and not the other, and the bare one has to
		// be reported anyway.
		name: "one write guarded and its sibling in the same function bare",
		source: `package p
func write(tx T) {
	tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1 AND archived_at IS NULL`" + `)
	tx.Exec(ctx, ` + "`UPDATE activity SET subject = $2 WHERE id = $1`" + `)
}`,
		subject: true,
		answers: map[string]bool{"organization": true, "activity": false},
	}, {
		// The marker must be a call site, not prose. A gate that read comments
		// would let a sentence about liveness stand in for one.
		name: "a marker named only in a comment",
		source: `package p
func write(tx T) {
	// EnsureWritableLive is what this ought to take.
	tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1`" + `)
}`,
		subject: true,
	},
}

func TestTheLivenessCensusJudgesAWriteAndCreditsOnlyAnAnswer(t *testing.T) {
	t.Parallel()
	for _, tc := range livenessCases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "synthetic.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the case source: %v", err)
			}
			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if candidate, isFunc := decl.(*ast.FuncDecl); isFunc && candidate.Name.Name == "write" {
					fn = candidate
				}
			}
			if fn == nil {
				t.Fatal("the case source declares no write function, so it proves nothing")
			}
			statements := statementsJudged(fn, packageLevelStatements([]*ast.File{file}), nil)

			written := byIDWritesIn(statements, retirableForCases)
			if subject := len(written) > 0; subject != tc.subject {
				t.Fatalf("judged as a by-id write of a retirable row = %t, want %t — a write this census "+
					"does not judge is one it reports PASS over rather than reporting a gap in", subject, tc.subject)
			}
			if !tc.subject {
				return
			}
			stated := statesLivenessInGo(fn)
			for table, answered := range written {
				if answered != tc.answers[table] {
					t.Errorf("%s credited as refused in its own statement = %t, want %t",
						table, answered, tc.answers[table])
				}
			}
			if !stated && !allAnswered(written) && tc.stated {
				t.Errorf("credited as answering for liveness = false, want true")
			}
			if (stated || allAnswered(written)) && !tc.stated {
				t.Errorf("credited as answering for liveness = true, want false")
			}
		})
	}
}
