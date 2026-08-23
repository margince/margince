// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A cached answer is keyed by a fingerprint, and the fingerprint has to move
// when the prompt that produced the answer moves. Otherwise a deploy that
// rewords a prompt keeps serving text written the old way to every record whose
// facts have not changed — which is most of them.
//
// That is not hypothetical. `orgbrief`'s own docblock recorded it happening, in
// its own words: a deploy reworded the floor, left the version constant alone,
// and kept serving the old sentences. The remedy chosen at the time was a
// comment telling the next author to remember. This is the test that replaces
// remembering.
//
// THE RULE: a `promptVersion` declaration in a package that DECLARES A PROMPT
// must be derived — `ai.PromptDigest(theSystemPrompt)` — never a hand-typed
// string constant.
//
// That is the subject precisely, and it took two wrong ones to find. "Every
// declared prompt must be digested" reports 28 surfaces, most of which cache
// nothing — a prompt with no cached output has no key to go stale, and
// reporting them buries the few that matter. "Every package that declares a
// prompt AND computes a fingerprint" still reports 18, because `internal/compose`
// is one directory holding a dozen unrelated surfaces and a directory is the
// wrong grain for coupling a prompt to the key that caches ITS output.
//
// What is exactly derivable is the thing being replaced: a constant named
// `promptVersion`. There are two in the tree, and one of them is correct —
// `personbrief` declares no prompt at all and writes its whole output from Go
// code, so there is nothing for a digest to see. The rule does not need a
// waiver for it; it simply does not apply.
//
// WHAT THE RULE DOES NOT COVER, stated because a reader will otherwise assume
// it does: wording built by Go code — a `fmt.Sprintf` inside a deterministic
// floor — is not a string anything can hash. Those surfaces keep an explicit
// version constant for that half, and the two ride the fingerprint together.
// `personbrief` is the pure case: it declares no prompt at all, writes its
// whole output from code, and is correctly absent from this census. "Adopt it
// everywhere" would have been the wrong answer for exactly one of the four.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// promptConstant is a declared prompt: a package-level string constant whose
// name ends in `System` or `Prompt`. That is this tree's own naming — every one
// of them is `briefSystem`, `askSystem`, `dossierSystem`, `growthFitSystem` —
// and a census keyed on the naming is a census that finds the next one.
var promptConstantName = regexp.MustCompile(`(?:System|Prompt)$`)

// promptSurfaces are the trees where a prompt may be declared.
var promptSurfaceRoots = []string{"internal/compose", "internal/modules"}

// declaredPrompts returns, per package directory, the prompt constants it
// declares.
func declaredPrompts(t *testing.T) map[string][]string {
	t.Helper()
	prompts := map[string][]string{}
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
					lit, ok := value.Values[0].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if promptConstantName.MatchString(value.Names[0].Name) {
						prompts[dir] = append(prompts[dir], value.Names[0].Name)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return prompts
}

func TestEveryPromptVersionIsDerivedFromItsPrompt(t *testing.T) {
	prompts := declaredPrompts(t)
	if len(prompts) == 0 {
		t.Fatal("the census found no prompt constant at all, so it is judging nothing")
	}
	handTyped := handTypedPromptVersions(t)
	derived := promptVersionArguments(t)

	var findings []string
	for dir := range handTyped {
		if len(prompts[dir]) == 0 {
			// No prompt to digest: the constant versions something a digest
			// cannot see, which is a legitimate reason to keep it by hand.
			continue
		}
		findings = append(findings, dir+": promptVersion is a constant beside "+
			strings.Join(prompts[dir], ", "))
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d hand-typed promptVersion constant(s) sit in a package that declares a prompt.\n\n"+
			"A key keyed on a constant serves yesterday's writing after a deploy that reworded the "+
			"prompt and left the constant alone — silently, to every record whose facts have not "+
			"moved. Derive it:\n\n\tvar promptVersion = ai.PromptDigest(theSystemPrompt)\n\n"+
			"and keep a separate constant for any wording built in Go code, which a digest cannot "+
			"reach.\n\n\t%s", len(findings), strings.Join(findings, "\n\t"))
	}
	// The derived side has to be real, or every arm above passes over a tree
	// where nothing is derived at all.
	if len(derived) == 0 {
		t.Error("no package derives a prompt version, so this census is vouching for nothing")
	}
}

// handTypedPromptVersions are the directories declaring `promptVersion` as a
// string CONSTANT rather than deriving it.
func handTypedPromptVersions(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, root := range promptSurfaceRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok || len(value.Names) == 0 {
						continue
					}
					if strings.Contains(strings.ToLower(value.Names[0].Name), "promptversion") {
						out[filepath.ToSlash(filepath.Dir(path))] = true
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return out
}

// promptVersionArguments returns, per package directory, the identifiers handed
// to ai.PromptVersion there.
func promptVersionArguments(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	fset := token.NewFileSet()
	for _, root := range promptSurfaceRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			ast.Inspect(file, func(node ast.Node) bool {
				callExpr, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := callExpr.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "PromptDigest" {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "ai" {
					return true
				}
				for _, arg := range callExpr.Args {
					if ident, ok := arg.(*ast.Ident); ok {
						out[dir] = append(out[dir], ident.Name)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return out
}

// TestThePromptCensusSeesWhatItClaimsTo proves the naming rule the census is
// keyed on, in both directions.
//
// A census asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting.
func TestThePromptCensusSeesWhatItClaimsTo(t *testing.T) {
	for _, name := range []string{"briefSystem", "askSystem", "dossierSystem", "growthFitSystem", "replyPrompt"} {
		if !promptConstantName.MatchString(name) {
			t.Errorf("the census does not recognise %q as a prompt constant", name)
		}
	}
	// Names that end in neither word are not prompts, and reporting them would
	// bury the finding under every string constant in the tree.
	for _, name := range []string{"storedVersion", "auditKeyDomain", "entityTypePerson", "systemOfRecord"} {
		if promptConstantName.MatchString(name) {
			t.Errorf("the census reports %q, which is not a prompt constant", name)
		}
	}
	// And the tree really does hold the ones the rule is written for, so the
	// naming convention it keys on is the tree's rather than an invention.
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
}
