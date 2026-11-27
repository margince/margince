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

// containerPrefix spells the map, slice, array and pointer types a language
// map may sit in, whose literals elide its type.
const containerPrefix = `(?:map\[[^\[\]]*\]|\[[^\[\]]*\]|\*)*`

var (
	languageMapType = regexp.MustCompile(`type\s+(\w+)\s+` + containerPrefix + `map\[textlang\.Lang\]`)
	namedType       = regexp.MustCompile(`(?m)type\s+(\w+)\s+(?:=\s*)?(\w+)[ \t]*(?:$|;|//)`)
)

// languageLiteral matches a literal of a language map, of a named type that
// holds one, or of a container whose elements are either.
func languageLiteral(mapTypes []string) *regexp.Regexp {
	spelled := []string{`map\[textlang\.Lang\]`}
	for _, name := range mapTypes {
		spelled = append(spelled, name+`\{`)
	}
	return regexp.MustCompile(`(?:[=:]|return)\s*` + containerPrefix + `(?:` + strings.Join(spelled, "|") + `)`)
}

func TestEveryLanguageKeyedCopyTableIsHeldToTheStylePages(t *testing.T) {
	t.Parallel()
	packages, checks := languagePackages(t, "."), langcopytestChecks(t)
	tables, read := 0, map[string]bool{}
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
		maps.Copy(read, pkg.literalIn)
	}
	for dir := range unstyledLanguageTables {
		if _, exists := packages[dir]; !exists {
			t.Errorf("unstyledLanguageTables names %s, which holds no language-keyed table; delete it", dir)
		}
	}
	flagged := languageLiteralFiles(t, ".")
	for _, path := range flagged {
		if !read[path] {
			t.Errorf("%s spells a language-keyed literal that the syntax scan did not read as a table", path)
		}
	}
	t.Logf("%d language-keyed tables, %d files the text census flags", tables, len(flagged))
	if len(flagged) == 0 {
		t.Fatal("the text census flags no file, so it is reading a tree that holds no copy table")
	}
}

// languageLiteralFiles is the census the syntax arm answers to: each non-test
// Go file under root that spells a language-keyed literal. It reads plain text
// and skips no directory, so a directory the arm stops reading fails.
func languageLiteralFiles(t *testing.T, root string) []string {
	t.Helper()
	texts := map[string][]byte{}
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		text, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		texts[filepath.ToSlash(path)] = text
		return nil
	})
	if err != nil {
		t.Fatalf("reading the module as text for language-keyed literals: %v", err)
	}
	mapTypes := languageMapTypes(texts)
	literals := map[string]*regexp.Regexp{}
	var flagged []string
	for path, text := range texts {
		dir := filepath.Dir(path)
		if literals[dir] == nil {
			literals[dir] = languageLiteral(mapTypes[dir])
		}
		if literals[dir].Match(text) {
			flagged = append(flagged, path)
		}
	}
	slices.Sort(flagged)
	return flagged
}

