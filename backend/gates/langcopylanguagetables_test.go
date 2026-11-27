// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every copy table keyed by language is read by a test that holds it to the style pages.
//
// A map[textlang.Lang]T table no test walks through langcopytest ships a dash
// or a "we" unseen. A test holds a table by naming it or a variable built from
// it. Unseen: a table filled by assignment, a walk of part of a table, and a
// named map type that another package declares.

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

func TestEveryLanguageKeyedCopyTableIsHeldToTheStylePages(t *testing.T) {
	t.Parallel()
	packages, checks := languagePackages(t, "."), langcopytestChecks(t)
	tables, spans := 0, map[string][][2]int{}
	for _, dir := range slices.Sorted(maps.Keys(packages)) {
		pkg := packages[dir]
		held := pkg.held(checks)
		waived := unstyledLanguageTables[dir]
		for _, name := range slices.Sorted(maps.Keys(pkg.tables)) {
			tables++
			_, isWaived := waived[name]
			switch {
			case held[name] && isWaived:
				t.Errorf("%s is named unstyled in this gate, and a test now holds it to the style pages; "+
					"delete its entry", pkg.tables[name])
			case !held[name] && !isWaived:
				t.Errorf("%s is a copy table keyed by language, and no test that calls a langcopytest style check names it. "+
					"Walk it with langcopytest.StyleByLanguage or CensusByLanguage in its register, "+
					"or, if no reader sees it, name it in unstyledLanguageTables with why", pkg.tables[name])
			}
		}
		for name := range waived {
			if _, exists := pkg.tables[name]; !exists {
				t.Errorf("unstyledLanguageTables names %s in %s, which holds no such language-keyed table; delete it", name, dir)
			}
		}
		maps.Copy(spans, pkg.spans)
	}
	for dir := range unstyledLanguageTables {
		if _, exists := packages[dir]; !exists {
			t.Errorf("unstyledLanguageTables names %s, which holds no language-keyed table; delete it", dir)
		}
	}
	literals := languageLiteralsIn(languageTexts(t, "."))
	for _, literal := range unreadLiterals(literals, spans) {
		t.Errorf("%s:%d spells a language-keyed literal outside every table the syntax scan read",
			literal.path, literal.line)
	}
	flagged := map[string]bool{}
	for _, literal := range literals {
		flagged[literal.path] = true
	}
	t.Logf("%d language-keyed tables, %d literals in %d files the text census flags", tables, len(literals), len(flagged))
	if len(flagged) == 0 {
		t.Fatal("the text census flags no file, so it is reading a tree that holds no copy table")
	}
}

// withNamedChains runs to a fixed point, so a cycle of names ends and stays unkeyed.
func withNamedChains(keyed map[string]bool, namedAs map[string]string) {
	for grew := true; grew; {
		grew = false
		for name, underlying := range namedAs {
			if keyed[underlying] && !keyed[name] {
				keyed[name], grew = true, true
			}
		}
	}
}

