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
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// photoKeyAssignment matches the column being given a value in a SQL string —
// `photo_object_key = <something>` in an UPDATE, or the column named in an
// INSERT's column list.
var photoKeyAssignment = regexp.MustCompile(`photo_object_key\s*(=\s*(\S+)|[,)])`)

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
			for _, match := range photoKeyAssignment.FindAllStringSubmatch(statement, -1) {
				if strings.EqualFold(strings.Trim(match[2], " ,"), "NULL") {
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
					parsed.Path, strings.TrimSpace(match[0]))
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
