// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// A cached answer is keyed by a fingerprint, and the fingerprint has to move
// when the prompt that produced the answer moves. Otherwise a deploy that
// rewords a prompt keeps serving text written the old way to every record whose
// facts have not changed — which is most of them. Nothing errors, nothing is
// logged, and the reader cannot tell.
//
// THE RULE: in a package that DECLARES A PROMPT, a `promptVersion` declaration
// must be derived — `ai.PromptDigest(theBuilder)` — never a hand-typed string.
// A hand-typed one depends on somebody remembering, and remembering is the
// thing that fails.
//
// THE SECOND RULE: every prompt such a package declares must be REACHED by one
// of the digested builders. Deriving one prompt's version and leaving a sibling
// prompt out of the digest reintroduces the same defect for that sibling.
//
// The subject is a `promptVersion` declaration rather than "every declared
// prompt", because a prompt whose output is not cached has no key to go stale
// and reporting it buries the ones that matter.
//
// WHAT THE RULE DOES NOT COVER, stated because a reader will otherwise assume
// it does:
//
//   - Wording built by Go code from runtime facts — a `fmt.Sprintf` inside a
//     deterministic floor — is not a string anything can hash. A surface with
//     both halves keeps a hand-typed `floorVersion` beside its derived
//     `promptVersion`, and the two ride one fingerprint together; only the
//     second is this rule's subject, and the census reads the name rather than
//     the pair, so a floor version is invisible here whatever it is called.
//   - A prompt declared in a different package from the version that caches its
//     output. Both censuses key on the declaring directory, so such a split is
//     invisible here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// promptConstantName is this tree's own naming for a model instruction — every
// one of them is `briefSystem`, `askSystem`, `dossierSystem`, `growthFitSystem`
// — and a census keyed on the naming is one that finds the next one.
var promptConstantName = regexp.MustCompile(`(?:System|Prompt)$`)

// promptFloor separates a model instruction from a provenance LABEL that ends
// in the same word. `trustSystem`, `roleSystem`, `actorTypeSystem`,
// `fieldSourceSystem` and `transcriptSourceSystem` name where a record came
// from and are 2 to 19 characters; the shortest real prompt in the tree is 239.
// Reporting a label would fire on a package whose hand-typed version is
// legitimate, which teaches a reader to ignore this census.
//
// Length rather than "contains a newline": `judgeSystemPrompt` is a genuine
// prompt written on one line.
const promptFloor = 100

// promptSurfaceRoots are the trees where a prompt may be declared.
var promptSurfaceRoots = []string{"internal/compose", "internal/modules"}

// uncachedPrompts are prompts a deriving package declares but deliberately
// keeps OUT of its digest. Ratified by name because the reason is a property of
// the surface, not of the syntax: nothing in the declaration says whether its
// output is cached.
var uncachedPrompts = gatekit.Waive(map[string]string{
	"internal/compose/orgbrief:askSystem": "Ask answers are not cached (orgbrief.Service.Ask says so and explains why: a question is asked once and read once). Binding the ask prompt to the BRIEF's key would rewrite every cached brief for a change that cannot affect one.",
})

// promptPackage is one package directory: its product files, and the string
// constants they declare, resolved once.
//
// Grouped by package rather than visited file by file because Go constants are
// package-scoped — a prompt lives in `write.go` while the version caching its
// output lives in `input.go`, and a census resolving names per file cannot see
// across that.
type promptPackage struct {
	dir    string
	files  []*ast.File
	consts map[string]string
}

// promptSurfacePackages returns every package under the prompt surfaces.
//
// The traversal lives here alone. Each census below states only its own check;
// a walk copied into three of them is three chances for one to drift into
// reading a smaller tree than its siblings and reporting clean.
func promptSurfacePackages(t *testing.T) []promptPackage {
	t.Helper()
	byDir := map[string][]*ast.File{}
	fset := token.NewFileSet()
	for _, root := range promptSurfaceRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "testdata" || name == "contracts" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			byDir[filepath.ToSlash(filepath.Dir(path))] = append(byDir[filepath.ToSlash(filepath.Dir(path))], file)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	packages := make([]promptPackage, 0, len(byDir))
	for dir, files := range byDir {
		packages = append(packages, promptPackage{dir: dir, files: files, consts: stringConstants(files)})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].dir < packages[j].dir })
	return packages
}

