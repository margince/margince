// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A subject's photo may not be STORED until erasure can destroy the bytes.
//
// `person.photo_object_key` names an object in the blob store. The Art. 17
// cascade clears the column, because a pointer to a subject's photograph is a
// record of the subject — but clearing a pointer is not destroying what it
// points at, and the attachment arm purges only rows in `attachment`. Erasing
// a person today therefore forgets where their photograph is while the
// photograph stays where it was.
//
// That is not a live defect: nothing in this tree writes the column, so no key
// and no object exist. It becomes one the day somebody builds the upload —
// silently, because the erasure will keep passing every test it has, and the
// only thing that changed is that the pointer it nulls now pointed somewhere.
//
// So the gap is held rather than written down. The rule is that the column
// takes no value but NULL; an author who needs it to take one is the author
// who must also purge the object, the way eraseAttachments does — read the
// keys, delete the objects FIRST, then clear the rows, inside the erasure's
// own transaction.

import (
	"go/ast"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// photoKeyAssignment matches the column being given a value in a SQL string —
// `photo_object_key = <something>` in an UPDATE.
var photoKeyAssignment = regexp.MustCompile(`photo_object_key\s*=\s*([^,)\s]+)`)

// photoKeyInsert finds where an INSERT into person begins. The two lists after
// it are read with a paren-aware scan rather than by regex: a call or a cast in
// the VALUES row carries its own parentheses, and `[^)]*` would stop at the
// first of them and shift every position after it.
var photoKeyInsert = regexp.MustCompile(`(?is)INSERT\s+INTO\s+person\s*\(`)

// namesTheSubjectPhotoKey reports whether a file sends SQL mentioning the
// column at all — read or write. The write is judged below; the read is what
// makes the scope's negative sweep meaningful, since a file that never names
// the column cannot be hiding one.
func namesTheSubjectPhotoKey(_ string, file *ast.File) bool {
	for _, statement := range gatekit.SQLStatementsOf(file) {
		if strings.Contains(statement, "photo_object_key") {
			return true
		}
	}
	return false
}

func TestTheSubjectPhotoIsNeverStoredWhereErasureCannotFollow(t *testing.T) {
	t.Parallel()

	writes := 0
	for _, parsed := range (gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: namesTheSubjectPhotoKey,
	}).Files(t) {
		for _, statement := range gatekit.SQLStatementsOf(parsed.File) {
			// A SELECT reading the column is fine and common — person360 draws
			// the avatar from it. Only a statement that puts a value there owes
			// the erasure a purge.
			if !storesAValue(statement) || !strings.Contains(statement, "photo_object_key") {
				continue
			}
			for _, stored := range photoKeyValues(statement) {
				if isNull(stored) {
					continue
				}
				writes++
				t.Errorf("%s gives person.photo_object_key a value (%q).\n"+
					"\tThe Art. 17 cascade clears this column, and clearing a pointer does not destroy "+
					"the object it names — the attachment arm purges only rows in `attachment`, so an "+
					"erased subject's photograph would stay in the store with nothing left naming it.\n"+
					"\tPurge the object in the eraser first, the way eraseAttachments does: read the "+
					"keys, delete the objects, then clear the rows, in the erasure's own transaction. "+
					"Then this gate has nothing to hold and can go.",
					parsed.Path, stored)
			}
		}
	}
	if writes > 0 {
		t.Logf("%d write(s) found; the erasure purges no photo object", writes)
	}
}

// storesAValue reports whether the statement is one that puts data in a row.
func storesAValue(statement string) bool {
	upper := strings.ToUpper(statement)
	return strings.Contains(upper, "INSERT INTO") || strings.Contains(upper, "UPDATE ")
}

