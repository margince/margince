// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every copy table keyed by language is read by a test that holds it to the style pages.
//
// A map[textlang.Lang]T table no test walks through langcopytest ships a dash
// or a "we" unseen. A test holds a table by naming it or a variable built from
// it. Unseen: a table filled by assignment, and a walk of part of a table.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// unstyledLanguageTables are the language-keyed tables that are not copy a
// reader sees, by directory and table, each with why.
var unstyledLanguageTables = map[string]map[string]string{
	"internal/compose/draftcheck": {
		"assumedMemory": draftMatcher, "wellbeing": draftMatcher, "invention": draftMatcher,
		"resolvedEvent": draftMatcher, "namingLeads": draftMatcher, "encounterFrame": draftMatcher,
		"metEncounter": draftMatcher, "spokenExchange": draftMatcher, "attributedClaim": draftMatcher,
		"scheduledArrangement": draftMatcher, "directedIntroduction": draftMatcher,
		"introductionEvent": draftMatcher, "firstTouchSubjects": draftMatcher,
	},
	"internal/compose/claims": {
		"writerSubjects": "the pronouns a claim's writer uses for themselves, matched in their mail and never shown",
	},
	"internal/platform/salutation": {
		"greetingOpeners": "the openers a greeting is recognised by in a member's mail, never shown",
	},
	"internal/shared/kernel/datenotation": {
		"LocalePatterns": "date layouts a parser reads and a formatter fills, not sentences",
	},
	"internal/shared/kernel/draftfloor": {
		"table": memberDraft, "groupGreetings": memberDraft, "formalGreetings": memberDraft, "substance": memberDraft,
	},
	"internal/compose": {
		"legacySignalSummaryByLang": "the wording older stored rows carry, matched to reword them and never shown",
	},
	"internal/compose/company360": {"introTable": memberDraft},
	"internal/compose/network":    {"noteTable": memberDraft},
	"internal/modules/signals":    {"introTable": memberDraft},
}

const (
	draftMatcher = "phrases a draft is matched against, never shown"
	memberDraft  = "a draft the member sends as themselves, which no style page governs"
)

// languageTableFloor sits below the tables the tree holds today, so a sweep
// that stopped reading the tree fails here instead of passing over nothing.
const languageTableFloor = 45

// languageLiteral is what a text scan reads as a language-keyed literal. It
// holds the syntax arm to every file a plain grep would flag.
var languageLiteral = regexp.MustCompile(`(?:[=:]|return)\s*map\[textlang\.Lang\]`)

func TestEveryLanguageKeyedCopyTableIsHeldToTheStylePages(t *testing.T) {
	t.Parallel()
	packages := languagePackages(t, ".")
	tables, flagged := 0, 0
	for _, dir := range slices.Sorted(maps.Keys(packages)) {
		pkg := packages[dir]
		held := pkg.held()
		waived := unstyledLanguageTables[dir]
		for _, name := range slices.Sorted(maps.Keys(pkg.tables)) {
			tables++
			_, isWaived := waived[name]
			switch {
			case held[name] && isWaived:
				t.Errorf("%s is named unstyled in this gate, and a test now holds it to the style pages; "+
					"delete its entry", pkg.tables[name])
			case !held[name] && !isWaived:
				t.Errorf("%s is a copy table keyed by language, and no test that calls langcopytest names it. "+
					"Walk it with langcopytest.CensusByLanguage, EnglishText and VietnameseText in its "+
					"register, or, if no reader sees it, name it in unstyledLanguageTables with why", pkg.tables[name])
			}
		}
		for name := range waived {
			if _, exists := pkg.tables[name]; !exists {
				t.Errorf("unstyledLanguageTables names %s in %s, which holds no such language-keyed table; delete it", name, dir)
			}
		}
		for _, path := range pkg.grepped {
			flagged++
			if !pkg.literalIn[path] {
				t.Errorf("%s spells a language-keyed literal that the syntax scan did not read as a table", path)
			}
		}
	}
	for dir := range unstyledLanguageTables {
		if _, exists := packages[dir]; !exists {
			t.Errorf("unstyledLanguageTables names %s, which holds no language-keyed table; delete it", dir)
		}
	}
	t.Logf("%d language-keyed tables, %d files a grep flags", tables, flagged)
	if tables < languageTableFloor || flagged == 0 {
		t.Fatalf("found %d language-keyed tables in %d grepped files, and the tree holds at least %d; the sweep "+
			"is reading a smaller tree than the one it judges", tables, flagged, languageTableFloor)
	}
}

