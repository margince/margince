// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The confirm page shows a data subject their own record, and only that.
//
// It is the one surface in this tree that discloses a person's record to
// somebody holding no principal at all — the authority is a token in a mailbox.
// So the question "which fields may cross it" has a different answer here than
// anywhere else, and the answer has to be a decision somebody made rather than
// whatever the read model happened to carry.
//
// The failure this prevents is quiet. Person360 grows a field, somebody widens
// the confirm projection to reuse it, and the workspace's own working notes —
// who owns this contact, what the model scored them, what the research lane
// wrote about them — arrive in the subject's inbox. Nothing would fail; the
// page would simply say more than it should, to the one reader who must not
// see it.
//
// WHAT IT CANNOT SEE: a value smuggled inside an allowed field. Putting a
// lifecycle stage into the `title` string would pass. The projection reads
// named columns and nothing computes those strings, which is what keeps that
// awkward rather than merely forbidden.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// theConfirmProjection is the read this gate judges.
const theConfirmProjection = "internal/modules/consent/confirmcard.go"

// disclosableToTheSubject is what the confirm card may name: the person's own
// contact details, the employer they already know they work for, and the
// provenance that answers Art. 14. Each is something the subject either
// supplied or is entitled to be told.
var disclosableToTheSubject = map[string]bool{
	"full_name": true,
	"title":     true,
	"email":     true,
	"phone":     true,
	// The employer, and the columns the employment read needs to find it.
	"display_name":    true,
	"organization_id": true,
	"person_id":       true,
	"kind":            true,
	"ended_at":        true,
	"archived_at":     true,
	"created_at":      true,
	"is_primary":      true,
	"id":              true,
	// The Art. 14 answer.
	"field_name":  true,
	"source":      true,
	"captured_at": true,
	"object_type": true,
	"object_id":   true,
	// The subject's own consent state, so somebody who already said yes is not
	// asked as though they had not.
	"state":      true,
	"purpose_id": true,
	"key":        true,
}

// columnRe finds the column names a SQL literal reads: a dotted reference
// behind a table alias. Read from the raw-string SQL only, because the Go
// around it is full of dots that are not columns.
var columnRe = regexp.MustCompile(`\b[a-z_]+\.([a-z_]+)\b`)

// sqlLiteralRe isolates the backtick-quoted SQL in this file, so the walk reads
// the statements rather than the code that runs them.
var sqlLiteralRe = regexp.MustCompile("(?s)`([^`]*(?:SELECT|FROM)[^`]*)`")

func TestTheConfirmCardDisclosesOnlyWhatItsSubjectMaySee(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(theConfirmProjection)
	if err != nil {
		t.Fatalf("read the confirm projection: %v", err)
	}

	statements := sqlLiteralRe.FindAllStringSubmatch(string(source), -1)
	if len(statements) == 0 {
		t.Fatalf("no SQL found in %s — the extractor has stopped reading it", theConfirmProjection)
	}
	seen := map[string]bool{}
	for _, statement := range statements {
		for _, match := range columnRe.FindAllStringSubmatch(statement[1], -1) {
			seen[match[1]] = true
		}
	}
	// A census that judged nothing certifies nothing: this file is a SQL read,
	// so finding no columns means the extractor stopped working rather than the
	// projection having narrowed.
	if len(seen) < 8 {
		t.Fatalf("only %d column(s) found in %s — the extractor has stopped reading it",
			len(seen), theConfirmProjection)
	}

	var undeclared []string
	for column := range seen {
		if !disclosableToTheSubject[column] {
			undeclared = append(undeclared, column)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		t.Errorf("the confirm card reads %s, which nobody has declared disclosable to the subject.\n"+
			"  This surface answers a caller holding no principal — the authority is a token in a mailbox.\n"+
			"  A field that reaches it reaches the person it is about, so add it to disclosableToTheSubject\n"+
			"  with the reason they may see it, or take it out of the read.",
			strings.Join(undeclared, ", "))
	}
}
