// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// Every prompt this product sends says what language to answer in, or says
// plainly why it does not need to.
//
// A model asked nothing about language answers in whatever language its input
// happened to be in. For a reply to a German thread that is right. For a claim
// filed on a record it is not: the thread has one reader, the record has
// everyone, so a Vietnamese conversation produced a Vietnamese claim a German
// colleague then had to read. Roughly two dozen prompts said nothing at all,
// and nothing made the twenty-fifth author notice.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It finds each `model.Request` composite
// literal and asks whether a language rule reaches its System field. That is a
// syntactic reach — it proves an instruction was ATTACHED, never that the right
// language was resolved, and a prompt could satisfy it while passing the wrong
// variable. The behavioural half is proven by tests that assemble a real prompt
// and read the language back out of it; this half is what stops a NEW prompt
// from being written with no language rule at all.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/draftrules"
	"github.com/gradionhq/margince/backend/internal/compose/promptlang"
)

// The heading a language rule opens with, read from the package that DEFINES
// it rather than restated here. A gate holding its own copy of the string it
// searches for is a second spelling of that string: the day promptlang changes
// the heading, a restated copy would keep passing and stop being about
// anything.
//
// Two packages write a rule under it — promptlang.Rule for the shared record,
// draftrules.Shared for correspondence, whose language is the correspondence's
// own rather than the installation's — and the assertion below keeps them one
// convention.
var languageRuleHeading = promptlang.Heading

// waiverPrefix marks a request whose output is not prose. A reasonless waiver
// is itself a finding, the same bar //craft:ignore holds.
const waiverPrefix = "//promptlang:exempt"

// promptTrees are the trees whose prompts this gate governs: everything that
// composes a request to a model on this installation's behalf.
var promptTrees = []string{
	"internal/compose",
	"internal/modules",
}

func TestEveryPromptSaysWhatLanguageToAnswerIn(t *testing.T) {
	// The two rule spellings must stay recognisable to one gate. A block that
	// stopped opening with the heading would leave every drafting surface
	// looking unguarded, and the gate would fail loudly rather than quietly
	// pass the wrong thing.
	if !strings.HasPrefix(draftrules.Shared, languageRuleHeading) { //nolint:gocritic // HasPrefix(subject, prefix): draftrules.Shared is the subject and the heading is the prefix, which is the order this reads in
		t.Fatalf("draftrules.Shared no longer opens with %q, so this gate can no longer recognise the drafting surfaces' own language rule", languageRuleHeading)
	}

	sites := everyModelRequest(t)
	if len(sites) == 0 {
		t.Fatal("found no model.Request literals at all; this gate would pass vacuously")
	}
	for _, site := range sites {
		if site.governed || site.waived {
			continue
		}
		t.Errorf("%s builds a model.Request with no language rule: its output language is whatever the input happened to be in. "+
			"Compose promptlang.Rule(<base language>) into the System prompt, or — if what it returns is data rather than prose "+
			"(numbers, enum values, field extractions) — mark it %s <reason>",
			site.where, waiverPrefix)
	}
}

func TestEveryPromptLanguageWaiverGivesAReason(t *testing.T) {
	for _, site := range everyModelRequest(t) {
		if site.waived && site.waiverReason == "" {
			t.Errorf("%s waives the language rule without saying why. A waiver nobody can check is one nobody will revisit: "+
				"say what this request returns that is not prose", site.where)
		}
	}
}

// requestSite is one place the product builds a request to a model.
type requestSite struct {
	// where names the file and line, which is what a failure has to print for
	// somebody to go and look.
	where string
	// governed is true when a language rule reaches this literal's System field.
	governed bool
	// waived is true when the enclosing function carries an exempt comment.
	waived       bool
	waiverReason string
}

// everyModelRequest walks the prompt trees and returns one entry per
// `model.Request` composite literal.
//
// Derived from the tree rather than from a list: a list is a thing to forget to
// add to, and forgetting is the failure this gate exists to catch.
func everyModelRequest(t *testing.T) []requestSite {
	t.Helper()
	var out []requestSite
	for _, tree := range promptTrees {
		err := filepath.WalkDir(tree, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Test files are excluded: a test's fixture prompt answers to the
			// assertions around it, not to what an installation reads.
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			out = append(out, requestSitesIn(t, path)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree, err)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].where < out[j].where })
	return out
}

// requestSitesIn parses one file and reports every model.Request literal in it.
func requestSitesIn(t *testing.T, path string) []requestSite {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	// The import path decides what the local name means. Assuming the package
	// is called `model` would miss a file that aliases it and, worse, would
	// wrongly claim a literal in a file with its own unrelated `model` package.
	local, ok := localNameFor(file, "shared/ports/model")
	if !ok {
		return nil
	}
	source := readSource(t, path)
	var out []requestSite
	ast.Inspect(file, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit || !isRequestLiteral(lit, local) {
			return true
		}
		pos := fset.Position(lit.Pos())
		reason, waived := waiverAround(file, fset, lit)
		out = append(out, requestSite{
			where:        fmt.Sprintf("%s:%d", path, pos.Line),
			governed:     systemCarriesALanguageRule(lit, source),
			waived:       waived,
			waiverReason: reason,
		})
		return true
	})
	return out
}

