// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A refusal that wraps an apperrors sentinel opens as a sentence, never with a package's name.
//
// httperr.Classify puts the whole error text in the fault detail. The text a
// `fmt.Errorf` wraps around a sentinel is what the REST body and the MCP refusal
// show. `settings: x is not a registered setting: not found` names a Go package
// to a caller who has never seen one. The wrapped sentinel is followed through
// package vars that wrap it in turn, in this package or another one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// packagePrefix is a leading identifier-shaped word and a colon. That is how a
// Go package or step names itself, and never how a sentence for a caller opens.
var packagePrefix = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_./-]*: `)

// sentinelWrapRoots are the trees of this module that ship code. extensions/
// is not here: a unit imports only backend/pkg, which holds no apperrors.
var sentinelWrapRoots = []string{"internal", "pkg", "cmd"}

type sentinelKey struct{ pkg, name string }

// wrapSource is one parsed file and its package's import path. imports maps
// each qualifier to its import path, so a selector resolves to the var it reads.
type wrapSource struct {
	pkg     string
	file    *ast.File
	imports map[string]string
}

// sentinelWrap is one fmt.Errorf that wraps a sentinel. opaque is a format
// that opens with something other than a literal, so its opening cannot be read.
type sentinelWrap struct {
	at     token.Position
	format string
	opaque bool
}

func TestEverySentinelWrapOpensAsASentence(t *testing.T) {
	t.Parallel()
	var sources []wrapSource
	for _, root := range sentinelWrapRoots {
		sources = append(sources, wrapSources(t, root)...)
	}
	sentinels := sentinelVars(apperrorsSeed(t), sources)

	wraps := 0
	for _, src := range sources {
		for _, wrap := range sentinelWrapsIn(src, sentinels) {
			wraps++
			if finding := wrap.finding(); finding != "" {
				t.Errorf("%s: %s", wrap.at, finding)
			}
		}
	}
	if wraps == 0 {
		t.Fatalf("found no fmt.Errorf wrapping an apperrors sentinel under %v: the walk or the sentinel "+
			"resolution is broken, not the tree", sentinelWrapRoots)
	}
}

// finding says what is wrong with the wrap's opening, or "" when it reads as a
// sentence.
func (w sentinelWrap) finding() string {
	switch {
	case w.opaque:
		return "this wrap's format does not open with a string literal, so no reader of the source can " +
			"tell what the caller sees first. Open it with the literal sentence"
	case packagePrefix.MatchString(w.format):
		return strconv.Quote(w.format) + " opens with a package-style prefix, and the caller reads this " +
			"text whole in the REST detail and the MCP refusal. Reword it as a sentence for that caller"
	}
	return ""
}

func apperrorsSeed(t *testing.T) map[sentinelKey]bool {
	t.Helper()
	seed := map[sentinelKey]bool{}
	for name := range declaredSentinels(t) {
		seed[sentinelKey{modulePath + "/" + apperrorsDir, name}] = true
	}
	return seed
}

func wrapSources(t *testing.T, root string) []wrapSource {
	t.Helper()
	var sources []wrapSource
	err := gatekit.WalkDir(root, func(file string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(file, ".go") ||
			strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_gen.go") ||
			strings.Contains(file, "/testdata/") {
			return err
		}
		parsed, err := gatekit.ParseFile(file, 0)
		if err != nil {
			return err
		}
		sources = append(sources, newWrapSource(modulePath+"/"+path.Dir(file), parsed))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return sources
}

func newWrapSource(pkg string, parsed *ast.File) wrapSource {
	imports := map[string]string{}
	for _, spec := range parsed.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		qualifier := gatekit.DeclaredPackageName(imported)
		if spec.Name != nil {
			qualifier = spec.Name.Name
		}
		imports[qualifier] = imported
	}
	return wrapSource{pkg: pkg, file: parsed, imports: imports}
}

// sentinelVars widens the seed with every package-level var whose value is a
// sentinel or wraps one, until no further var joins.
func sentinelVars(seed map[sentinelKey]bool, sources []wrapSource) map[sentinelKey]bool {
	type declared struct {
		src   wrapSource
		value ast.Expr
	}
	candidates := map[sentinelKey]declared{}
	for _, src := range sources {
		for _, decl := range src.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != len(value.Values) {
					continue
				}
				for i, name := range value.Names {
					candidates[sentinelKey{src.pkg, name.Name}] = declared{src, value.Values[i]}
				}
			}
		}
	}
	sentinels := map[sentinelKey]bool{}
	for key := range seed {
		sentinels[key] = true
	}
	for grew := true; grew; {
		grew = false
		for key, candidate := range candidates {
			if sentinels[key] {
				continue
			}
			_, wraps := sentinelWrapCall(candidate.src, candidate.value, sentinels)
			if wraps || refersToSentinel(candidate.src, candidate.value, sentinels) {
				sentinels[key] = true
				grew = true
			}
		}
	}
	return sentinels
}

func sentinelWrapsIn(src wrapSource, sentinels map[sentinelKey]bool) []sentinelWrap {
	var wraps []sentinelWrap
	ast.Inspect(src.file, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if wrap, found := sentinelWrapCall(src, expr, sentinels); found {
			wrap.at = gatekit.SourceFileSet().Position(expr.Pos())
			wraps = append(wraps, wrap)
		}
		return true
	})
	return wraps
}

// sentinelWrapCall reports whether expr is a fmt.Errorf that wraps a sentinel,
// and the format it wraps it in.
func sentinelWrapCall(src wrapSource, expr ast.Expr, sentinels map[sentinelKey]bool) (sentinelWrap, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) < 2 || !callsErrorf(src, call.Fun) {
		return sentinelWrap{}, false
	}
	wrap := sentinelWrap{opaque: !opensWithLiteral(call.Args[0])}
	wrap.format, _ = gatekit.ConcatenatedString(call.Args[0])
	if !wrap.opaque && !strings.Contains(wrap.format, "%w") {
		return sentinelWrap{}, false
	}
	for _, arg := range call.Args[1:] {
		if refersToSentinel(src, arg, sentinels) {
			return wrap, true
		}
	}
	return sentinelWrap{}, false
}

func opensWithLiteral(format ast.Expr) bool {
	for {
		switch e := format.(type) {
		case *ast.BasicLit:
			return e.Kind == token.STRING
		case *ast.BinaryExpr:
			format = e.X
		default:
			return false
		}
	}
}

func callsErrorf(src wrapSource, fun ast.Expr) bool {
	qualifier, dotImported := gatekit.ImportedAs(src.file, "fmt")
	switch f := fun.(type) {
	case *ast.Ident:
		return dotImported && f.Name == "Errorf"
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		return ok && qualifier != "" && pkg.Name == qualifier && f.Sel.Name == "Errorf"
	}
	return false
}

func refersToSentinel(src wrapSource, expr ast.Expr, sentinels map[sentinelKey]bool) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return sentinels[sentinelKey{src.pkg, e.Name}]
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && sentinels[sentinelKey{src.imports[pkg.Name], e.Sel.Name}]
	}
	return false
}

func TestASentinelWrapIsJudgedByItsOpening(t *testing.T) {
	t.Parallel()
	seed := apperrorsSeed(t)
	for name, tc := range map[string]struct {
		source         string
		wraps, flagged int
	}{
		"a package prefix": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
func f(key string) error { return fmt.Errorf("settings: %s is not a registered setting: %w", key, apperrors.ErrNotFound) }`,
			wraps: 1, flagged: 1,
		},
		"a sentence": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