func languageMapTypes(texts map[string][]byte) map[string][]string {
	keyed, namedAs := map[string]map[string]bool{}, map[string]map[string]string{}
	for path, text := range texts {
		dir := filepath.Dir(path)
		if keyed[dir] == nil {
			keyed[dir], namedAs[dir] = map[string]bool{}, map[string]string{}
		}
		for _, declared := range languageMapType.FindAllSubmatch(text, -1) {
			keyed[dir][string(declared[1])] = true
		}
		for _, declared := range namedType.FindAllSubmatch(text, -1) {
			namedAs[dir][string(declared[1])] = string(declared[2])
		}
	}
	names := map[string][]string{}
	for dir := range keyed {
		withNamedChains(keyed[dir], namedAs[dir])
		names[dir] = slices.Sorted(maps.Keys(keyed[dir]))
	}
	return names
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

// TestTheLanguageTableArmSeesATableAndItsWalk is the vacuity check. Shown six
// tables, two eliding a language map's type and one of a renamed one, the arm
// finds all six and the four walked. A local named like a table holds none, nor
// does a helper no Test runs, nor a test that calls no style check.
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
`
	pkg := &languagePackage{tables: map[string]string{}, literalIn: map[string]bool{}}
	for name, text := range map[string]string{"copy.go": source, "copy_test.go": walk} {
		file, err := parser.ParseFile(token.NewFileSet(), name, text, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		pkg.add(name, file)
	}
	pkg.resolve()
	want := []string{"buildShown", "messages", "nested", "orphaned", "unwalked", "walked"}
	if got := slices.Sorted(maps.Keys(pkg.tables)); !slices.Equal(got, want) {
		t.Errorf("the arm read the tables %v, want %v", got, want)
	}
	held := pkg.held(langcopytestChecks(t))
	if !held["walked"] || !held["buildShown"] || !held["nested"] || !held["messages"] || held["unwalked"] || held["orphaned"] {
		t.Errorf("the arm read the walked tables as %v", held)
	}
	chained := []byte("type raw map[textlang.Lang]string\ntype copyTable raw\ntype looped looping\ntype looping looped\n")
	if got := languageMapTypes(map[string][]byte{"copy/copy.go": chained})["copy"]; !slices.Equal(got, []string{"copyTable", "raw"}) {
		t.Errorf("the text census reads the language map types %v, want [copyTable raw]", got)
	}
	literal := languageLiteral([]string{"byKey"})
	for _, line := range []string{"var nested = map[string]map[textlang.Lang]string{", "return []*byKey{"} {
		if !literal.MatchString(line) {
			t.Errorf("the text census does not read %q as a language-keyed literal", line)
		}
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
	mapTypes  map[string]bool
	typeSpecs map[string]ast.Expr
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
			packages[dir] = &languagePackage{tables: map[string]string{}, literalIn: map[string]bool{}}
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
// named map type and its literals may sit in different files.
func (p *languagePackage) resolve() {
	p.mapTypes, p.typeSpecs = map[string]bool{}, map[string]ast.Expr{}
	namedAs := map[string]string{}
	for _, file := range p.files {
		lang := textlangQualifier(file)
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.TypeSpec); ok {
				p.typeSpecs[spec.Name.Name] = spec.Type
				p.mapTypes[spec.Name.Name] = languageKeyed(spec.Type, lang, nil)
				if underlying, named := ast.Unparen(spec.Type).(*ast.Ident); named {
					namedAs[spec.Name.Name] = underlying.Name
				}
			}
			return true
		})
	}
	withNamedChains(p.mapTypes, namedAs)
	for i, file := range p.files {
		lang := textlangQualifier(file)
		for _, decl := range file.Decls {
			for name, part := range declParts(decl) {
				implied := map[*ast.CompositeLit]ast.Expr{}
				ast.Inspect(part, func(node ast.Node) bool {
					literal, ok := node.(*ast.CompositeLit)
					if !ok {
						return true
					}
					literalType := literal.Type
					if literalType == nil {
						literalType = implied[literal]
					}
					if !languageKeyed(literalType, lang, p.mapTypes) {
						p.imply(literal, literalType, implied)
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

// imply gives each element literal that elides its type the element type of
// the literal around it. So {lang: …} inside a map of language maps is read.
func (p *languagePackage) imply(literal *ast.CompositeLit, literalType ast.Expr, implied map[*ast.CompositeLit]ast.Expr) {
	keyType, elementType := p.elementTypes(literalType)
	elide := func(element, elementType ast.Expr) {
		if pointer, ok := elementType.(*ast.StarExpr); ok {
			elementType = pointer.X
		}
		if inner, ok := element.(*ast.CompositeLit); ok && inner.Type == nil && elementType != nil {
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
func (p *languagePackage) elementTypes(container ast.Expr) (keyType, elementType ast.Expr) {
	for range len(p.typeSpecs) + 1 {
		switch t := container.(type) {
		case *ast.MapType:
			return t.Key, t.Value
		case *ast.ArrayType:
			return nil, t.Elt
		case *ast.ParenExpr:
			container = t.X
		case *ast.Ident:
			container = p.typeSpecs[t.Name]
		default:
			return nil, nil
		}
	}
	return nil, nil
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

// namedByStyleTests answers every name spelled by a test function that a Test
// runs and that calls a langcopytest style check. It follows calls to the
// directory's own test helpers. A helper no Test runs checks nothing.
func namedByStyleTests(tests []*ast.File, checks styleChecks) []string {
	reaches, calls, names := map[string]bool{}, map[string][]string{}, map[string][]string{}
	spelled := map[string][]string{}
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
					if checks.calledBy(n, qualifier) {
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
// a method of a value it returns, such as a Register.
func (c styleChecks) calledBy(call *ast.CallExpr, qualifier string) bool {
	callee := ast.Unparen(call.Fun)
	switch generic := callee.(type) {
	case *ast.IndexExpr:
		callee = generic.X
	case *ast.IndexListExpr:
		callee = generic.X
	}
	selector, ok := callee.(*ast.SelectorExpr)
	if !ok || qualifier == "" {
		return false
	}
	if x, isIdent := selector.X.(*ast.Ident); isIdent && x.Name == qualifier {
		return c.funcs[selector.Sel.Name]
	}
	return c.methods[selector.Sel.Name]
}