// photoKeyValues answers every value one statement puts in the column.
//
// An INSERT is read POSITIONALLY rather than by matching the column name where
// it appears: the name in an insert's column list has no value beside it, so a
// gate that reported the name alone would refuse `VALUES (…, NULL, …)` — an
// explicit nothing, which is what the rule allows.
func photoKeyValues(statement string) []string {
	stored := photoKeyUpdates(statement)
	for _, at := range photoKeyInsert.FindAllStringIndex(statement, -1) {
		columnList, after := parenthesized(statement, at[1]-1)
		row, ok := valuesRow(statement, after)
		if !ok {
			continue
		}
		values := splitSQLList(row)
		for i, column := range splitSQLList(columnList) {
			if !strings.EqualFold(strings.TrimSpace(column), "photo_object_key") {
				continue
			}
			if i < len(values) {
				stored = append(stored, values[i])
				continue
			}
			// Fewer values than columns is not valid SQL, and a gate that read
			// past the end would report nothing for it. Say the statement is
			// unreadable rather than agreeing with it.
			stored = append(stored, "<no value at the column's position>")
		}
	}
	return stored
}

// splitSQLList splits a comma-separated SQL list, ignoring commas inside
// nested parentheses — a cast or a function call in a VALUES row would
// otherwise shift every position after it.
func splitSQLList(list string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(list[start:]))
}

// isNull reports whether a stored value is an explicit nothing, cast or not:
// `NULL` and `NULL::text` both leave the column empty, and the rule is about
// what the column holds rather than how it was spelled.
func isNull(value string) bool {
	bare, _, _ := strings.Cut(strings.TrimSpace(value), "::")
	return strings.EqualFold(strings.TrimSpace(bare), "NULL")
}

// What the reader makes of one statement, asked directly.
//
// The INSERT arm is positional, and every case below is one somebody will
// write: an explicit nothing, a cast nothing, a real key several columns in,
// and a cast in an earlier position that would shift every value after it if
// the split were naive.
func TestThePhotoKeyReaderSeesWhatAStatementStores(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		statement string
		want      []string
	}{
		"an explicit nothing": {
			`INSERT INTO person (id, photo_object_key) VALUES ($1, NULL)`,
			[]string{"NULL"},
		},
		"a cast nothing": {
			`INSERT INTO person (id, photo_object_key) VALUES ($1, NULL::text)`,
			[]string{"NULL::text"},
		},
		"a real key": {
			`INSERT INTO person (id, full_name, photo_object_key) VALUES ($1, $2, $3)`,
			[]string{"$3"},
		},
		"a cast in an earlier column": {
			`INSERT INTO person (id, source, photo_object_key) VALUES (gen_random_uuid(), $1::text, $2)`,
			[]string{"$2"},
		},
		"an update": {
			`UPDATE person SET photo_object_key = $2 WHERE id = $1`,
			[]string{"$2"},
		},
		"a read": {
			`SELECT photo_object_key FROM person WHERE id = $1`,
			nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := photoKeyValues(c.statement)
			if !slices.Equal(got, c.want) {
				t.Errorf("read %v, want %v", got, c.want)
			}
		})
	}
}

func TestAnExplicitNothingIsNotAStoredKey(t *testing.T) {
	t.Parallel()

	for _, nothing := range []string{"NULL", "null", " NULL ", "NULL::text", "null :: uuid"} {
		if !isNull(nothing) {
			t.Errorf("%q read as a stored key — the rule is about what the column holds, not how "+
				"the nothing was spelled", nothing)
		}
	}
	for _, value := range []string{"$1", "'people/x.jpg'", "nullify(x)"} {
		if isNull(value) {
			t.Errorf("%q read as nothing", value)
		}
	}
}

// photoKeyUpdates answers the values an UPDATE assigns to the column.
func photoKeyUpdates(statement string) []string {
	var stored []string
	for _, m := range photoKeyAssignment.FindAllStringSubmatch(statement, -1) {
		stored = append(stored, m[1])
	}
	return stored
}

// parenthesized reads the balanced group starting at the "(" at open, and
// answers its contents and the index just past its ")".
func parenthesized(text string, open int) (string, int) {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[open+1 : i], i + 1
			}
		}
	}
	return "", len(text)
}

// valuesRow reads the row after a VALUES keyword, from where the column list
// ended. It answers false for an INSERT ... SELECT, which stores no literal
// row this reader can place a column in.
func valuesRow(statement string, from int) (string, bool) {
	rest := statement[from:]
	at := strings.Index(strings.ToUpper(rest), "VALUES")
	if at < 0 {
		return "", false
	}
	open := strings.Index(rest[at:], "(")
	if open < 0 {
		return "", false
	}
	row, _ := parenthesized(rest, at+open)
	return row, true
}