// TestTheLanguageTableArmSeesATableAndItsWalk is the vacuity check: shown three
// tables, one of a named map type, the arm finds all three and the two walked.
func TestTheLanguageTableArmSeesATableAndItsWalk(t *testing.T) {
	t.Parallel()
	source := `package copy
import tl "github.com/margince/margince/backend/internal/shared/kernel/textlang"
type spellings map[tl.Lang][]string
var walked = map[tl.Lang]struct{ Saved string }{tl.English: {Saved: "Saved"}}
var shown = buildShown()
func buildShown() map[string]spellings { return map[string]spellings{"x": spellings{}} }
var unwalked = struct{ text map[tl.Lang]string }{text: map[tl.Lang]string{}}
`
	walk := `package copy
import "github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
func check(t langcopytest.T) { langcopytest.EnglishText(t, "x", "y") }
func TestWalked(t *testing.T) { check(t); _ = walked; _ = shown }
func TestUnwalked(t *testing.T) { _ = unwalked }
`
	pkg := &languagePackage{tables: map[string]string{}, literalIn: map[string]bool{}}
	for name, text := range map[string]string{"copy.go": source, "copy_test.go": walk} {
		file, err := parser.ParseFile(token.NewFileSet(), name, text, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		pkg.add(name, file, []byte(text))
	}
	pkg.resolve()
	if got := slices.Sorted(maps.Keys(pkg.tables)); !slices.Equal(got, []string{"buildShown", "unwalked", "walked"}) {
		t.Errorf("the arm read the tables %v", got)
	}
	if held := pkg.held(); !held["walked"] || !held["buildShown"] || held["unwalked"] {
		t.Errorf("the arm read the walked tables as %v", held)
	}
}

// languagePackage is one directory's language-keyed tables and the tests that
// may walk them.
type languagePackage struct {
	files, tests []*ast.File
	paths        []string
	// tables maps a top-level name to where its language-keyed literal stands.
	tables    map[string]string
	literalIn map[string]bool
	grepped   []string
	mapTypes  map[string]bool
}

func languagePackages(t *testing.T, root string) map[string]*languagePackage {
	t.Helper()
	packages := map[string]*languagePackage{}
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
		text, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if packages[dir] == nil {
			packages[dir] = &languagePackage{tables: map[string]string{}, literalIn: map[string]bool{}}
		}
		packages[dir].add(path, file, text)
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping the module for language-keyed tables: %v", err)
	}
	for dir, pkg := range packages {
		pkg.resolve()
		if len(pkg.tables) == 0 && len(pkg.grepped) == 0 {
			delete(packages, dir)
		}
	}
	return packages
}

func (p *languagePackage) add(path string, file *ast.File, text []byte) {
	if strings.HasSuffix(path, "_test.go") {
		p.tests = append(p.tests, file)
		return
	}
	p.files, p.paths = append(p.files, file), append(p.paths, path)
	if languageLiteral.Match(text) {
		p.grepped = append(p.grepped, path)
	}
}

// resolve finds the tables once every file of the directory is in, since a
// named map type and its literals may sit in different files.
func (p *languagePackage) resolve() {
	p.mapTypes = map[string]bool{}
	for _, file := range p.files {
		lang := textlangQualifier(file)
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.TypeSpec); ok && languageKeyed(spec.Type, lang, nil) {
				p.mapTypes[spec.Name.Name] = true
			}
			return true
		})
	}
	for i, file := range p.files {
		lang := textlangQualifier(file)
		for _, decl := range file.Decls {
			for name, part := range declParts(decl) {
				ast.Inspect(part, func(node ast.Node) bool {
					literal, ok := node.(*ast.CompositeLit)
					if !ok || !languageKeyed(literal.Type, lang, p.mapTypes) {
						return true
					}
					if _, seen := p.tables[name]; !seen {
						p.tables[name] = p.paths[i] + ": " + name
					}
					p.literalIn[p.paths[i]] = true
					return false
				})
			}
		}
	}
}