// eachDeclaredString calls visit once per NAME a package binds, at the grain Go
// declares them.
//
// `const unused, briefSystem = "x", theLongPrompt` binds two names in one spec,
// and reading only the first hides the second — which is a way to declare a
// prompt, or a hand-typed version, that a census keyed on `Names[0]` cannot see.
//
// A spec whose names and values do not align one-to-one (a multi-return call, an
// iota run) binds no string this can read. Such a spec is reported to unreadable
// rather than skipped, so a shape this cannot judge fails loudly instead of
// passing quietly.
func eachDeclaredString(
	pkg promptPackage, allow func(token.Token) bool,
	visit func(name string, value ast.Expr), unreadable func(name string),
) {
	for _, file := range pkg.files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || !allow(gen.Tok) {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Values) == 0 {
					continue
				}
				if len(value.Names) != len(value.Values) {
					for _, name := range value.Names {
						unreadable(name.Name)
					}
					continue
				}
				for i, name := range value.Names {
					visit(name.Name, value.Values[i])
				}
			}
		}
	}
}

func isConst(tok token.Token) bool { return tok == token.CONST }

func isConstOrVar(tok token.Token) bool { return tok == token.CONST || tok == token.VAR }

func namesAVersion(name string) bool {
	return strings.Contains(strings.ToLower(name), "promptversion")
}

// declaredPrompts returns the prompt constants a package declares.
func declaredPrompts(pkg promptPackage) []string {
	var prompts []string
	eachDeclaredString(pkg, isConst, func(name string, value ast.Expr) {
		if !promptConstantName.MatchString(name) {
			return
		}
		if text, ok := gatekit.StringExpr(value, pkg.consts, gatekit.FoldStrict); ok && len(text) >= promptFloor {
			prompts = append(prompts, name)
		}
	}, func(string) {})
	sort.Strings(prompts)
	return prompts
}

// hasHandTypedVersion reports whether a package declares a `promptVersion` bound
// to a string this can read, whether spelled `const` or `var`.
//
// Both spellings, because the compliant form is a `var` — so `var promptVersion
// = "v2"` is the shape a regression naturally takes, and a census reading only
// `const` would be blind to exactly the thing it guards.
func hasHandTypedVersion(pkg promptPackage) bool {
	handTyped := false
	eachDeclaredString(pkg, isConstOrVar, func(name string, value ast.Expr) {
		if !namesAVersion(name) {
			return
		}
		if _, resolved := gatekit.StringExpr(value, pkg.consts, gatekit.FoldStrict); resolved {
			handTyped = true
		}
	}, func(string) {})
	return handTyped
}

// digestCoverage reports which prompts a package's ai.PromptDigest calls reach,
// and whether it makes any such call at all.
//
// Resolved through the BUILDER rather than by trusting an argument's name: the
// digest takes a function, and what that function sends is the only thing that
// says which prompt it covered. A named builder is followed to its declaration;
// a function literal is read where it stands.
//
// unsupported names an argument of neither shape, so a spelling this cannot
// follow fails loudly rather than silently covering nothing.
func digestCoverage(pkg promptPackage) (reached map[string]bool, derives bool, unsupported []string) {
	reached = map[string]bool{}
	var follow []string
	for _, file := range pkg.files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "PromptDigest" {
				return true
			}
			if pkgName, ok := sel.X.(*ast.Ident); !ok || pkgName.Name != "ai" {
				return true
			}
			derives = true
			if call.Ellipsis.IsValid() {
				// A spread hands over a slice built elsewhere; its elements are
				// not in this call to follow.
				unsupported = append(unsupported, types.ExprString(call.Args[len(call.Args)-1])+"...")
				return true
			}
			for _, arg := range call.Args {
				switch builder := arg.(type) {
				case *ast.Ident:
					reached[builder.Name] = true
					follow = append(follow, builder.Name)
				case *ast.FuncLit:
					collectIdents(builder.Body, reached)
					follow = append(follow, returnedNames(builder.Body)...)
				default:
					unsupported = append(unsupported, types.ExprString(arg))
				}
			}
			return true
		})
	}
	followBuilders(pkg, follow, reached)
	return reached, derives, unsupported
}

