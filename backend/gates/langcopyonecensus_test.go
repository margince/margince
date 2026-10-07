// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// One census reads every langcopy.Phrase table, and no owner walks its own.
//
// A private walk over one package's table is free to read LESS than the table
// holds: a struct field that is a map of phrases counted as one phrase, or
// skipped, certifies untranslated entries as translated. An under-reading
// census reports the same word, PASS, over a smaller subject, and no assertion
// fires to say so.
//
// langcopytest.Census is the answer, and its doc comment says it is the one
// census every langcopy.Phrase table calls. This is the test that fails when
// that stops being true.
//
// WHAT THIS GATE CANNOT SEE, stated because the claim it holds is narrower
// than the sentence a reader might take from it:
//
//   - A copy table that does NOT use langcopy.Phrase is not governed here.
//     Several remain — internal/modules/{deals,agents,capture,automation},
//     platform/mailcopy, compose/promptlang — and they are a different shape,
//     a map[textlang.Lang]T keyed language-outward rather than a struct of
//     phrases. Migrating them is real work, not a rename, and until it is done
//     each keeps its own walk. A new table written in THAT shape passes here.
//   - The private-walk arm recognises a reflect walk over textlang.Shipped. A
//     walk written some other way — a hand-listed switch over each shipped
//     language — is invisible to it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	langcopyImport     = "github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	langcopytestImport = langcopyImport + "/langcopytest"
	textlangImport     = "github.com/margince/margince/backend/internal/shared/kernel/textlang"

	// langcopyTree is the census's own home, swept past rather than judged: it
	// declares Phrase, and it is the one place a reflect walk over the shipped
	// languages belongs.
	langcopyTree = "internal/shared/kernel/langcopy"
)

// phraseTableFloor sits BELOW the number of packages that own a Phrase table
// today, so a walk that stopped reaching the tree fails here rather than
// certifying an empty corpus. Raise it only alongside evidence; it is a floor
// under a broken scan, not a count of the tree.
const phraseTableFloor = 4

// phraseOwner is one package that declares a langcopy.Phrase table: whether
// anything in it calls the shared census, and every file in it that walks the
// table itself.
type phraseOwner struct {
	certifiedBy string
	privateWalk []string
}

func TestOneCensusReadsEveryPhraseTable(t *testing.T) {
	t.Parallel()
	owners := phraseOwners(t)
	if len(owners) < phraseTableFloor {
		t.Fatalf("found %d package(s) declaring a langcopy.Phrase table, and there are at least %d — "+
			"this gate is reading a smaller tree than the one it judges, which passes exactly like a "+
			"clean one", len(owners), phraseTableFloor)
	}
	for _, dir := range slices.Sorted(maps.Keys(owners)) {
		if owners[dir].certifiedBy == "" {
			t.Errorf("%s declares a langcopy.Phrase table and no test there calls langcopytest.Census, "+
				"so nothing asserts the table is written in every shipped language. Go fills an omitted "+
				"field of a keyed literal with \"\", so an untranslated sentence goes MISSING from a card "+
				"rather than arriving in the wrong language", dir)
		}
	}
}

func TestNoPhraseTableOwnerWalksItsOwnTable(t *testing.T) {
	t.Parallel()
	owners := phraseOwners(t)
	for _, dir := range slices.Sorted(maps.Keys(owners)) {
		for _, path := range owners[dir].privateWalk {
			t.Errorf("%s reflects over textlang.Shipped, which is a second census of a table "+
				"langcopytest.Census already reads. Two censuses over one table are free to check "+
				"different subsets of it, and a private one is free to read less than the table holds. "+
				"Call langcopytest.Census(t, table) instead", path)
		}
	}
}

// TestTheCensusGateRecognisesAPrivateShippedWalk is the vacuity check.
//
// Both arms above pass by finding nothing, which is also what they do if
// References stops resolving a qualifier or the walk reads the wrong tree. So
// the predicates are shown the shape they are written for — a package's own
// reflect walk over textlang.Shipped — rather than only today's tree.
func TestTheCensusGateRecognisesAPrivateShippedWalk(t *testing.T) {
	t.Parallel()
	privateCensus := `package contactbrief

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var floor = struct{ Greeting langcopy.Phrase }{}

func TestEveryShippedLanguageWritesTheContactFloor(t *testing.T) {
	shape := reflect.TypeOf(floor)
	for _, lang := range textlang.Shipped {
		_ = shape.NumField()
		_ = lang
	}
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "copy_test.go", privateCensus, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the walk this gate was written to catch: %v", err)
	}
	if !declaresPhraseTable(file) {
		t.Error("the owner predicate no longer sees a file declaring a langcopy.Phrase table, " +
			"so this gate has no corpus and passes over every table in the tree")
	}
	if !walksShippedLanguages(file) {
		t.Error("the private-walk predicate no longer sees a reflect walk over textlang.Shipped, " +
			"which is the exact shape of the three censuses this one replaced")
	}
}

// declaresPhraseTable reports whether a file names langcopy.Phrase, which is
// what a copy table reads like after it has been migrated onto the shared type.
func declaresPhraseTable(file *ast.File) bool {
	return gatekit.References(file, langcopyImport, "Phrase")
}

// walksShippedLanguages reports whether a file reflects over the shipped
// languages — the shape every private census had, and the one thing an owner
// package has no reason to do once it calls the shared one.
func walksShippedLanguages(file *ast.File) bool {
	if !gatekit.References(file, textlangImport, "Shipped") {
		return false
	}
	return gatekit.References(file, "reflect", "TypeOf") ||
		gatekit.References(file, "reflect", "ValueOf")
}

// phraseOwners sweeps the module for packages declaring a langcopy.Phrase
// table and answers, per package, both questions the two arms ask.
//
// Keyed by DIRECTORY rather than by file, because the table and the test that
// certifies it are different files: copy.go declares the phrases and copy_test.go
// calls the census, and neither names what the other does.
func phraseOwners(t *testing.T) map[string]*phraseOwner {
	t.Helper()
	owners := map[string]*phraseOwner{}
	files := map[string][]*ast.File{}
	census := map[string]string{}
	walks := map[string][]string{}

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		path = filepath.ToSlash(path)
		if entry.IsDir() {
			if unauthoredDir(entry.Name()) || gatekit.Under(path, langcopyTree) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		files[dir] = append(files[dir], file)
		if gatekit.References(file, langcopytestImport, "Census") {
			census[dir] = path
		}
		if walksShippedLanguages(file) {
			walks[dir] = append(walks[dir], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping the module for langcopy.Phrase tables: %v", err)
	}

	for dir, parsed := range files {
		if !slices.ContainsFunc(parsed, declaresPhraseTable) {
			continue
		}
		owners[dir] = &phraseOwner{certifiedBy: census[dir], privateWalk: walks[dir]}
	}
	return owners
}
