// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Every JSON request body is bounded in one place.
//
// "A JSON body is at most 1 MiB" used to live in two: the chassis
// (httpserver.LimitBodies) and httperr.Decode. A handler decoding r.Body
// directly was correct — the chassis bounds it — and nothing said so, so the
// chassis was its ONLY bound. Nineteen handlers did, two of them
// unauthenticated, and three took []string bodies, which is the shape where a
// widened ceiling becomes heap amplification rather than a big string.
//
// The cost is not that they were wrong. It is that they were correct BY
// ACCIDENT of a decision made somewhere else: an earlier cut of the
// route-ceiling work widened on Content-Type alone and those nineteen silently
// became 25 MB each, and no test anywhere failed. That is what this refuses —
// a bound a handler inherits without naming, which the next change to the
// chassis can take away in silence.
//
// The enumeration in the issue that found this listed sixteen. By the time it
// was fixed there were nineteen. Which is the argument for a rule rather than a
// list, and this gate is that rule.
//
// WHAT IS PERMITTED. httperr.Decode for a handler answering problem+json, and
// httperr.DecodeOrRefusal for the two whose wire shape is somebody else's — dynamic
// client registration speaks RFC 7591, a report run treats an absent body as
// its defaults. Both go through the same reader and the same cap; only the
// refusal differs. What is forbidden is reaching r.Body with a decoder at all.
//
// NOT A SIZE CHECK. This gate cannot see how large a body may be — that is
// httperr.MaxBodyBytes and the operator's per-route upload ceiling. It sees
// only whether a handler asked for the bound or inherited it, which is the
// property that decides whether a chassis change can silently unbind it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	netHTTPPkg      = "net/http"
	encodingJSONPkg = "encoding/json"
	ioPkg           = "io"
)

// The package that owns the bound. Its own reader is the one every other file
// is required to reach, so it cannot be required to reach itself.
const httperrDir = "/platform/httperr/"

func TestNoHandlerDecodesARequestBodyItself(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileTouchesRequestBody,
		// Not extensions/: a unit never holds an *http.Request. The core
		// answers its routes and hands it a decoded record.
	}.Files(t)

	judged := 0
	for _, parsed := range files {
		if strings.Contains(parsed.Path, httperrDir) {
			continue
		}
		judged++
		for _, site := range rawBodyDecodesIn(parsed) {
			t.Errorf("%s: %s reads r.Body with its own decoder.\n"+
				"\tThe 1 MiB cap then comes only from the chassis, which this file does not "+
				"mention and cannot be held to — an earlier widening of the route ceiling made "+
				"exactly these handlers 25 MB each with no test failing.\n"+
				"\tUse httperr.Decode, or httperr.DecodeOrRefusal if this endpoint answers in a wire "+
				"shape that is not problem+json.", parsed.Path, site)
		}
	}
	// A prohibition that swept nothing reads exactly like a clean tree, and the
	// count is taken after the exclusion: httperr's own files touch r.Body by
	// definition, so counting before it would let a walk that found only those
	// report a successful empty run.
	if judged == 0 {
		t.Fatal("no file outside httperr touches a request body, so this prohibition judged " +
			"nothing — the walk is broken, or the shapes it looks for were renamed")
	}
}

func fileTouchesRequestBody(_ string, file *ast.File) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if requests := requestParams(file, fn); len(requests) > 0 && touchesBodyOf(fn.Body, requests) {
			return true
		}
	}
	return false
}

// requestParam is the name this function gave its *http.Request, or "" for one
// that takes none.
//
// The PARAMETER, not any identifier spelled `r`. `resp.Body` on an outbound
// call is a response this code is reading back, `req.Body` on a request this
// code is BUILDING is a body it wrote itself, and neither is a body a caller
// sent — a matcher on `.Body` alone reported both, plus every test harness that
// builds a request to send. What this rule is about is the one body that
// arrives from outside and whose size the sender chooses.
func requestParams(file *ast.File, fn *ast.FuncDecl) []string {
	if fn.Type.Params == nil {
		return nil
	}
	qualifier, dotImported := gatekit.ImportedAs(file, netHTTPPkg)
	var names []string
	for _, field := range fn.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if !isHTTPRequestType(star.X, qualifier, dotImported) {
			continue
		}
		// EVERY name in the field, not the first. `func h(w http.ResponseWriter,
		// r, next *http.Request)` declares two, and a matcher taking only
		// `field.Names[0]` would read one of them and let the other through.
		for _, name := range field.Names {
			if name.Name != "_" {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

// isHTTPRequestType matches `http.Request` under the name this file gave
// net/http, or the bare `Request` a dot-import puts in scope.
//
// Resolved from the IMPORT PATH rather than from the identifier `http`. An
// alias — `stdhttp "net/http"` — would otherwise slip every handler in the file
// past this gate, and the alias is not exotic: a file importing two packages
// that both end in `http` has to rename one.
func isHTTPRequestType(expr ast.Expr, qualifier string, dotImported bool) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return dotImported && t.Name == "Request"
	case *ast.SelectorExpr:
		if t.Sel.Name != "Request" {
			return false
		}
		pkg, ok := t.X.(*ast.Ident)
		return ok && qualifier != "" && pkg.Name == qualifier
	}
	return false
}

func touchesBodyOf(body *ast.BlockStmt, requests []string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if isBodyOfAny(n, requests) {
			found = true
		}
		return true
	})
	return found
}