// followBuilders widens reached through the functions a builder RETURNS THROUGH,
// to a fixed point.
//
// A builder is free to delegate — a literal that calls a named builder, a named
// builder that calls another — and the prompt is sent at the bottom of that
// chain, not the top. Following one hop would clear the shape this tree happens
// to write today and miss the next one.
//
// Only the RETURN position is followed. A body may also call a helper for an
// unrelated effect, and following that would let a prompt the helper merely
// mentions count as sent — which would weaken the census whose whole point is to
// fail when a prompt rides no digest. What a builder returns is what reaches the
// model.
//
// Identifiers in a followed body still all count toward reached, so a prompt
// bound to a local before being returned is not missed. Being generous about the
// prompt and strict about the chain keeps the census from reporting a covered
// prompt as uncovered, which is the failure that teaches a reader to ignore it.
func followBuilders(pkg promptPackage, follow []string, reached map[string]bool) {
	bodies := map[string]*ast.BlockStmt{}
	for _, file := range pkg.files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				bodies[fn.Name.Name] = fn.Body
			}
		}
	}
	for len(follow) > 0 {
		name := follow[len(follow)-1]
		follow = follow[:len(follow)-1]
		body, isFunction := bodies[name]
		if !isFunction {
			continue
		}
		delete(bodies, name) // each body contributes once; this also ends a cycle
		collectIdents(body, reached)
		follow = append(follow, returnedNames(body)...)
	}
}

// returnedNames are the identifiers this body's OWN return statements name —
// the constants its value is built from, and the functions it delegates to.
//
// A nested closure's return is not this function's, so the walk does not descend
// into one. Otherwise a helper defined inline and never called would put its
// prompt on the chain, which is the same over-reach as following a call made for
// an unrelated effect. A literal handed to the digest is a builder in its own
// right and is walked as one, from its own body.
//
// A function called INSIDE a return expression is followed on purpose: `return
// statusSystemFor(fence)` sends what that builder sends, and delegation is the
// shape this has to see through.
func returnedNames(body *ast.BlockStmt) []string {
	var names []string
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			ast.Inspect(result, func(inner ast.Node) bool {
				if _, nested := inner.(*ast.FuncLit); nested {
					return false
				}
				if ident, ok := inner.(*ast.Ident); ok {
					names = append(names, ident.Name)
				}
				return true
			})
		}
		return true
	})
	return names
}

func collectIdents(node ast.Node, into map[string]bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			into[ident.Name] = true
		}
		return true
	})
}

func TestEveryPromptVersionIsDerivedFromItsPrompt(t *testing.T) {
	t.Parallel()
	packages := promptSurfacePackages(t)
	var findings, unreadable []string
	declaring := 0
	for _, pkg := range packages {
		prompts := declaredPrompts(pkg)
		if len(prompts) == 0 {
			// No prompt to digest: the declaration versions something a digest
			// cannot see, which is a legitimate reason to keep it by hand.
			continue
		}
		declaring++
		eachDeclaredString(pkg, isConstOrVar, func(string, ast.Expr) {}, func(name string) {
			if namesAVersion(name) || promptConstantName.MatchString(name) {
				unreadable = append(unreadable, pkg.dir+": "+name)
			}
		})
		if hasHandTypedVersion(pkg) {
			findings = append(findings, pkg.dir+": promptVersion is hand-typed beside "+
				strings.Join(prompts, ", "))
		}
	}
	if declaring == 0 {
		t.Fatal("the census found no package declaring a prompt, so it is judging nothing")
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d hand-typed promptVersion declaration(s) sit in a package that declares a prompt.\n\n"+
			"A key keyed on a typed string serves yesterday's writing after a deploy that reworded the "+
			"prompt and left it alone — silently, to every record whose facts have not moved. Derive "+
			"it:\n\n\tvar promptVersion = ai.PromptDigest(theSystemPromptBuilder)\n\n"+
			"and keep a separate constant for any wording built in Go code, which a digest cannot "+
			"reach.\n\n\t%s", len(findings), strings.Join(findings, "\n\t"))
	}
	// A declaration this cannot read is not a declaration this has cleared.
	if len(unreadable) > 0 {
		sort.Strings(unreadable)
		t.Errorf("%d prompt-or-version declaration(s) bind names and values that do not align "+
			"one-to-one, so the census cannot read them and cannot vouch for them. Split the "+
			"declaration, or teach eachDeclaredString the shape:\n\n\t%s",
			len(unreadable), strings.Join(unreadable, "\n\t"))
	}
}

