// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// A test named in a comment exists.
//
// Naming the gate that holds an invariant — "bound by the census rather than by
// comment" — is one of the better habits in this tree, and it works only while
// the name resolves.
//
// A stale name is silent in the worst direction. It reads as a decision: the
// next author greps for it, finds nothing, and cannot tell whether the gate was
// renamed, deleted, or never written. `craft static` cannot see it, because it
// is a comment; no compiler sees it, for the same reason. Fifteen accumulated
// before anybody looked.
//
// The corpus is every tracked .go file in the tree, not this module: a comment
// in an extension or in the tool chain makes the same promise, and a comment
// here may name a gate that lives there.
//
// A name RESOLVES against every identifier the tree declares, not only against
// test functions — because the question a reader is really asking is whether
// grepping the name finds anything. `jobs.Config.TestOnly` is a field and
// `capture.TestMailboxLedger` is a type; prose naming either is telling the
// truth, and a gate that only knew about `func Test…` would call both stale and
// teach people to stop naming things.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// commentTestName is a Go test name as prose writes one. `Test` followed by an
// upper-case letter is what the toolchain itself requires of a test function,
// so this cannot match an ordinary word.
var commentTestName = regexp.MustCompile(`\bTest[A-Z][A-Za-z0-9_]*`)

// abbreviated marks a name a comment deliberately cut short — "TestEverySeat…"
// — which is a reference to a family rather than to one function. Both
// spellings, because prose here uses each.
var abbreviated = regexp.MustCompile(`^\s*(?:…|\.\.\.)`)

// runInvocation is a comment telling the reader how to run something. `-run`
// takes an unanchored regular expression, so a prefix is not a truncated name
// but the ordinary way to write one — `-run TestEveryPolicyRecordType` selects
// TestEveryPolicyRecordTypeIsOneItsToolCanSend and is exactly what its author
// meant. Requiring the full name here would teach people to paste a longer
// string than the command needs.
var runInvocation = regexp.MustCompile(`-run\s+$`)

func TestEveryTestNamedInACommentExists(t *testing.T) {
	t.Parallel()

	declared, testFuncs, comments := goCommentCensus(t)

	// The censusable corpus is asserted before its result is believed. A walk
	// that found no files reports a clean tree, and a clean tree is exactly
	// what a broken walk looks like. The test functions are counted separately
	// from the identifiers they are a subset of: a walk that collected every
	// `Test`-prefixed field and no test function at all would clear an
	// identifier floor while having lost the thing this gate is named for.
	if len(declared) < 1000 {
		t.Fatalf("collected %d Test-prefixed identifiers across the tree — the walk found far less than this repository holds, so a green result here would mean nothing", len(declared))
	}
	if len(testFuncs) < 1000 {
		t.Fatalf("collected %d declared test functions across the tree — the walk found far less than this repository holds, so a green result here would mean nothing", len(testFuncs))
	}
	if len(comments) < 100 {
		t.Fatalf("collected %d comment groups naming a test — the walk found far less than this repository holds, so a green result here would mean nothing", len(comments))
	}

	var findings []string
	for _, c := range comments {
		for _, name := range c.named {
			if declared[name] {
				continue
			}
			if c.wraps(name, declared) || c.abbreviates(name) || c.selects(name, declared) {
				continue
			}
			findings = append(findings, c.rel+": "+name)
		}
	}
	sort.Strings(findings)

	for _, f := range findings {
		rel, name, _ := strings.Cut(f, ": ")
		t.Errorf("%s names %s, which nothing in the tree declares — rename the reference to what actually holds this, or delete the claim if nothing does",
			rel, name)
	}
}

// commentBlock is one comment group and the three readings of it this gate
// compares. Prose wraps, and a name broken across two lines is a false hit
// unless the wrap is read back — but healing the text in place is worse, since
// an em-dash or a hyphen that belongs to the sentence gets absorbed into the
// identifier and invents a name nobody wrote. So the text is never rewritten:
// the alternative readings are built beside it and a name that resolves under
// ANY of them is a name the comment got right.
type commentBlock struct {
	rel string
	// named is what the comment says line by line, which is how a reader who
	// has not noticed the wrap reads it too.
	named []string
	// joined is the same text with the line breaks closed up, and hyphenJoined
	// with a trailing hyphen dropped first — the two ways a name is split.
	joined       string
	hyphenJoined string
	// text is the group as written, for the ellipsis lookahead.
	text string
}