// rawBodyDecodesIn reports each place this file builds a reader over r.Body,
// named by the function it sits in.
func rawBodyDecodesIn(parsed gatekit.ParsedFile) []string {
	var sites []string
	for _, decl := range parsed.File.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		requests := requestParams(parsed.File, fn)
		if len(requests) == 0 {
			continue
		}
		// A function that has already replaced the body with a bounded reader
		// has named its cap in view, which is the property this gate is about.
		// The overlay webhook does exactly that and says why: MaxBytesReader
		// rather than a bare LimitReader, so an over-cap batch answers 413
		// instead of being truncated and then rejected as a bad signature.
		if boundsItsOwnBody(fn.Body, requests) {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// TWO SHAPES, and only two.
			//
			// `json.NewDecoder(r.Body)` — a decoder straight onto the body,
			// which is the shape with no cap of its own anywhere in view.
			//
			// `io.ReadAll(r.Body)` with no limiter — the same, one layer down.
			//
			// What is NOT forbidden is a bounded peek: several edges read a
			// prefix of the body to throttle on, put it back in front of the
			// unread remainder, and hand the handler the request the client
			// actually sent. Those name their own cap on the line
			// (`io.LimitReader(r.Body, linkRequestBodyLimit)`), which is the
			// property this gate is about — a bound a reader can see. They
			// TRUNCATE rather than refuse, which is right for a peek that falls
			// back and wrong for a decode, and the handler behind them still
			// answers the oversized body itself.
			shape := forbiddenBodyRead(parsed.File, call, requests)
			if shape == "" {
				return true
			}
			sites = append(sites, fn.Name.Name+" ("+shape+")")
			return false
		})
	}
	return dedupeSites(sites)
}

// isBodyOfAny matches `<request>.Body` for ANY name this function gave an
// *http.Request, and nothing else.
func isBodyOfAny(n ast.Node, requests []string) bool {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Body" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, request := range requests {
		if ident.Name == request {
			return true
		}
	}
	return false
}

// boundsItsOwnBody reports `<request>.Body = http.MaxBytesReader(…)` anywhere in
// this function — the one way a handler may take the bound into its own hands,
// and it can only ever TIGHTEN what the chassis already granted.
func boundsItsOwnBody(body *ast.BlockStmt, requests []string) bool {
	bounded := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		if !isBodyOfAny(assign.Lhs[0], requests) {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "MaxBytesReader" {
			bounded = true
		}
		return true
	})
	return bounded
}

// forbiddenBodyRead names the shape if this call is one of the two, or "".
//
// Both are resolved through the file's own imports. `codec "encoding/json"` and
// `reader "io"` are legal and would otherwise walk straight past a matcher
// keyed on the identifiers `json` and `io` — which is the same hole the request
// type had, one layer along.
func forbiddenBodyRead(file *ast.File, call *ast.CallExpr, requests []string) string {
	if len(call.Args) == 0 || !isBodyOfAny(call.Args[0], requests) {
		return ""
	}
	if callsPackageFunc(file, call.Fun, encodingJSONPkg, "NewDecoder") {
		return "json.NewDecoder"
	}
	if callsPackageFunc(file, call.Fun, ioPkg, "ReadAll") {
		return "io.ReadAll with no limiter"
	}
	return ""
}

// callsPackageFunc reports a call to importPath.name under whatever name this
// file gave that import, dot-imports included.
func callsPackageFunc(file *ast.File, fun ast.Expr, importPath, name string) bool {
	qualifier, dotImported := gatekit.ImportedAs(file, importPath)
	switch f := fun.(type) {
	case *ast.Ident:
		return dotImported && f.Name == name
	case *ast.SelectorExpr:
		if f.Sel.Name != name {
			return false
		}
		pkg, ok := f.X.(*ast.Ident)
		return ok && qualifier != "" && pkg.Name == qualifier
	}
	return false
}

func dedupeSites(items []string) []string {
	var kept []string
	seen := map[string]bool{}
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			kept = append(kept, item)
		}
	}
	return kept
}

