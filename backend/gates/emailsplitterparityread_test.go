// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Reading the two splitters' tables — one out of a Go AST, one out of
// TypeScript source text.
//
// Both readers normalise a pattern to the same pair of (case-insensitive,
// body), because the two dialects spell the flag differently: Go carries it
// inside the pattern as `(?i)`, TypeScript hangs it off the literal as `/i`.
// Comparing the raw text would report every case-insensitive pattern as drift
// and teach the next reader to ignore this gate.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// normalizedPattern is one regular expression as both dialects can state it.
func normalizedPattern(body string, insensitive bool) string {
	if trimmed := strings.TrimPrefix(body, "(?i)"); trimmed != body {
		body, insensitive = trimmed, true
	}
	return fmt.Sprintf("i=%t %s", insensitive, body)
}

// goSplitterTables reads every vocabulary out of the Go file's declarations.
//
// Through the AST rather than by pattern, because the entries ARE string
// literals: a regex over the source would have to decide for itself where a
// literal ends, which is the reader this tree keeps once (gatekit) rather than
// per census.
func goSplitterTables(t *testing.T) map[string][]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), goSplitter, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", goSplitter, err)
	}
	out := map[string][]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			if entries := goTableEntries(t, value.Values[0]); entries != nil {
				out[value.Names[0].Name] = entries
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no vocabulary parsed out of %s — a gate that reads nothing agrees with everything", goSplitter)
	}
	return out
}

// goTableEntries reads one declaration's value: a slice of strings, a set
// spelled as a map's keys, a slice of compiled patterns, or a single one.
func goTableEntries(t *testing.T, value ast.Expr) []string {
	t.Helper()
	if call, ok := value.(*ast.CallExpr); ok {
		if pattern, ok := mustCompileArgument(t, call); ok {
			return []string{pattern}
		}
		return nil
	}
	composite, ok := value.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	entries := []string{}
	for _, element := range composite.Elts {
		switch node := element.(type) {
		case *ast.KeyValueExpr:
			// A set spelled `map[string]bool{"mfg": true}`: the KEY is the entry.
			if text, ok := gatekit.StringExpr(node.Key, nil, gatekit.FoldStrict); ok {
				entries = append(entries, text)
			}
		case *ast.CallExpr:
			if pattern, ok := mustCompileArgument(t, node); ok {
				entries = append(entries, pattern)
			}
		default:
			if text, ok := gatekit.StringExpr(element, nil, gatekit.FoldStrict); ok {
				entries = append(entries, text)
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

// mustCompileArgument reads the pattern out of a regexp.MustCompile call.
func mustCompileArgument(t *testing.T, call *ast.CallExpr) (string, bool) {
	t.Helper()
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "MustCompile" || len(call.Args) != 1 {
		return "", false
	}
	body, ok := gatekit.StringExpr(call.Args[0], nil, gatekit.FoldStrict)
	if !ok {
		t.Fatalf("%s: a regexp.MustCompile argument this gate cannot read is a pattern it cannot compare", goSplitter)
	}
	return normalizedPattern(body, false), true
}

// goSplitterConst reads one integer constant out of the Go file.
func goSplitterConst(t *testing.T, name string) int {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), goSplitter, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", goSplitter, err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
				continue
			}
			lit, ok := value.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.INT {
				t.Fatalf("%s: %s is not an integer literal", goSplitter, name)
			}
			parsed, convErr := strconv.Atoi(lit.Value)
			if convErr != nil {
				t.Fatalf("%s: %s = %q is not a number", goSplitter, name, lit.Value)
			}
			return parsed
		}
	}
	t.Fatalf("%s declares no const %s — this gate is reading a shape that is gone", goSplitter, name)
	return 0
}

// tsComments strips comments before anything is parsed. Without it a line
// MENTIONING a sign-off keeps this gate green after the real entry is deleted —
// the lesson frontendminorunits_test.go records, applied here before it costs
// anything.
var tsComments = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