// TestTheLanguageTableArmSeesATableAndItsWalk is the vacuity check. Shown eight
// tables, two eliding a language map's type and one of a renamed one, the arm
// finds all eight and the five walked. A local named like a table holds none, nor
// does a helper no Test runs, nor a test that calls no style check of langcopytest.
func TestTheLanguageTableArmSeesATableAndItsWalk(t *testing.T) {
	t.Parallel()
	source := `package copy
import tl "github.com/margince/margince/backend/internal/shared/kernel/textlang"
type spellings map[tl.Lang][]string
type byKey map[string]map[tl.Lang]string
var walked = map[tl.Lang]struct{ Saved string }{tl.English: {Saved: "Saved"}}
var shown = buildShown()
func buildShown() map[string]spellings { return map[string]spellings{"x": spellings{}} }
var unwalked = struct{ text map[tl.Lang]string }{text: map[tl.Lang]string{}}
var nested = map[string]map[tl.Lang]string{"saved": {tl.English: "We saved it"}}
var orphaned = []*byKey{{"saved": {tl.English: "We saved it"}}}
type raw map[tl.Lang]string
type copyTable raw
var messages = copyTable{tl.English: "We saved it"}
type looped looping
type looping looped
var cycled = looped{}
var notices = map[tl.Lang]string{tl.English: "We saved it"}
var greetings = map[tl.Lang]string{tl.English: "Hello"}
`
	walk := `package copy
import "github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
func check(t langcopytest.T) { langcopytest.EnglishText(t, "x", "y") }
func TestWalked(t *testing.T) { check(t); _ = walked; _ = shown; t.Run("x", checkNested) }
func checkNested(t *testing.T) { langcopytest.EnglishText(t, "x", nested["saved"]["en"]) }
func TestUnwalked(t *testing.T) { _ = langcopytest.Sentences(t, unwalked); var _ langcopytest.Register }
func TestMessages(t *testing.T) { langcopytest.Outsider("x").VietnameseText(t, "x", messages["vi"]) }
func TestShadowed(t *testing.T) { check(t); unwalked := struct{ unwalked int }{unwalked: 1}; _ = unwalked.unwalked }
func unusedCheck(t langcopytest.T) { langcopytest.EnglishText(t, "x", orphaned[0]) }
func TestLocal(t *testing.T) { unusedCheck := func() {}; unusedCheck(); _ = orphaned }
type otherChecker struct{}
func (otherChecker) EnglishText() {}
func TestNotices(t *testing.T) {
	var _ langcopytest.Register
	otherChecker{}.EnglishText()
	_ = notices
}
func reader() (langcopytest.Register, string) { return langcopytest.Agent("x"), "" }
func TestGreetings(t *testing.T) { register, _ := reader(); register.EnglishText(t, "x", greetings["en"]) }
`
	pkg := parsedLanguagePackage(t, map[string]string{"copy.go": source, "copy_test.go": walk})
	want := []string{"buildShown", "greetings", "messages", "nested", "notices", "orphaned", "unwalked", "walked"}
	if got := slices.Sorted(maps.Keys(pkg.tables)); !slices.Equal(got, want) {
		t.Errorf("the arm read the tables %v, want %v", got, want)
	}
	held := pkg.held(langcopytestChecks(t))
	if !held["walked"] || !held["buildShown"] || !held["nested"] || !held["messages"] || !held["greetings"] ||
		held["unwalked"] || held["orphaned"] || held["notices"] {
		t.Errorf("the arm read the walked tables as %v", held)
	}
	chained := []byte("type raw map[textlang.Lang]string\ntype copyTable raw\ntype looped looping\ntype looping looped\n")
	if got := spellingIn([]languageText{lexLanguageText(chained)}).mapTypes; !slices.Equal(got, []string{"copyTable", "raw"}) {
		t.Errorf("the text census reads the language map types %v, want [copyTable raw]", got)
	}
	literal := spellingIn([]languageText{lexLanguageText([]byte("type byKey map[string]map[textlang.Lang]string\n"))}).literal()
	for _, line := range []string{"var nested = map[string]map[textlang.Lang]string{", "return []*byKey{"} {
		if !literal.MatchString(line) {
			t.Errorf("the text census does not read %q as a language-keyed literal", line)
		}
	}
}

// TestTheLanguageTableArmReadsNamesAcrossFiles is the vacuity check for a name
// one file declares and another spells. One file holds three tables: keyed
// directly, keyed by an alias, and of a map type declared under another import
// name of textlang. The text census reads each literal apart. A test file that
// does not import langcopytest walks the alias table through a helper's Register.
func TestTheLanguageTableArmReadsNamesAcrossFiles(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"keys.go": `package copy
import tl "github.com/margince/margince/backend/internal/shared/kernel/textlang"
type locale = tl.Lang
type byKey map[string]map[tl.Lang]string
`,
		"tables.go": `package copy
import "github.com/margince/margince/backend/internal/shared/kernel/textlang"
var direct = map[textlang.Lang]string{textlang.English: "Saved"}
var aliased = map[locale]string{textlang.English: "We saved it"}
var elided = byKey{"saved": {textlang.English: "We saved it"}}
`,
		"reader_test.go": `package copy
import "github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
func reader() (langcopytest.Register, string) { return langcopytest.Agent("x"), "" }
`,
		"copy_test.go": `package copy
func TestAliased(t *testing.T) { register, _ := reader(); register.EnglishText(t, "x", aliased["en"]) }
`,
	}
	pkg := parsedLanguagePackage(t, files)
	if got, want := slices.Sorted(maps.Keys(pkg.tables)), []string{"aliased", "direct", "elided"}; !slices.Equal(got, want) {
		t.Errorf("the arm read the tables %v, want %v", got, want)
	}
	if held := pkg.held(langcopytestChecks(t)); !held["aliased"] || held["direct"] || held["elided"] {
		t.Errorf("the arm read the walked tables as %v, want aliased alone", held)
	}
	literals := languageLiteralsIn(sourceTexts(files))
	if got := len(literals); got != 3 {
		t.Errorf("the text census reads %d language-keyed literals, want 3: %v", got, literals)
	}
	if unread := unreadLiterals(literals, pkg.spans); len(unread) != 0 {
		t.Errorf("the syntax scan holds no table at the literals %v", unread)
	}
	pkg.spans["copy/tables.go"] = pkg.spans["copy/tables.go"][:1]
	if unread := unreadLiterals(literals, pkg.spans); len(unread) != 2 {
		t.Errorf("with only the first table of tables.go read, the census reports %v, want the two after it", unread)
	}
}

