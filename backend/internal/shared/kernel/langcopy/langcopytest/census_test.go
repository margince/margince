// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

// Census and NoCount are exercised elsewhere only through three well-formed
// production tables, so every branch that reports a PROBLEM has never been
// observed doing so — a census that can fail short has already failed, per
// AGENTS.md, because it would read a smaller subject and report PASS with
// nothing firing to say so. This file plants a malformed table for each
// branch and checks what the helper actually reports, plus one well-formed
// table per shape to prove the negative cases are the malformation talking
// and not the harness.
//
// White-box (this package, not langcopytest_test) so run's callback can name
// reporter directly rather than exporting a type with no second caller.

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// spy stands in for *testing.T: it records what Census/NoCount report instead
// of failing the test that calls them. Fatalf stops the walk the way the real
// one does — runtime.Goexit(), run inside a goroutine of its own so the exit
// unwinds only that call, never the test recording it.
type spy struct {
	errors []string
	fatal  string
}

func (s *spy) Helper() {}

func (s *spy) Errorf(format string, args ...any) {
	s.errors = append(s.errors, fmt.Sprintf(format, args...))
}

func (s *spy) Fatalf(format string, args ...any) {
	s.fatal = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// run calls census in its own goroutine so a Fatalf's runtime.Goexit()
// unwinds that goroutine alone, and waits for it before handing back what was
// recorded.
func run(census func(reporter)) *spy {
	s := &spy{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		census(s)
	}()
	<-done
	return s
}

func reports(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestCensusReportsAnUnwrittenLanguage(t *testing.T) {
	t.Parallel()
	type table struct{ Greeting langcopy.Phrase }
	bad := table{Greeting: langcopy.Phrase{En: "hi", Vi: "chào"}}
	s := run(func(r reporter) { Census(r, bad) })
	// A bare language code is not a safe needle: "de" is a substring of
	// "renders", which every censusEntry message carries. Tying the code to
	// the unwritten branch's own wording and the field name is a fragment no
	// other branch's message can produce.
	want := fmt.Sprintf("%s leaves Greeting unwritten", textlang.German)
	if !reports(s.errors, want) {
		t.Fatalf("an unwritten German sentence was not reported as %q: %v", want, s.errors)
	}
}

func TestCensusReportsADroppedPlaceholder(t *testing.T) {
	t.Parallel()
	type table struct{ Count langcopy.Phrase }
	bad := table{Count: langcopy.Phrase{En: "%d things", De: "Dinge", Vi: "%d thứ"}}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "Count") {
		t.Fatalf("a German sentence dropping the English %%d was not reported: %v", s.errors)
	}
}

func TestCensusReadsAPaddedPlaceholder(t *testing.T) {
	t.Parallel()
	type table struct{ Count langcopy.Phrase }
	bad := table{Count: langcopy.Phrase{En: "%02d things", De: "Dinge", Vi: "%d thứ"}}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "Count") {
		t.Fatalf("a German sentence dropping the English %%02d was not reported: %v", s.errors)
	}
	if reports(s.errors, fmt.Sprintf("%s writes", textlang.Vietnamese)) {
		t.Fatalf("a translation padding %%02d as %%d was reported as a mismatch: %v", s.errors)
	}
}

func TestCensusReportsAnAddedPlaceholder(t *testing.T) {
	t.Parallel()
	type table struct{ Fine langcopy.Phrase }
	bad := table{Fine: langcopy.Phrase{En: "fine", De: "%d fein", Vi: "ổn"}}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "Fine") {
		t.Fatalf("a German sentence adding a placeholder the English lacks was not reported: %v", s.errors)
	}
}

func TestCensusNamesABadEntryInAMapField(t *testing.T) {
	t.Parallel()
	type table struct{ Items map[string]langcopy.Phrase }
	bad := table{Items: map[string]langcopy.Phrase{
		"apple": {En: "apple", Vi: "táo"}, // German unwritten
	}}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "Items[apple]") {
		t.Fatalf("a bad map-field entry was not named Field[key]: %v", s.errors)
	}
}

func TestCensusNamesABadEntryInAWholeTableMap(t *testing.T) {
	t.Parallel()
	bad := map[string]langcopy.Phrase{
		"apple": {En: "apple", Vi: "táo"}, // German unwritten
	}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "[apple]") {
		t.Fatalf("a bad whole-table-map entry was not named [key]: %v", s.errors)
	}
}

