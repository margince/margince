// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// people.writePersonProfileField is the one writer of person_profile_field, and the
// one place the precedence rule lives: a machine fill claims an unanswered
// field, a human's acceptance replaces what is there.
//
// That was five hand-copied INSERTs, three of them DO NOTHING and one DO
// UPDATE, each arguing its own conflict clause in a comment beside it. Nothing
// could see the disagreement: all five live inside ONE package, and
// tableownership_test.go is keyed `package:table`, so its waiver model is blind
// to intra-package duplicates by design. This is what sees them.
//
// It judges the STATEMENT and not the file: a file may hold the writer's own
// insert and a read of the same table, and asking whether both shapes appear
// somewhere in one file reports a pairing nobody wrote.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// personProfileFieldInsert matches a write of the table, in either spelling the
// tree uses: the column list on the same line as the table, or on the next.
// Whitespace-tolerant rather than line-based, because two of the five copies
// wrapped after the table name and a line-keyed census would have seen three.
var personProfileFieldInsert = regexp.MustCompile(`INSERT\s+INTO\s+person_profile_field`)

// personProfileFieldOwner is where the writer this census requires lives. Its
// own statement is the definition rather than a copy of it.
const personProfileFieldOwner = "internal/modules/people/personprofilefieldwrite.go"

// bulkRelinkIsNotAFill ratifies the merge carry-over, which is the one write of
// this table that writePersonProfileField cannot serve.
//
// Keyed by FILE, so a second statement appearing in mergerelink.go is still a
// finding: the ratification covers the write that exists, not the topic.
var bulkRelinkIsNotAFill = gatekit.Waive(map[string]string{
	"internal/modules/people/mergerelink.go": "re-homes existing rows whole at a merge, all of a person's fields in one statement, rather than deciding one fill — it examines no value and writes none of its own",
})

func TestEveryProfileFieldWriteUsesTheOneWriter(t *testing.T) {
	// A ratification that stops matching describes a write that has moved or
	// been folded in, and leaving it quietly re-exempts whatever takes its name.
	defer bulkRelinkIsNotAFill.AssertAllMatched(t)

	fset := token.NewFileSet()
	var findings []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		if filepath.ToSlash(path) == personProfileFieldOwner {
			continue
		}
		// PRODUCT writes, and the boundary is architectural rather than a
		// shortcut: writePersonProfileField is unexported, so a fixture in another
		// package could not call it however much it wanted to, and a test that
		// seeds a prior state is not answering the question this census asks —
		// which production path writes the row, and under whose authority. The
		// rule that a test must seed through the real writer is held where a
		// test CAN reach one; it is not this gate's subject.
		if strings.HasSuffix(filepath.Base(path), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			for _, sql := range personProfileFieldStatements(decl) {
				judged++
				if !personProfileFieldInsert.MatchString(sql) {
					continue
				}
				if bulkRelinkIsNotAFill.Waived(t, filepath.ToSlash(path)) {
					continue
				}
				findings = append(findings, fmt.Sprintf("%s: %s", path, firstPersonProfileFieldLine(sql)))
			}
		}
	}
	// A census that judged nothing certifies nothing. The floor sits far below
	// the real count so it catches a broken walk, not a changing tree.
	if judged < 5 {
		t.Fatalf("only %d statement(s) naming person_profile_field were judged, so this census covered almost nothing", judged)
	}
	if len(findings) == 0 {
		return
	}
	t.Errorf("these statements write person_profile_field by hand:\n  %s\n\n"+
		"people.writePersonProfileField is the one writer, and the ON CONFLICT clause is not the "+
		"caller's to choose: it follows from WHO is writing — a machine fill claims an "+
		"unanswered field (claimUnanswered), a human's acceptance replaces what is there "+
		"(replaceOnAcceptance). A hand-written clause is a second answer to that question.",
		strings.Join(findings, "\n  "))
}

// personProfileFieldStatements returns the SQL statements in a declaration that name
// the table at all — the candidate set this census counts against.
func personProfileFieldStatements(decl ast.Decl) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, helperScope{names: map[string]bool{"writePersonProfileField": true}})
		if !ok || !strings.Contains(text, "person_profile_field") {
			return true
		}
		out = append(out, text)
		return true
	})
	return out
}

// firstPersonProfileFieldLine points the report at the offending line rather than
// dumping the statement.
func firstPersonProfileFieldLine(sql string) string {
	lines := strings.Split(sql, "\n")
	for _, line := range lines {
		if strings.Contains(line, "person_profile_field") {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(lines[0])
}

// The census above passes identically over a clean tree and over a detector
// that has stopped detecting. These read the detector directly.
var personProfileFieldProbes = []struct {
	name  string
	fires bool
	src   string
}{
	{"the VALUES form three of the five copies used", true, "\nfunc write() string {\n\treturn `INSERT INTO person_profile_field (person_id, field, value) VALUES ($1, $2, $3) ON CONFLICT (person_id, field) DO NOTHING`\n}"},
	// Two of the five wrapped after the table name, which is where a
	// line-keyed census goes blind.
	{"the column list wrapped onto the next line", true, "\nfunc write() string {\n\treturn `INSERT INTO person_profile_field\n\t  (person_id, field, value)\n\tSELECT $1, $2, $3`\n}"},
	{"the DO UPDATE form, which is the disagreement itself", true, "\nfunc write() string {\n\treturn `INSERT INTO person_profile_field (person_id, field, value) VALUES ($1, $2, $3) ON CONFLICT (person_id, field) DO UPDATE SET value = EXCLUDED.value`\n}"},
	{"the statement split across a concatenation", true, "\nfunc write() string {\n\treturn `INSERT INTO ` + `person_profile_field (person_id, field) VALUES ($1, $2)`\n}"},
	{"an insert held inside a formatter's argument", true, "\nfunc write() string {\n\treturn fmt.Sprintf(`INSERT INTO person_profile_field (person_id, field) VALUES ($1, $2) %s`, tail)\n}"},

	// Reads and deletes are somebody else's rule: erasure and the retention
	// sweep clear this table deliberately, and every render of it is a read.
	{"a read of the table", false, "\nfunc read() string {\n\treturn `SELECT value FROM person_profile_field WHERE person_id = $1`\n}"},
	{"the erasure delete", false, "\nfunc erase() string {\n\treturn `DELETE FROM person_profile_field WHERE person_id = $1`\n}"},
	{"a join naming the table", false, "\nfunc read() string {\n\treturn `SELECT 1 FROM person p JOIN person_profile_field f ON f.person_id = p.id AND f.field = 'org_name'`\n}"},
}

func TestTheProfileFieldWriteDetectorSeesWhatItClaimsTo(t *testing.T) {
	fset := token.NewFileSet()
	for _, tc := range personProfileFieldProbes {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(fset, "probe.go", "package probe\n"+tc.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			hit := false
			for _, decl := range file.Decls {
				for _, sql := range personProfileFieldStatements(decl) {
					if personProfileFieldInsert.MatchString(sql) {
						hit = true
					}
				}
			}
			if tc.fires && !hit {
				t.Errorf("the detector missed a hand-written write — the census would read green over this:\n%s", tc.src)
			}
			if !tc.fires && hit {
				t.Errorf("the detector reported a statement that does not write the table:\n%s", tc.src)
			}
		})
	}
}