// TestBothCensusesReadEachSpellingOfATable holds the two arms to one answer for
// spellings one of them once misread. A declaration or a comment is no literal.
// A type may be declared in a group or with type parameters, and a local type
// may share a package type's name.
func TestBothCensusesReadEachSpellingOfATable(t *testing.T) {
	t.Parallel()
	const header = "package copy\nimport \"" + textlangImport + "\"\n"
	for _, tc := range []struct{ name, source, table string }{
		{"a map alias", "type T = map[textlang.Lang]string\nvar x = T{textlang.English: \"a\"}\n", "x"},
		{"a commented literal", "// y = map[textlang.Lang]string{}\n/* z = map[textlang.Lang]string{} */\n" +
			"var x = map[textlang.Lang]string{textlang.English: \"a\"}\n", "x"},
		{"a grouped key alias", "type (\n\tlocale = textlang.Lang\n)\nvar x = map[locale]string{textlang.English: \"a\"}\n", "x"},
		{"a grouped map type", "type (\n\tid int\n\traw map[textlang.Lang]string\n)\nvar x = raw{textlang.English: \"a\"}\n", "x"},
		{"a generic map type", "type Table[T any] map[textlang.Lang]T\nvar x = Table[string]{textlang.English: \"a\"}\n", "x"},
		{"a type switch", "func f(v any) any {\n\tswitch v.(type) {\n\tdefault:\n\t\treturn map[textlang.Lang]string{}\n\t}\n}\n", "f"},
		{"a local type named like a key alias", "type locale = textlang.Lang\n" +
			"func f() any { type locale string; return map[locale]string{} }\n", "f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"copy.go": header + tc.source}
			pkg := parsedLanguagePackage(t, files)
			if got := slices.Sorted(maps.Keys(pkg.tables)); !slices.Equal(got, []string{tc.table}) {
				t.Errorf("the syntax scan read the tables %v, want [%s]", got, tc.table)
			}
			literals := languageLiteralsIn(sourceTexts(files))
			if unread := unreadLiterals(literals, pkg.spans); len(literals) != 1 || len(unread) != 0 {
				t.Errorf("the text census read the literals %v, of which %v lie in no table; want one, in %s",
					literals, unread, tc.table)
			}
		})
	}
}

// sourceTexts keys each non-test file as the census reads it.
func sourceTexts(files map[string]string) map[string][]byte {
	texts := map[string][]byte{}
	for name, text := range files {
		if !strings.HasSuffix(name, "_test.go") {
			texts["copy/"+name] = []byte(text)
		}
	}
	return texts
}