// isRequestLiteral reports whether this composite literal builds a
// model.Request. `[]model.Request{...}` is a slice literal whose ELEMENTS are
// the requests, and those elements are visited on their own, so only the
// selector form counts here — counting both would report one request twice.
func isRequestLiteral(lit *ast.CompositeLit, local string) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Request" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == local
}

// localNameFor returns the name this file refers to an imported package by,
// honouring an alias. Absent when the file does not import it at all.
func localNameFor(file *ast.File, suffix string) (string, bool) {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !strings.HasSuffix(path, suffix) {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		return path[strings.LastIndex(path, "/")+1:], true
	}
	return "", false
}

// systemCarriesALanguageRule reports whether a language rule reaches this
// literal's System field.
//
// The System expression is usually a concatenation of constants built
// elsewhere in the file, so the check is textual over the whole file rather
// than structural over the expression: following the value would mean
// resolving constants across packages, which is a type-checker's job and not
// worth pulling one in for. That is the imprecision this gate's own doc
// comment admits to — it can see that a rule is present and reachable, not
// that it was passed the right language.
func systemCarriesALanguageRule(lit *ast.CompositeLit, source string) bool {
	if !hasSystemField(lit) {
		// A request with no System field at all carries no prompt for a rule
		// to live in — a continuation turn, or a tool-result round trip. The
		// prompt that started it is governed where it was built.
		return true
	}
	return strings.Contains(source, "promptlang.") ||
		strings.Contains(source, "draftrules.Shared") ||
		strings.Contains(source, languageRuleHeading)
}

func hasSystemField(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "System" {
			return true
		}
	}
	return false
}

// waiverAround finds an exempt comment on the function enclosing this literal,
// and returns the reason written after it.
func waiverAround(file *ast.File, fset *token.FileSet, lit *ast.CompositeLit) (reason string, waived bool) {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !strings.HasPrefix(comment.Text, waiverPrefix) {
				continue
			}
			// A waiver governs the function it sits in or above, which is what
			// keeps one comment from silencing an unrelated request further
			// down the file.
			if fset.Position(comment.Pos()).Line > fset.Position(lit.Pos()).Line {
				continue
			}
			if enclosing := functionAround(file, lit); enclosing != nil &&
				fset.Position(comment.Pos()).Line < fset.Position(enclosing.Pos()).Line-1 {
				continue
			}
			return strings.TrimSpace(strings.TrimPrefix(comment.Text, waiverPrefix)), true
		}
	}
	return "", false
}

// functionAround returns the function declaration containing this literal.
func functionAround(file *ast.File, lit *ast.CompositeLit) *ast.FuncDecl {
	var found *ast.FuncDecl
	ast.Inspect(file, func(node ast.Node) bool {
		decl, ok := node.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if decl.Pos() <= lit.Pos() && lit.End() <= decl.End() {
			found = decl
		}
		return true
	})
	return found
}

func readSource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(raw)
}

// promptlang's docblock says it is where "write in this language" is spelled.
// That is only worth saying if something fails when a second one appears, which
// is what this does. A comment nobody checks is what stops the next author from
// looking, and this tree has already grown a second spelling of the rule once:
// the onboarding prompts carried their own "Respond in <locale>." for months,
// without the carve-outs that keep enum values and ids untranslated.
//
// So: no file outside promptlang may write the heading itself. Composing
// promptlang.Rule is how a prompt gets one, and draftrules is the one sanctioned
// second block — correspondence follows the correspondence's language, not the
// installation's, and it says so where it is declared.
func TestOnlyPromptlangSpellsTheLanguageRule(t *testing.T) {
	const (
		owner     = "internal/compose/promptlang/promptlang.go"
		sanctionA = "internal/compose/draftrules"
	)
	var offenders []string
	for _, tree := range promptTrees {
		err := filepath.WalkDir(tree, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if path == owner || strings.HasPrefix(path, sanctionA) {
				return nil
			}
			// The heading as it appears in SOURCE, which is not the same string
			// as the heading itself: promptlang.Heading holds a real newline,
			// and a Go file spells that as the two characters \ and n. Searching
			// for the runtime value finds nothing in any source file — including
			// a genuine second spelling — so the check would pass while looking
			// at the wrong thing.
			//
			// Both quotings are searched because a rule could be written in a
			// raw literal across two lines just as easily as an escaped one.
			source := readSource(t, path)
			escaped := strings.ReplaceAll(languageRuleHeading, "\n", `\n`)
			if strings.Contains(source, `"`+escaped) || strings.Contains(source, "`"+languageRuleHeading) {
				offenders = append(offenders, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree, err)
		}
	}
	for _, path := range offenders {
		t.Errorf("%s writes the language-rule heading itself, so there are now two spellings of one rule. "+
			"Compose promptlang.Rule(<language>) instead — a second block drifts from the first, and the "+
			"half that gets forgotten is the carve-out list that keeps enum values, ids and quoted source "+
			"text from being translated", path)
	}
}