func TestCensusRefusesAStructWithNoPhraseFields(t *testing.T) {
	t.Parallel()
	s := run(func(r reporter) { Census(r, struct{}{}) })
	if s.fatal == "" {
		t.Fatal("a struct with no phrase fields at all did not hit the certifies-nothing refusal")
	}
}

func TestCensusRefusesAnEmptyMapTable(t *testing.T) {
	t.Parallel()
	s := run(func(r reporter) { Census(r, map[string]langcopy.Phrase{}) })
	if s.fatal == "" {
		t.Fatal("an empty map table did not hit the certifies-nothing refusal")
	}
}

func TestCensusReportsAFieldThatIsNeitherAPhraseNorAMapOfThem(t *testing.T) {
	t.Parallel()
	type table struct {
		Greeting langcopy.Phrase // keeps the table non-empty so only Bogus is under test
		Bogus    string
	}
	bad := table{Greeting: langcopy.Phrase{En: "hi", De: "hallo", Vi: "chào"}, Bogus: "not a phrase"}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "Bogus") {
		t.Fatalf("a field that is not a phrase or a map of them was not reported: %v", s.errors)
	}
}

// censusField admits a map field by its Kind alone, not by its element type,
// so a field typed as a map of an interface — not the map[string]langcopy.Phrase
// every production table uses — reaches censusMap with an entry that is not
// actually a Phrase. Planted here rather than deleted as dead code: Census's
// own contract takes `any`, and censusField's admission is not narrower than
// that, so this is a real path, not a hypothetical one.
func TestCensusNamesABadEntryInAnInterfaceValuedMap(t *testing.T) {
	t.Parallel()
	type table struct {
		Greeting langcopy.Phrase // keeps the table non-empty so only Items is under test
		Items    map[string]any
	}
	bad := table{
		Greeting: langcopy.Phrase{En: "hi", De: "hallo", Vi: "chào"},
		Items:    map[string]any{"apple": "not a phrase"},
	}
	s := run(func(r reporter) { Census(r, bad) })
	if !reports(s.errors, "Items[apple] is not a phrase") {
		t.Fatalf("a map entry that is not a phrase was not reported: %v", s.errors)
	}
}

func TestNoCountReportsAPlaceholder(t *testing.T) {
	t.Parallel()
	singulars := map[string]langcopy.Phrase{"OneThing": {En: "%d thing", De: "%d Ding", Vi: "%d thứ"}}
	s := run(func(r reporter) { NoCount(r, singulars) })
	if !reports(s.errors, "OneThing") {
		t.Fatalf("a singular sentence carrying a placeholder was not reported: %v", s.errors)
	}
}

// A well-formed table of each shape passes cleanly, so the malformed cases
// above are proved to be the malformation talking and not the harness.
func TestCensusAndNoCountPassOnWellFormedTables(t *testing.T) {
	t.Parallel()
	type nested struct {
		Direct langcopy.Phrase
		Mapped map[string]langcopy.Phrase
	}
	good := nested{
		Direct: langcopy.Phrase{En: "hi", De: "hallo", Vi: "chào"},
		Mapped: map[string]langcopy.Phrase{"apple": {En: "apple", De: "Apfel", Vi: "táo"}},
	}
	if s := run(func(r reporter) { Census(r, good) }); len(s.errors) != 0 || s.fatal != "" {
		t.Fatalf("a well-formed struct with a direct and a mapped phrase reported %v / %q", s.errors, s.fatal)
	}

	wholeTable := map[string]langcopy.Phrase{"apple": {En: "apple", De: "Apfel", Vi: "táo"}}
	if s := run(func(r reporter) { Census(r, wholeTable) }); len(s.errors) != 0 || s.fatal != "" {
		t.Fatalf("a well-formed whole-table map reported %v / %q", s.errors, s.fatal)
	}

	singulars := map[string]langcopy.Phrase{"OneThing": {En: "one thing", De: "ein Ding", Vi: "một thứ"}}
	if s := run(func(r reporter) { NoCount(r, singulars) }); len(s.errors) != 0 {
		t.Fatalf("a well-formed singular reported %v", s.errors)
	}
}