// wraps reports whether name is the start of a declared test that the comment
// broke across a line. The name must be a strict prefix of something declared,
// and that something must appear in one of the closed-up readings — a prefix
// alone would excuse any truncation.
func (c commentBlock) wraps(name string, declared map[string]bool) bool {
	for _, reading := range []string{c.joined, c.hyphenJoined} {
		for _, candidate := range commentTestName.FindAllString(reading, -1) {
			if declared[candidate] && strings.HasPrefix(candidate, name) && candidate != name {
				return true
			}
		}
	}
	return false
}

// selects reports whether the name is a `go test -run` argument that picks out
// declared tests by prefix.
func (c commentBlock) selects(name string, declared map[string]bool) bool {
	segments := strings.Split(c.text, name)
	for _, before := range segments[:len(segments)-1] {
		if runInvocation.MatchString(before) && namesAFamily(name, declared) {
			return true
		}
	}
	return false
}

// namesAFamily reports whether anything declared begins with this name and is
// longer than it. A `-run` argument matching nothing is still a stale
// reference — the command it documents selects no test.
func namesAFamily(name string, declared map[string]bool) bool {
	for candidate := range declared {
		if candidate != name && strings.HasPrefix(candidate, name) {
			return true
		}
	}
	return false
}

// abbreviates reports whether the comment cut the name short on purpose, which
// it signals with an ellipsis immediately after.
func (c commentBlock) abbreviates(name string) bool {
	for _, after := range strings.Split(c.text, name)[1:] {
		if abbreviated.MatchString(after) {
			return true
		}
	}
	return false
}

// goCommentCensus parses every tracked .go file once, returning every
// Test-prefixed identifier the tree declares, the test functions among them,
// and every comment group naming a candidate.
//
// A file the parser cannot read is not skipped quietly. Go source that does not
// parse is either deliberate test data or a genuine problem, and either way a
// silent skip shrinks the corpus with nothing to notice — so a partial parse
// still yields its comments, and its declarations are simply what the parser
// recovered.
func goCommentCensus(t *testing.T) (declared, testFuncs map[string]bool, comments []commentBlock) {
	t.Helper()

	declared, testFuncs = map[string]bool{}, map[string]bool{}

	for _, f := range trackedFiles(t) {
		if f.symlink || filepath.Ext(f.path) != ".go" {
			continue
		}
		// This file's own prose names the pattern it looks for and nothing
		// else; the exemption is the exact path, so a second file by this name
		// elsewhere is scanned like any other.
		if f.path == "backend/gates/commentnamedtests_test.go" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(repoRoot, f.path))
		if err != nil {
			// Tracked but absent mid-rebase or after `git rm`; not this gate's
			// business.
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("reading %s: %v", f.path, err)
		}
		parsed, _ := parser.ParseFile(token.NewFileSet(), f.path, src, parser.ParseComments|parser.SkipObjectResolution)
		if parsed == nil {
			continue
		}
		// Every identifier, in any position. A declaration and a use are the
		// same answer to the reader's question, and telling them apart would
		// cost the cross-module case: a gate named in one module's comment is
		// declared in another's file, and both are in this walk either way.
		ast.Inspect(parsed, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && commentTestName.MatchString(ident.Name) {
				declared[ident.Name] = true
			}
			return true
		})
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				testFuncs[fn.Name.Name] = true
			}
		}
		for _, group := range parsed.Comments {
			text := group.Text()
			named := commentTestName.FindAllString(text, -1)
			if len(named) == 0 {
				continue
			}
			comments = append(comments, commentBlock{
				rel:          f.path,
				named:        named,
				text:         text,
				joined:       strings.ReplaceAll(text, "\n", ""),
				hyphenJoined: strings.ReplaceAll(strings.ReplaceAll(text, "-\n", ""), "\n", ""),
			})
		}
	}
	return declared, testFuncs, comments
}
