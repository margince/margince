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
	"fmt"
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

// valueOpening is a format that opens with a value and a colon, the prefix
// shape filled in at run time. %w is not one: it leads with the sentinel's text.
var valueOpening = regexp.MustCompile(`^%[^w%]: `)

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
	// opener is the value a format that opens with a verb puts first.
	opener ast.Expr
	// helper is set when opener is a parameter of the enclosing function. Its
	// callers then write the opening, and each call is judged instead.
	helper *wrapHelper
}

// wrapHelper is a function that wraps a sentinel around a sentence its caller
// passes as the parameter at index. rest is the format after that opening.
type wrapHelper struct {
	pkg, fn string
	index   int
	rest    string
}

func TestEverySentinelWrapOpensAsASentence(t *testing.T) {
	t.Parallel()
	var sources []wrapSource
	for _, root := range sentinelWrapRoots {
		sources = append(sources, wrapSources(t, root)...)
	}
	wraps, findings := sentinelWrapFindings(sources, sentinelVars(apperrorsSeed(t), sources))
	for _, finding := range findings {
		t.Error(finding)
	}
	if wraps == 0 {
		t.Fatalf("found no fmt.Errorf wrapping an apperrors sentinel under %v: the walk or the sentinel "+
			"resolution is broken, not the tree", sentinelWrapRoots)
	}
}

// sentinelWrapFindings judges every wrap in sources, and every call that hands a
// wrap helper its opening. It returns how many wraps it read.
func sentinelWrapFindings(sources []wrapSource, sentinels map[sentinelKey]bool) (int, []string) {
	var findings []string
	var helpers []wrapHelper
	wraps := 0
	for _, src := range sources {
		for _, wrap := range sentinelWrapsIn(src, sentinels) {
			wraps++
			if wrap.helper != nil {
				helpers = append(helpers, *wrap.helper)
			}
			if finding := wrap.finding(); finding != "" {
				findings = append(findings, fmt.Sprintf("%s: %s", wrap.at, finding))
			}
		}
	}
	for _, src := range sources {
		findings = append(findings, helperCallFindings(src, helpers)...)
	}
	return wraps, findings
}

// finding says what is wrong with the wrap's opening, or "" when it reads as a
// sentence.
func (w sentinelWrap) finding() string {
	switch {
	case w.opaque:
		return "this wrap's format does not open with a string literal, so no reader of the source can " +
			"tell what the caller sees first. Open it with the literal sentence"
	case w.helper != nil:
		return ""
	case valueOpening.MatchString(w.format) && !isErrorText(w.opener):
		return strconv.Quote(w.format) + " opens with a value, such as a check or a verb name, and the caller " +
			"reads it first. Open with words; only a type's own Error() sentence may lead"
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
	for _, decl := range src.file.Decls {
		fn, params := helperShape(decl)
		ast.Inspect(decl, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			if wrap, found := sentinelWrapCall(src, expr, sentinels); found {
				wrap.at = gatekit.SourceFileSet().Position(expr.Pos())
				if id, isIdent := wrap.opener.(*ast.Ident); isIdent && fn != "" {
					if index, isParam := params[id.Name]; isParam {
						wrap.helper = &wrapHelper{pkg: src.pkg, fn: fn, index: index, rest: wrap.format[2:]}
					}
				}
				wraps = append(wraps, wrap)
			}
			return true
		})
	}
	return wraps
}

// helperShape names a plain function and the index of each parameter. A wrap
// that opens with one of them takes its opening from the caller.
func helperShape(decl ast.Decl) (string, map[string]int) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv != nil {
		return "", nil
	}
	params := map[string]int{}
	index := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			params[name.Name] = index
			index++
		}
		if len(field.Names) == 0 {
			index++
		}
	}
	return fn.Name.Name, params
}

// helperCallFindings judges the opening each call in src hands a wrap helper of
// its own package, as the caller would read it.
func helperCallFindings(src wrapSource, helpers []wrapHelper) []string {
	var findings []string
	ast.Inspect(src.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		for _, helper := range helpers {
			if helper.pkg != src.pkg || helper.fn != name.Name || helper.index >= len(call.Args) {
				continue
			}
			at := gatekit.SourceFileSet().Position(call.Pos())
			opening, literal := gatekit.ConcatenatedString(call.Args[helper.index])
			switch {
			case !literal || !opensWithLiteral(call.Args[helper.index]):
				findings = append(findings, fmt.Sprintf("%s: %s opens a refusal with a value; pass it the literal sentence", at, helper.fn))
			case packagePrefix.MatchString(opening + helper.rest):
				findings = append(findings, fmt.Sprintf("%s: %s is handed %s, which opens with a package-style prefix; "+
					"reword it as a sentence for the caller", at, helper.fn, strconv.Quote(opening)))
			}
		}
		return true
	})
	return findings
}

// isErrorText reports a call of Error() with no arguments: a type's own
// sentence, which the type already answers as its whole detail.
func isErrorText(opener ast.Expr) bool {
	call, ok := opener.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	return ok && method.Sel.Name == "Error"
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
	if valueOpening.MatchString(wrap.format) {
		wrap.opener = call.Args[1]
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
		"a check name that opens the refusal": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
func f() error { check := "deal.update"; return fmt.Errorf("%s: %w", check, apperrors.ErrPermissionDenied) }`,
			wraps: 1, flagged: 1,
		},
		"a type's own sentence that opens the refusal": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
type ceiling struct{}
func (ceiling) Error() string { return "at most ten a minute" }
func f() error { return fmt.Errorf("%s: %w", ceiling{}.Error(), apperrors.ErrBudgetExceeded) }`,
			wraps: 1, flagged: 0,
		},
		"a helper whose callers write the opening": {
			source: `package probe
import ("fmt"; "github.com/margince/margince/backend/internal/shared/apperrors")
func invalid(message string) error { return fmt.Errorf("%s: %w", message, apperrors.ErrInvalidArgument) }
func a() error { return invalid("choose a valid product") }
func b() error { return invalid("reporting: no window") }
func c(name string) error { return invalid(name) }`,
			wraps: 1, flagged: 2,
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
			sources := []wrapSource{newWrapSource("example.test/probe", file)}
			wraps, findings := sentinelWrapFindings(sources, sentinelVars(seed, sources))
			if wraps != tc.wraps || len(findings) != tc.flagged {
				t.Errorf("found %d wrap(s), flagged %v; want %d and %d", wraps, findings, tc.wraps, tc.flagged)
			}
		})
	}
}