func parsedLanguagePackage(t *testing.T, files map[string]string) *languagePackage {
	t.Helper()
	pkg := newLanguagePackage()
	for name, text := range files {
		file, err := parser.ParseFile(token.NewFileSet(), name, text, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		pkg.add("copy/"+name, file)
	}
	pkg.resolve()
	return pkg
}

// languagePackage is one directory's language-keyed tables and the tests that
// may walk them.
type languagePackage struct {
	files, tests []*ast.File
	paths        []string
	// tables maps a top-level name to where its language-keyed literal stands,
	// and spans each file to the byte ranges of those declarations.
	tables             map[string]string
	spans              map[string][][2]int
	langKeys, mapTypes map[string]bool
	typeSpecs          map[string]qualifiedType
}

// qualifiedType is a type expression with the name its own file imports textlang
// under, which a file spelling the type's name may import under another.
type qualifiedType struct {
	expr ast.Expr
	lang string
}

func newLanguagePackage() *languagePackage {
	return &languagePackage{tables: map[string]string{}, spans: map[string][][2]int{}}
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
		dir := filepath.ToSlash(filepath.Dir(path))
		if packages[dir] == nil {
			packages[dir] = newLanguagePackage()
		}
		packages[dir].add(path, file)
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping the module for language-keyed tables: %v", err)
	}
	for dir, pkg := range packages {
		pkg.resolve()
		if len(pkg.tables) == 0 {
			delete(packages, dir)
		}
	}
	return packages
}

func (p *languagePackage) add(path string, file *ast.File) {
	if strings.HasSuffix(path, "_test.go") {
		p.tests = append(p.tests, file)
		return
	}
	p.files, p.paths = append(p.files, file), append(p.paths, path)
}

// resolve finds the tables once every file of the directory is in, since a
// named type and its literals may sit in different files.
func (p *languagePackage) resolve() {
	p.langKeys, p.mapTypes, p.typeSpecs = map[string]bool{}, map[string]bool{}, map[string]qualifiedType{}
	namedAs := map[string]string{}
	for _, file := range p.files {
		lang := textlangQualifier(file)
		for _, spec := range topLevelTypeSpecs(file) {
			p.typeSpecs[spec.Name.Name] = qualifiedType{spec.Type, lang}
			if underlying, named := ast.Unparen(spec.Type).(*ast.Ident); named {
				namedAs[spec.Name.Name] = underlying.Name
			}
		}
	}
	for name, spec := range p.typeSpecs {
		p.langKeys[name] = isTextlangLang(spec)
	}
	withNamedChains(p.langKeys, namedAs)
	for name, spec := range p.typeSpecs {
		p.mapTypes[name] = p.keyedMap(spec)
	}
	withNamedChains(p.mapTypes, namedAs)
	for i, file := range p.files {
		lang := textlangQualifier(file)
		for _, decl := range file.Decls {
			for name, part := range declParts(decl) {
				if !p.holdsLanguageLiteral(part, lang) {
					continue
				}
				if _, seen := p.tables[name]; !seen {
					p.tables[name] = p.paths[i] + ": " + name
				}
				span := [2]int{int(part.Pos() - file.FileStart), int(part.End() - file.FileStart)}
				p.spans[p.paths[i]] = append(p.spans[p.paths[i]], span)
			}
		}
	}
}

// topLevelTypeSpecs leaves out a type a function declares, which would
// otherwise stand for a package type of its name in every file.
func topLevelTypeSpecs(file *ast.File) []*ast.TypeSpec {
	var specs []*ast.TypeSpec
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				if typeSpec, isType := spec.(*ast.TypeSpec); isType {
					specs = append(specs, typeSpec)
				}
			}
		}
	}
	return specs
}

// holdsLanguageLiteral reports whether part spells a language-keyed literal,
// its type written out or implied by the literal around it.
func (p *languagePackage) holdsLanguageLiteral(part ast.Node, lang string) bool {
	found, implied := false, map[*ast.CompositeLit]qualifiedType{}
	ast.Inspect(part, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || found {
			return !found
		}
		literalType := qualifiedType{literal.Type, lang}
		if literal.Type == nil {
			literalType = implied[literal]
		}
		found = p.languageKeyed(literalType)
		p.imply(literal, literalType, implied)
		return !found
	})
	return found
}

// imply gives each element literal that elides its type the element type of
// the literal around it. So {lang: …} inside a map of language maps is read.
func (p *languagePackage) imply(literal *ast.CompositeLit, literalType qualifiedType, implied map[*ast.CompositeLit]qualifiedType) {
	keyType, elementType := p.elementTypes(literalType)
	elide := func(element ast.Expr, elementType qualifiedType) {
		if pointer, ok := elementType.expr.(*ast.StarExpr); ok {
			elementType.expr = pointer.X
		}
		if inner, ok := element.(*ast.CompositeLit); ok && inner.Type == nil && elementType.expr != nil {
			implied[inner] = elementType
		}
	}
	for _, element := range literal.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			elide(pair.Key, keyType)
			element = pair.Value
		}
		elide(element, elementType)
	}
}

