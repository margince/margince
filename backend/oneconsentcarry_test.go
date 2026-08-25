// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// The consent carry — what happens to a retiring record's consent when another
// record survives it — is spelled once inside the people module.
//
// It was spelled three times: a person merge, a lead merge, and a lead's
// promotion to a person, each with its own copy of one CTE differing only in a
// key column and a literal. Consent is the domain where a fix applied to two
// of three copies is a lawful-processing defect rather than an untidiness: the
// rule the CTE encodes is "a withdrawal always wins", and a copy that missed a
// correction turns an opt-out back into a grant.
//
// The three had already drifted, on whether the proof events follow the state,
// and the difference lived only in three prose comments in three files. It is
// declared in the spec now, and asserted against real rows by
// TestEachConsentCarryProvesItsProofRule.
//
// SCOPED TO people, deliberately. The consent module owns these tables and
// writes them for its own reasons — a preference centre save, a double
// opt-in confirmation — and those are not carries. What this gate governs is
// the SIBLING that reaches across into them under the package's sanctioned
// cross-aggregate ownership, which is where a second copy went unnoticed
// three times.
//
// WHAT IT CANNOT SEE: a carry assembled from SQL fragments that never spell
// the statement in one literal. Every write in this tree spells it whole.

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// consentStateStatement matches the writes a carry makes: the withdrawal flip,
// the proof row it appends, and the re-point that moves the state onto the
// survivor. A read of either table is not a subject — the defect is a second
// implementation of the RULE, not a second reader of the rows.
var consentStateStatement = regexp.MustCompile(
	`(?is)INSERT\s+INTO\s+consent_event\b|UPDATE\s+person_consent\b|DELETE\s+FROM\s+person_consent\b`)

// writesConsentState reports whether a file carries any of those statements.
func writesConsentState(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || !consentStateStatement.MatchString(lit.Value) {
			return !found
		}
		found = true
		return false
	})
	return found
}

func TestTheConsentCarryIsSpelledOnceInPeople(t *testing.T) {
	scope := gatekit.Scope{
		Roots:   []string{"internal/modules/people"},
		Subject: writesConsentState,
		Exempt: gatekit.Waive(map[string]string{
			"internal/modules/consent/store.go": "the consent module OWNS these tables and writes them for its own reasons — a preference-centre save, a double opt-in confirmation, a withdrawal a person asked for. None of those is a carry: they record what a subject decided, where a carry decides what happens to a decision when the record holding it retires. The two would not share an implementation even if they were in one package",
		}),
	}
	inside := scope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("the people module writes consent state from %d files:\n\t%s\n\n"+
			"One carry, with its differences declared in the spec rather than left to a reader comparing "+
			"files. A withdrawal that wins in two copies and not the third is a lawful-processing defect",
			len(inside), strings.Join(where, "\n\t"))
	}
}