// held answers the tables a test that reaches langcopytest names, directly or
// through a variable whose value is built from them.
func (p *languagePackage) held() map[string]bool {
	built, funcs := map[string][]string{}, map[string]bool{}
	for _, file := range p.files {
		for _, decl := range file.Decls {
			for name, part := range declParts(decl) {
				_, funcs[name] = part.(*ast.FuncDecl)
				built[name] = append(built[name], spelledNames(part)...)
			}
		}
	}
	held, seen := map[string]bool{}, map[string]bool{}
	var follow func(name string, fromValue bool)
	follow = func(name string, fromValue bool) {
		if _, table := p.tables[name]; table {
			held[name] = true
		}
		// A function a variable's value calls builds that value; one a test or
		// another function calls is not walked by it.
		if seen[name] || (funcs[name] && !fromValue) {
			return
		}
		seen[name] = true
		for _, next := range built[name] {
			follow(next, !funcs[name])
		}
	}
	for _, name := range namedByStyleTests(p.tests) {
		follow(name, false)
	}
	return held
}

// declParts names each top-level function and variable of a declaration with
// the syntax that builds it.
func declParts(decl ast.Decl) map[string]ast.Node {
	parts := map[string]ast.Node{}
	switch d := decl.(type) {
	case *ast.FuncDecl:
		parts[d.Name.Name] = d
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok {
				for _, name := range value.Names {
					parts[name.Name] = value
				}
			}
		}
	}
	return parts
}

func spelledNames(node ast.Node) []string {
	var names []string
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			names = append(names, ident.Name)
		}
		return true
	})
	return names
}

// languageKeyed reports whether a literal's type is a map keyed by
// textlang.Lang, or a map type of the package declared so.
func languageKeyed(expr ast.Expr, qualifier string, mapTypes map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.MapType:
		key, ok := e.Key.(*ast.SelectorExpr)
		if !ok || key.Sel.Name != "Lang" {
			return false
		}
		x, ok := key.X.(*ast.Ident)
		return ok && qualifier != "" && x.Name == qualifier
	case *ast.Ident:
		return mapTypes[e.Name]
	}
	return false
}

func textlangQualifier(file *ast.File) string {
	qualifier, _ := gatekit.ImportedAs(file, textlangImport)
	return qualifier
}

// namedByStyleTests answers every name the test functions that reach
// langcopytest spell, following calls to the directory's own test helpers.
func namedByStyleTests(tests []*ast.File) []string {
	reaches, calls, names := map[string]bool{}, map[string][]string{}, map[string][]string{}
	for _, file := range tests {
		qualifier, _ := gatekit.ImportedAs(file, langcopytestImport)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			names[fn.Name.Name] = append(names[fn.Name.Name], spelledNames(fn.Body)...)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.SelectorExpr:
					if x, isIdent := n.X.(*ast.Ident); isIdent && qualifier != "" && x.Name == qualifier {
						reaches[fn.Name.Name] = true
					}
				case *ast.CallExpr:
					if callee, isIdent := n.Fun.(*ast.Ident); isIdent {
						calls[fn.Name.Name] = append(calls[fn.Name.Name], callee.Name)
					}
				}
				return true
			})
		}
	}
	for grew := true; grew; {
		grew = false
		for caller, callees := range calls {
			if !reaches[caller] && slices.ContainsFunc(callees, func(name string) bool { return reaches[name] }) {
				reaches[caller], grew = true, true
			}
		}
	}
	var named []string
	for caller := range reaches {
		named = append(named, names[caller]...)
	}
	return named
}