// elementTypes answers the key and element types of a map, slice or array
// type, through the package's named types. A struct elides nothing.
func (p *languagePackage) elementTypes(container qualifiedType) (keyType, elementType qualifiedType) {
	for range len(p.typeSpecs) + 1 {
		switch t := container.expr.(type) {
		case *ast.MapType:
			return qualifiedType{t.Key, container.lang}, qualifiedType{t.Value, container.lang}
		case *ast.ArrayType:
			return qualifiedType{}, qualifiedType{t.Elt, container.lang}
		case *ast.ParenExpr:
			container.expr = t.X
		case *ast.IndexExpr:
			container.expr = t.X
		case *ast.IndexListExpr:
			container.expr = t.X
		case *ast.Ident:
			container = p.typeSpecs[t.Name]
		default:
			return qualifiedType{}, qualifiedType{}
		}
	}
	return qualifiedType{}, qualifiedType{}
}

// held answers the tables a test that reaches langcopytest names, directly or
// through a variable whose value is built from them.
func (p *languagePackage) held(checks styleChecks) map[string]bool {
	built, funcs := map[string][]string{}, map[string]bool{}
	for _, file := range p.files {
		for _, decl := range file.Decls {
			for name, part := range declParts(decl) {
				_, funcs[name] = part.(*ast.FuncDecl)
				built[name] = append(built[name], packageNames(part)...)
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
	for _, name := range namedByStyleTests(p.tests, checks) {
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

// packageNames answers the names node spells that can mean a top-level
// declaration: not a field it selects or keys, nor a name it declares itself.
func packageNames(node ast.Node) []string {
	local, field := declaredIn(node), map[*ast.Ident]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			field[n.Sel] = true
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok {
				field[key] = true
			}
		}
		return true
	})
	var names []string
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && !field[ident] && !local[ident.Name] {
			names = append(names, ident.Name)
		}
		return true
	})
	return names
}

// declaredIn answers the names node declares for itself. A name declared
// anywhere in node counts throughout, even outside its scope.
func declaredIn(node ast.Node) map[string]bool {
	local := map[string]bool{}
	declare := func(exprs ...ast.Expr) {
		for _, expr := range exprs {
			if ident, ok := expr.(*ast.Ident); ok {
				local[ident.Name] = true
			}
		}
	}
	parameters := func(lists ...*ast.FieldList) {
		for _, list := range lists {
			if list == nil {
				continue
			}
			for _, parameter := range list.List {
				for _, name := range parameter.Names {
					declare(name)
				}
			}
		}
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			parameters(n.Recv)
		case *ast.FuncType:
			parameters(n.TypeParams, n.Params, n.Results)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				declare(n.Lhs...)
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				declare(n.Key, n.Value)
			}
		case *ast.DeclStmt:
			ast.Inspect(n, func(inner ast.Node) bool {
				switch spec := inner.(type) {
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						declare(name)
					}
				case *ast.TypeSpec:
					declare(spec.Name)
				}
				return true
			})
		}
		return true
	})
	return local
}

// languageKeyed reports whether a literal's type is a map keyed by
// textlang.Lang, or a map type of the package declared so.
func (p *languagePackage) languageKeyed(literalType qualifiedType) bool {
	named := ast.Unparen(literalType.expr)
	switch instance := named.(type) {
	case *ast.IndexExpr:
		named = instance.X
	case *ast.IndexListExpr:
		named = instance.X
	}
	if name, isName := named.(*ast.Ident); isName {
		return p.mapTypes[name.Name]
	}
	return p.keyedMap(literalType)
}

// keyedMap reports whether a type is a map keyed by textlang.Lang. The key is
// spelled with its file's qualifier or by a name the package declares for it.
func (p *languagePackage) keyedMap(typ qualifiedType) bool {
	m, ok := ast.Unparen(typ.expr).(*ast.MapType)
	if !ok {
		return false
	}
	if key, named := ast.Unparen(m.Key).(*ast.Ident); named {
		return p.langKeys[key.Name]
	}
	return isTextlangLang(qualifiedType{m.Key, typ.lang})
}

