// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

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
//     deterministic floor — is not a string anything can hash. Those surfaces
//     keep an explicit version constant for that half, and the two ride the
//     fingerprint together. `personbrief` is the pure case: it declares no
//     prompt at all and writes its whole output from code, so the rule does not
//     apply to it and it needs no waiver.
//   - A prompt declared in a different package from the version that caches its
//     output. Both censuses key on the declaring directory, so such a split is
//     invisible here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
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

// promptSurfaceFiles returns the product files of each prompt surface, grouped
// by package directory.
//
// Grouped rather than visited one at a time because Go constants are
// package-scoped: a prompt lives in `write.go` while the version that caches its
// output lives in `input.go`, and a census that resolved names file by file
// would not see across that.
func promptSurfaceFiles(t *testing.T) map[string][]*ast.File {
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
			dir := filepath.ToSlash(filepath.Dir(path))
			byDir[dir] = append(byDir[dir], file)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return byDir
}

// declaredPrompts returns, per package directory, the prompt constants it
// declares.
func declaredPrompts(t *testing.T) map[string][]string {
	t.Helper()
	prompts := map[string][]string{}
	for dir, files := range promptSurfaceFiles(t) {
		consts := stringConstants(files)
		for _, file := range files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok || len(value.Names) == 0 || len(value.Values) == 0 {
						continue
					}
					text, ok := stringValue(value.Values[0], consts)
					if !ok || len(text) < promptFloor {
						continue
					}
					if promptConstantName.MatchString(value.Names[0].Name) {
						prompts[dir] = append(prompts[dir], value.Names[0].Name)
					}
				}
			}
		}
	}
	return prompts
}

// handTypedPromptVersions are the directories declaring a `promptVersion` as a
// hand-typed STRING, whether spelled `const` or `var`.
//
// Both spellings, because the compliant form is a `var` — so `var
// promptVersion = "v2"` is the shape a regression naturally takes, and a census
// that only read `const` would be blind to exactly the thing it guards.
func handTypedPromptVersions(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for dir, files := range promptSurfaceFiles(t) {
		consts := stringConstants(files)
		for _, file := range files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok || len(value.Names) == 0 || len(value.Values) == 0 {
						continue
					}
					if !strings.Contains(strings.ToLower(value.Names[0].Name), "promptversion") {
						continue
					}
					if _, resolved := stringValue(value.Values[0], consts); resolved {
						out[dir] = true
					}
				}
			}
		}
	}
	return out
}

// digestedBuilders returns, per package directory, the identifiers handed to
// ai.PromptDigest there.
func digestedBuilders(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for dir, files := range promptSurfaceFiles(t) {
		for _, file := range files {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "PromptDigest" {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "ai" {
					return true
				}
				for _, arg := range call.Args {
					if ident, ok := arg.(*ast.Ident); ok {
						out[dir] = append(out[dir], ident.Name)
					}
				}
				return true
			})
		}
	}
	return out
}

// promptsReachedByDigest returns, per directory, the prompt constants the
// digested builders actually name in their bodies.
//
// Resolved through the builder rather than by trusting its name: the digest
// takes a function, and what that function sends is the only thing that says
// which prompt got covered.
func promptsReachedByDigest(t *testing.T, builders map[string][]string) map[string]map[string]bool {
	t.Helper()
	reached := map[string]map[string]bool{}
	for dir, files := range promptSurfaceFiles(t) {
		wanted := builders[dir]
		if len(wanted) == 0 {
			continue
		}
		reached[dir] = map[string]bool{}
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !slices.Contains(wanted, fn.Name.Name) {
					continue
				}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					if ident, ok := node.(*ast.Ident); ok {
						reached[dir][ident.Name] = true
					}
					return true
				})
			}
		}
	}
	return reached
}

func TestEveryPromptVersionIsDerivedFromItsPrompt(t *testing.T) {
	prompts := declaredPrompts(t)
	if len(prompts) == 0 {
		t.Fatal("the census found no prompt constant at all, so it is judging nothing")
	}
	handTyped := handTypedPromptVersions(t)

	var findings []string
	for dir := range handTyped {
		if len(prompts[dir]) == 0 {
			// No prompt to digest: the declaration versions something a digest
			// cannot see, which is a legitimate reason to keep it by hand.
			continue
		}
		findings = append(findings, dir+": promptVersion is hand-typed beside "+
			strings.Join(prompts[dir], ", "))
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
}

func TestEveryPromptOfADerivingPackageRidesItsDigest(t *testing.T) {
	// A ratification that stops matching is one for a prompt that has moved or
	// been folded in, and leaving it quietly re-exempts whatever takes its name.
	defer uncachedPrompts.AssertAllMatched(t)

	prompts := declaredPrompts(t)
	builders := digestedBuilders(t)
	if len(builders) == 0 {
		t.Fatal("no package derives a prompt version, so this census is vouching for nothing")
	}
	reached := promptsReachedByDigest(t, builders)

	var findings []string
	for dir := range builders {
		for _, prompt := range prompts[dir] {
			if reached[dir][prompt] || uncachedPrompts.Waived(t, dir+":"+prompt) {
				continue
			}
			findings = append(findings, dir+": "+prompt)
		}
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
}

// TestThePromptCensusSeesWhatItClaimsTo proves the rules the censuses are keyed
// on, in both directions.
//
// A census asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting.
func TestThePromptCensusSeesWhatItClaimsTo(t *testing.T) {
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
	// actually goes through it, since reading only the first fragment would let
	// a prompt escape by being split across lines.
	joined, ok := stringValue(&ast.BinaryExpr{
		Op: token.ADD,
		X:  &ast.BasicLit{Kind: token.STRING, Value: `"You write "`},
		Y:  &ast.BasicLit{Kind: token.STRING, Value: `"a brief."`},
	}, nil)
	if !ok || joined != "You write a brief." {
		t.Errorf("a prompt assembled with `+` is not read whole: %q", joined)
	}
	// The tree really does hold the constants the rule is written for, so the
	// naming it keys on is the tree's rather than an invention.
	prompts := declaredPrompts(t)
	found := map[string]bool{}
	for _, names := range prompts {
		for _, name := range names {
			found[name] = true
		}
	}
	for _, want := range []string{"briefSystem", "dossierSystem", "growthFitSystem"} {
		if !found[want] {
			t.Errorf("the census no longer finds %s — the naming rule has drifted from the tree", want)
		}
	}
	// And it does NOT count a provenance label that ends in the same word, which
	// would fire on a package whose hand-typed version is legitimate.
	for _, label := range []string{"trustSystem", "roleSystem", "actorTypeSystem", "transcriptSourceSystem"} {
		if found[label] {
			t.Errorf("the census counts %s as a prompt; it names where a record came from", label)
		}
	}
}