// What the rule must and must not catch, written here because the tree no
// longer contains either kind.
//
// Every case above runs over a converted tree, so all of them pass whether the
// walk distinguishes a request body from a response body or not — and that
// distinction is the whole of the matcher. A `.Body` matcher with no request
// parameter behind it reported thirteen files on its first run: outbound
// responses being read back, requests this code was BUILDING, and every test
// harness that sends one.
func TestOnlyAnInboundRequestBodyIsJudged(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		source string
		leaks  int
	}{
		"a decoder straight onto the request body": {
			source: `package p
import ("encoding/json"; "net/http")
func h(w http.ResponseWriter, r *http.Request) {
	var req struct{ A string }
	_ = json.NewDecoder(r.Body).Decode(&req)
}`,
			leaks: 1,
		},
		// Whatever the handler named its request. Several in this tree do not
		// call it `r`, and a matcher keyed on the spelling would miss them.
		"the same, on a request named something else": {
			source: `package p
import ("encoding/json"; "net/http")
func h(w http.ResponseWriter, req *http.Request) {
	var body struct{ A string }
	_ = json.NewDecoder(req.Body).Decode(&body)
}`,
			leaks: 1,
		},
		"an unlimited read of the request body": {
			source: `package p
import ("io"; "net/http")
func h(w http.ResponseWriter, r *http.Request) {
	_, _ = io.ReadAll(r.Body)
}`,
			leaks: 1,
		},
		// A RESPONSE this code read back from somewhere else. Its size is the
		// far end's problem, and it is not a body a caller sent here.
		"a response body from an outbound call": {
			source: `package p
import ("encoding/json"; "net/http")
func h(w http.ResponseWriter, r *http.Request) {
	resp, _ := http.Get("https://example.invalid")
	var out struct{ A string }
	_ = json.NewDecoder(resp.Body).Decode(&out)
}`,
			leaks: 0,
		},
		// The handler bounded it itself, which is the one sanctioned way — and
		// it can only tighten what the chassis already granted.
		"a body the handler bounded first": {
			source: `package p
import ("io"; "net/http")
func h(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	_, _ = io.ReadAll(r.Body)
}`,
			leaks: 0,
		},
		// A bounded peek: the cap is on the line, and the handler behind it
		// still answers the body the client actually sent.
		"a peek with its own named cap": {
			source: `package p
import ("io"; "net/http")
const peek = 4096
func h(w http.ResponseWriter, r *http.Request) {
	_, _ = io.ReadAll(io.LimitReader(r.Body, peek))
}`,
			leaks: 0,
		},
		// Every one of these renames something the matcher used to key on by
		// spelling. None is exotic — a file importing two packages that both
		// end in `http` has to rename one — and each would have walked a whole
		// file past the gate.
		"net/http under an alias": {
			source: `package p
import (
	"encoding/json"
	stdhttp "net/http"
)
func h(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var req struct{ A string }
	_ = json.NewDecoder(r.Body).Decode(&req)
}`,
			leaks: 1,
		},
		"encoding/json under an alias": {
			source: `package p
import (
	codec "encoding/json"
	"net/http"
)
func h(w http.ResponseWriter, r *http.Request) {
	var req struct{ A string }
	_ = codec.NewDecoder(r.Body).Decode(&req)
}`,
			leaks: 1,
		},
		"io under an alias": {
			source: `package p
import (
	reader "io"
	"net/http"
)
func h(w http.ResponseWriter, r *http.Request) {
	_, _ = reader.ReadAll(r.Body)
}`,
			leaks: 1,
		},
		// A grouped parameter declares two names in one field, and the matcher
		// took only the first — so a read through the second was invisible.
		"the second of two request parameters": {
			source: `package p
import (
	"encoding/json"
	"net/http"
)
func h(w http.ResponseWriter, first, second *http.Request) {
	var req struct{ A string }
	_ = json.NewDecoder(second.Body).Decode(&req)
}`,
			leaks: 1,
		},
		// And the other direction, which is what makes resolving the import
		// load-bearing rather than decorative: a same-named function from a
		// DIFFERENT package. `bounded.ReadAll` is somebody's own helper that
		// already applies a cap, and a matcher keyed on the name alone would
		// report it as an unbounded read.
		"a same-named helper from another package": {
			source: `package p
import (
	"net/http"
	"example.test/bounded"
)
func h(w http.ResponseWriter, r *http.Request) {
	_, _ = bounded.ReadAll(r.Body)
}`,
			leaks: 0,
		},
		"the sanctioned decode": {
			source: `package p
import (
	"net/http"
	"github.com/margince/margince/backend/internal/platform/httperr"
)
func h(w http.ResponseWriter, r *http.Request) {
	var req struct{ A string }
	if !httperr.Decode(w, r, &req) {
		return
	}
}`,
			leaks: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			sites := rawBodyDecodesIn(gatekit.ParsedFile{Path: "probe.go", File: file})
			if len(sites) != tc.leaks {
				t.Errorf("found %d site(s) %v, want %d", len(sites), sites, tc.leaks)
			}
			// The scope predicate must agree with the finder: a file the sweep
			// never enters is one whose findings nobody reads.
			if reads := fileTouchesRequestBody("probe.go", file); tc.leaks > 0 && !reads {
				t.Error("the finder reports a site in a file the sweep would not enter")
			}
		})
	}
}