func isTextlangLang(typ qualifiedType) bool {
	key, ok := ast.Unparen(typ.expr).(*ast.SelectorExpr)
	if !ok || key.Sel.Name != "Lang" {
		return false
	}
	x, ok := key.X.(*ast.Ident)
	return ok && typ.lang != "" && x.Name == typ.lang
}

func textlangQualifier(file *ast.File) string {
	qualifier, _ := gatekit.ImportedAs(file, textlangImport)
	return qualifier
}

// namedByStyleTests answers every name spelled by a test function that a Test
// runs and that calls a langcopytest style check. It follows calls to the
// directory's own test helpers. A helper no Test runs checks nothing.
func namedByStyleTests(tests []*ast.File, checks styleChecks) []string {
	reaches, calls, names := map[string]bool{}, map[string][]string{}, map[string][]string{}
	spelled, values := map[string][]string{}, langcopytestValuesOf(tests)
	for _, file := range tests {
		qualifier, _ := gatekit.ImportedAs(file, langcopytestImport)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			names[fn.Name.Name] = append(names[fn.Name.Name], packageNames(fn)...)
			local := declaredIn(fn)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.Ident:
					if !local[n.Name] {
						spelled[fn.Name.Name] = append(spelled[fn.Name.Name], n.Name)
					}
				case *ast.CallExpr:
					if callee, isIdent := n.Fun.(*ast.Ident); isIdent && !local[callee.Name] {
						calls[fn.Name.Name] = append(calls[fn.Name.Name], callee.Name)
					}
					if checks.calledBy(n, qualifier, values) {
						reaches[fn.Name.Name] = true
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
	for caller := range runByTests(spelled) {
		if reaches[caller] {
			named = append(named, names[caller]...)
		}
	}
	return named
}

// runByTests answers the test functions a Test runs: each Test and every test
// function it spells. It may call one, pass one on or call it as a method.
func runByTests(spelled map[string][]string) map[string]bool {
	run := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if _, isFunc := spelled[name]; !isFunc || run[name] {
			return
		}
		run[name] = true
		for _, next := range spelled[name] {
			visit(next)
		}
	}
	for name := range spelled {
		if strings.HasPrefix(name, "Test") {
			visit(name)
		}
	}
	return run
}

// styleChecks are read from langcopytest, so a new check counts with no edit here.
type styleChecks struct{ funcs, methods map[string]bool }

// langcopytestChecks reads a check as an exported function or method that takes
// the reporter and returns nothing. Sentences returns what it read, so it is none.
func langcopytestChecks(t *testing.T) styleChecks {
	t.Helper()
	checks := styleChecks{funcs: map[string]bool{}, methods: map[string]bool{}}
	paths, err := gatekit.Glob(langcopyTree + "/langcopytest/*.go")
	if err != nil {
		t.Fatalf("listing langcopytest for its style checks: %v", err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("reading langcopytest for its style checks: %v", err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Type.Results != nil || len(fn.Type.Params.List) == 0 {
				continue
			}
			if first, named := fn.Type.Params.List[0].Type.(*ast.Ident); !named || first.Name != "reporter" {
				continue
			}
			if fn.Recv == nil {
				checks.funcs[fn.Name.Name] = true
			} else {
				checks.methods[fn.Name.Name] = true
			}
		}
	}
	if !checks.funcs["EnglishText"] || !checks.methods["VietnameseText"] {
		t.Fatalf("langcopytest declares no EnglishText function or VietnameseText method, so this gate reads no style check")
	}
	return checks
}

// calledBy reports whether call runs a style check: a langcopytest function, or
// a check method of a value langcopytest gave, such as a Register. A file that
// does not import langcopytest may still call a method on a value it was given.
func (c styleChecks) calledBy(call *ast.CallExpr, qualifier string, values langcopytestValues) bool {
	selector, ok := uninstantiated(call).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if x, isIdent := selector.X.(*ast.Ident); isIdent && x.Name == qualifier {
		return c.funcs[selector.Sel.Name]
	}
	return c.methods[selector.Sel.Name] && values.holds(selector.X, qualifier)
}

func uninstantiated(call *ast.CallExpr) ast.Expr {
	switch generic := ast.Unparen(call.Fun).(type) {
	case *ast.IndexExpr:
		return generic.X
	case *ast.IndexListExpr:
		return generic.X
	}
	return ast.Unparen(call.Fun)
}

// langcopytestValues are the names in a directory's tests that may hold a
// langcopytest value, and per test function which of its results is one.
type langcopytestValues struct {
	names   map[string]bool
	returns map[string][]bool
}

// langcopytestValuesOf runs to a fixed point, since a name may be assigned from
// one declared later in the file or in another file.
func langcopytestValuesOf(tests []*ast.File) langcopytestValues {
	values := langcopytestValues{names: map[string]bool{}, returns: map[string][]bool{}}
	for grew := true; grew; {
		before := len(values.names) + len(values.returns)
		for _, file := range tests {
			qualifier, _ := gatekit.ImportedAs(file, langcopytestImport)
			ast.Inspect(file, func(node ast.Node) bool {
				values.read(node, qualifier)
				return true
			})
		}
		grew = len(values.names)+len(values.returns) > before
	}
	return values
}

func (v langcopytestValues) read(node ast.Node, qualifier string) {
	switch n := node.(type) {
	case *ast.FuncDecl:
		if n.Type.Results != nil {
			var results []bool
			for _, field := range n.Type.Results.List {
				for range max(len(field.Names), 1) {
					results = append(results, typedBy(field.Type, qualifier))
				}
			}
			v.returns[n.Name.Name] = results
		}
	case *ast.Field:
		if typedBy(n.Type, qualifier) {
			v.mark(n.Names...)
		}
	case *ast.ValueSpec:
		names := make([]ast.Expr, len(n.Names))
		for i, name := range n.Names {
			names[i] = name
		}
		if n.Type != nil && typedBy(n.Type, qualifier) {
			v.mark(n.Names...)
		}
		v.assign(names, n.Values, qualifier)
	case *ast.AssignStmt:
		v.assign(n.Lhs, n.Rhs, qualifier)
	}
}

func (v langcopytestValues) assign(lhs, rhs []ast.Expr, qualifier string) {
	results := make([]bool, len(rhs))
	for i, value := range rhs {
		results[i] = v.holds(value, qualifier)
	}
	if call, isCall := singleCall(rhs); isCall && len(lhs) > 1 {
		if callee, isIdent := call.Fun.(*ast.Ident); isIdent {
			results = v.returns[callee.Name]
		}
	}
	for i, target := range lhs {
		if ident, isIdent := target.(*ast.Ident); isIdent && i < len(results) && results[i] {
			v.mark(ident)
		}
	}
}

func (v langcopytestValues) mark(names ...*ast.Ident) {
	for _, name := range names {
		if name.Name != "_" {
			v.names[name.Name] = true
		}
	}
}

// holds reports whether expr is a langcopytest value. A call into langcopytest
// gives one, as does a name or field typed or assigned one.
func (v langcopytestValues) holds(expr ast.Expr, qualifier string) bool {
	switch e := ast.Unparen(expr).(type) {
	case *ast.CallExpr:
		switch callee := uninstantiated(e).(type) {
		case *ast.SelectorExpr:
			x, isIdent := callee.X.(*ast.Ident)
			return isIdent && qualifier != "" && x.Name == qualifier
		case *ast.Ident:
			results := v.returns[callee.Name]
			return len(results) > 0 && results[0]
		}
	case *ast.Ident:
		return v.names[e.Name]
	case *ast.SelectorExpr:
		return v.names[e.Sel.Name]
	case *ast.IndexExpr:
		return v.holds(e.X, qualifier)
	case *ast.CompositeLit:
		return typedBy(e.Type, qualifier)
	}
	return false
}

// typedBy reports whether a type expression names a langcopytest type anywhere,
// as in langcopytest.Register or []langcopytest.Register.
func typedBy(typ ast.Expr, qualifier string) bool {
	found := false
	ast.Inspect(typ, func(node ast.Node) bool {
		if selector, isSelector := node.(*ast.SelectorExpr); isSelector {
			x, isIdent := selector.X.(*ast.Ident)
			found = found || (isIdent && qualifier != "" && x.Name == qualifier)
		}
		return !found
	})
	return found
}
