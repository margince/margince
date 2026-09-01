// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates_test

// A provider's match rules are checked against IdentifierFields(), so that
// list has to name every field a person actually carries.
//
// The failure this holds is the silent-short kind. If PersonIdentifiers gains
// a field and IdentifierFields() does not, then two things break in the same
// direction and neither reports anything: the registry stops rejecting a rule
// that names the new field, and `present` — which switches on the same closed
// set — answers false for it, so a rule naming it matches nobody. A provider
// declaring that rule looks up nobody at all, and refusing to spend money
// never looks like a fault.
//
// So the corpus is derived from the struct by reflection rather than listed
// here. A list would be the second copy of the thing it is checking.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// TestEveryIdentifierFieldIsNamedAndUnderstood pins the count against the
// struct.
//
// The wire spelling is NOT derived from the Go name here. `LinkedInURL` is one
// word to a reader and three runs of capitals to an algorithm, so a converter
// would have to be taught the exception — and a gate carrying a special case
// for its own subject has become a second copy of it. What can be derived
// without guessing is how MANY fields there are, and the companion test below
// proves each declared name is one the reader actually understands. Together
// those close the gap: a new field that nobody named fails the count, and a
// name that no reader knows fails the second test.
//
// Held by: this test (backend/gates/identifierfields_test.go)
func TestEveryIdentifierFieldIsNamedAndUnderstood(t *testing.T) {
	t.Parallel()

	declared := map[provider.IdentifierField]bool{}
	for _, f := range provider.IdentifierFields() {
		if declared[f] {
			t.Errorf("IdentifierFields() names %q twice, so the count below can pass while a field is unnamed", f)
		}
		declared[f] = true
	}

	shape := reflect.TypeOf(provider.PersonIdentifiers{})
	if shape.NumField() == 0 {
		t.Fatal("PersonIdentifiers has no fields, so this gate would pass over an empty subject")
	}

	if len(declared) != shape.NumField() {
		var carried []string
		for i := range shape.NumField() {
			carried = append(carried, shape.Field(i).Name)
		}
		t.Errorf("PersonIdentifiers carries %d fields (%s) but IdentifierFields() names %d.\n\n"+
			"A field nobody named cannot be rejected by the registry when a rule misspells it, and a rule "+
			"that does name it matches nobody — so a provider declaring one looks up no contact at all, "+
			"with no error anywhere. Add the constant, add it to IdentifierFields(), and teach `present` "+
			"to read it.",
			shape.NumField(), strings.Join(carried, ", "), len(declared))
	}
}

// A field every real subject carries must actually be readable. The census
// above compares NAMES; this proves the reader agrees, because a field listed
// and understood are two different things and only one of them spends money.
func TestEveryNamedIdentifierIsReadable(t *testing.T) {
	t.Parallel()

	// Every field set to a non-empty value, so a rule on any single one of
	// them must be satisfied. A field `present` does not know answers false
	// here even though the subject carries it.
	full := provider.PersonIdentifiers{
		LinkedInURL:   "https://www.linkedin.com/in/someone",
		FirstName:     "Anna",
		LastName:      "Muster",
		CompanyName:   "Example GmbH",
		CompanyDomain: "example.com",
	}
	shape := reflect.TypeOf(full)
	value := reflect.ValueOf(full)
	for i := range shape.NumField() {
		if value.Field(i).String() == "" {
			t.Fatalf("the fixture leaves PersonIdentifiers.%s empty, so this gate cannot tell a field "+
				"`present` does not know from one the fixture forgot", shape.Field(i).Name)
		}
	}

	for _, f := range provider.IdentifierFields() {
		rule := []provider.MatchRule{{AllOf: []provider.IdentifierField{f}}}
		if !full.Matchable(rule) {
			t.Errorf("a rule requiring only %q is not satisfied by a subject carrying every identifier: "+
				"`present` does not read that field, so any provider declaring it looks nobody up", f)
		}
	}
}