var (
	// A declaration's body, up to the terminator its opening bracket implies.
	tsDeclaration = regexp.MustCompile(`(?s)const\s+([A-Z][A-Z0-9_]*)\s*=\s*(new Set\(\[|\[|/)`)
	// A double- or single-quoted string, which is how both a word list and a
	// Set spell an entry. A backticked one is not read, and that is safe in the
	// direction that matters: an entry this parser misses is absent from the
	// TypeScript side and the Go side still holds it, so it surfaces as drift
	// rather than as agreement. What it cannot be is silent.
	tsString = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'`)
	// A regex literal and its flags. Matched with the delimiters, so a slash
	// inside a character class does not end it early.
	tsRegex = regexp.MustCompile(`/((?:[^/\\\n\[]|\\.|\[(?:[^\]\\]|\\.)*\])+)/([a-z]*)`)
	// A bare number, for the scan window.
	tsNumber = regexp.MustCompile(`const\s+([A-Z][A-Z0-9_]*)\s*=\s*(\d+)\s*;`)
)

// tsSplitterTables reads every vocabulary out of the TypeScript file.
func tsSplitterTables(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(tsSplitter)
	if err != nil {
		t.Fatalf("reading %s: %v", tsSplitter, err)
	}
	source := tsComments.ReplaceAllString(string(raw), " ")
	out := map[string][]string{}
	for _, match := range tsDeclaration.FindAllStringSubmatchIndex(source, -1) {
		name := source[match[2]:match[3]]
		body, ok := tsDeclarationBody(source, match[4], source[match[4]:match[5]])
		if !ok {
			t.Fatalf("%s: %s's declaration is unterminated", tsSplitter, name)
		}
		if entries := tsEntries(body); entries != nil {
			out[name] = entries
		}
	}
	if len(out) == 0 {
		t.Fatalf("no vocabulary parsed out of %s — a gate that reads nothing agrees with everything", tsSplitter)
	}
	return out
}

// tsDeclarationBody returns the text a declaration's opener encloses. A regex
// literal is its own body; a list or a Set runs to its closing bracket.
func tsDeclarationBody(source string, at int, opener string) (string, bool) {
	if opener == "/" {
		end := strings.Index(source[at:], ";")
		if end < 0 {
			return "", false
		}
		return source[at : at+end], true
	}
	depth := 0
	for i := at; i < len(source); i++ {
		switch source[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return source[at : i+1], true
			}
		}
	}
	return "", false
}

// tsEntries reads one declaration body as words or as patterns. A body holding
// regex literals is read as patterns and never as strings, so a pattern that
// happens to contain a quote is not half-read as a word.
func tsEntries(body string) []string {
	if matches := tsRegex.FindAllStringSubmatch(body, -1); len(matches) > 0 {
		entries := make([]string, 0, len(matches))
		for _, m := range matches {
			entries = append(entries, normalizedPattern(m[1], strings.Contains(m[2], "i")))
		}
		return entries
	}
	matches := tsString.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	entries := make([]string, 0, len(matches))
	for _, m := range matches {
		text := m[1]
		if text == "" {
			text = m[2]
		}
		entries = append(entries, text)
	}
	return entries
}

// tsSplitterConst reads one numeric constant out of the TypeScript file.
func tsSplitterConst(t *testing.T, name string) int {
	t.Helper()
	raw, err := os.ReadFile(tsSplitter)
	if err != nil {
		t.Fatalf("reading %s: %v", tsSplitter, err)
	}
	source := tsComments.ReplaceAllString(string(raw), " ")
	for _, m := range tsNumber.FindAllStringSubmatch(source, -1) {
		if m[1] == name {
			parsed, convErr := strconv.Atoi(m[2])
			if convErr != nil {
				t.Fatalf("%s: %s = %q is not a number", tsSplitter, name, m[2])
			}
			return parsed
		}
	}
	t.Fatalf("%s declares no const %s — this gate is reading a shape that is gone", tsSplitter, name)
	return 0
}