func TestEveryPromptOfADerivingPackageRidesItsDigest(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching is one for a prompt that has moved or
	// been folded in, and leaving it quietly re-exempts whatever takes its name.
	defer uncachedPrompts.AssertAllMatched(t)

	var findings, unsupported []string
	deriving := 0
	for _, pkg := range promptSurfacePackages(t) {
		reached, derives, unreadableArgs := digestCoverage(pkg)
		if !derives {
			continue
		}
		deriving++
		for _, arg := range unreadableArgs {
			unsupported = append(unsupported, pkg.dir+": ai.PromptDigest("+arg+")")
		}
		for _, prompt := range declaredPrompts(pkg) {
			if reached[prompt] || uncachedPrompts.Waived(t, pkg.dir+":"+prompt) {
				continue
			}
			findings = append(findings, pkg.dir+": "+prompt)
		}
	}
	if deriving == 0 {
		t.Fatal("no package derives a prompt version, so this census is vouching for nothing")
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d prompt(s) are declared by a package that derives its version, but no digested "+
			"builder sends them.\n\n"+
			"Deriving one prompt's version and leaving a sibling out reintroduces the defect for the "+
			"sibling: rewording it moves nothing, and its cached output is served unchanged. Add its "+
			"builder to the ai.PromptDigest call, or ratify it in uncachedPrompts with the reason its "+
			"output is not cached.\n\n\t%s", len(findings), strings.Join(findings, "\n\t"))
	}
	// An argument this cannot follow covers nothing, and silence would read as
	// coverage.
	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		t.Errorf("%d ai.PromptDigest argument(s) are neither a named builder nor a function "+
			"literal, so the census cannot tell which prompt they send:\n\n\t%s",
			len(unsupported), strings.Join(unsupported, "\n\t"))
	}
}

// TestThePromptCensusSeesWhatItClaimsTo proves the rules the censuses are keyed
// on, in both directions.
//
// A census asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting.
func TestThePromptCensusSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"briefSystem", "askSystem", "dossierSystem", "growthFitSystem", "judgeSystemPrompt"} {
		if !promptConstantName.MatchString(name) {
			t.Errorf("the census does not recognise %q as a prompt constant", name)
		}
	}
	for _, name := range []string{"storedVersion", "auditKeyDomain", "entityTypePerson", "systemOfRecord"} {
		if promptConstantName.MatchString(name) {
			t.Errorf("the census reports %q, which is not a prompt constant", name)
		}
	}
	// A `+` chain is one prompt. The shared reader in this package handles that
	// (and parenthesised and `string(...)` spellings); this asserts the census
	// goes through it, since reading only the first fragment would let a prompt
	// escape by being split across lines.
	joined, ok := gatekit.StringExpr(&ast.BinaryExpr{
		Op: token.ADD,
		X:  &ast.BasicLit{Kind: token.STRING, Value: `"You write "`},
		Y:  &ast.BasicLit{Kind: token.STRING, Value: `"a brief."`},
	}, nil, gatekit.FoldStrict)
	if !ok || joined != "You write a brief." {
		t.Errorf("a prompt assembled with `+` is not read whole: %q", joined)
	}
	// The tree really does hold the constants the rule is written for, so the
	// naming it keys on is the tree's rather than an invention — and it does NOT
	// count a provenance label that ends in the same word, which would fire on a
	// package whose hand-typed version is legitimate.
	//
	// Both maps come from ONE walk. Parsing the whole prompt surface twice to
	// answer two questions about the same declarations is the cost this gate pays
	// on every `make check-backend`.
	found := map[string]bool{}
	namedLikeAPrompt := map[string]bool{}
	for _, pkg := range promptSurfacePackages(t) {
		for _, name := range declaredPrompts(pkg) {
			found[name] = true
		}
		eachDeclaredString(pkg, isConst, func(name string, value ast.Expr) {
			if _, isString := gatekit.StringExpr(value, pkg.consts, gatekit.FoldStrict); isString && promptConstantName.MatchString(name) {
				namedLikeAPrompt[name] = true
			}
		}, func(string) {})
	}
	for _, want := range []string{"briefSystem", "dossierSystem", "growthFitSystem"} {
		if !found[want] {
			t.Errorf("the census no longer finds %s — the naming rule has drifted from the tree", want)
		}
	}
	// Each label is asserted to EXIST before it is asserted to be excluded. A name
	// renamed away is absent from `found` for the wrong reason, and the check would
	// go on passing while proving nothing about the floor it exists to test.
	for _, label := range []string{"trustSystem", "roleSystem", "actorTypeSystem", "transcriptSourceSystem"} {
		if !namedLikeAPrompt[label] {
			t.Errorf("%s is gone from the tree, so it no longer tests that the floor excludes a "+
				"provenance label — name one that is still there", label)
			continue
		}
		if found[label] {
			t.Errorf("the census counts %s as a prompt; it names where a record came from", label)
		}
	}
}