func f(key string) error { return fmt.Errorf("%s is not a setting this installation has: %w", key, apperrors.ErrNotFound) }`,
			wraps: 1, flagged: 0,
		},
		"a prefix across lines, under an alias": {
			source: `package probe
import ("fmt"; ae "github.com/margince/margince/backend/internal/shared/apperrors")
func f(id string) error {
	return fmt.Errorf(
		"capture: connection %s: %w",
		id,
		ae.ErrNotFound,
	)
}`,
			wraps: 1, flagged: 1,
		},
		"a package var that wraps a sentinel, wrapped again": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
var errGone = fmt.Errorf("the file's bytes were not kept: %w", apperrors.ErrNotFound)
func f() error { return fmt.Errorf("activities: %w", errGone) }`,
			wraps: 2, flagged: 1,
		},
		"a format that opens with a constant": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
const step = "settings"
func f() error { return fmt.Errorf(step+": %w", apperrors.ErrNotFound) }`,
			wraps: 1, flagged: 1,
		},
		"an internal error, which is not a refusal": {
			source: `package probe
import "fmt"
func f(err error) error { return fmt.Errorf("store: %w", err) }`,
			wraps: 0, flagged: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			src := newWrapSource("example.test/probe", file)
			wraps := sentinelWrapsIn(src, sentinelVars(seed, []wrapSource{src}))
			flagged := 0
			for _, wrap := range wraps {
				if wrap.finding() != "" {
					flagged++
				}
			}
			if len(wraps) != tc.wraps || flagged != tc.flagged {
				t.Errorf("found %d wrap(s), %d flagged; want %d and %d", len(wraps), flagged, tc.wraps, tc.flagged)
			}
		})
	}
}
