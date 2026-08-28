// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// probeUnitName is the name every probe unit here mounts under, and the one the
// literal paths below are written against. Named rather than passed in, because
// a name a caller could vary would have to be threaded through every one of
// those paths to stay true.
const probeUnitName = "u"

// inboundProbe is a unit whose handler records what it was given and answers a
// chosen outcome.
type inboundProbe struct {
	saw     extension.InboundRequest
	caller  extension.Caller
	calls   int
	outcome extension.InboundOutcome
	err     error
	// ingestErr is what Ingest answered when the handler tried it.
	ingestErr error
	tryIngest bool
}

func (p *inboundProbe) unit() extension.Extension {
	endpoint := inboundEndpointFixture("capture", "inbound")
	endpoint.Handle = func(ctx context.Context, rt extension.Runtime, req extension.InboundRequest) (extension.InboundOutcome, error) {
		p.calls++
		p.saw = req
		p.caller = rt.Caller()
		if p.tryIngest {
			// The record names the unit's DECLARED source, so the call reaches
			// the unattended gate rather than stopping at the source check —
			// otherwise the test would pass on a refusal that has nothing to do
			// with the edge's authority.
			_, p.ingestErr = rt.Ingest(ctx, "someone", extension.Record{System: "probe"})
		}
		return p.outcome, p.err
	}
	e := unitWithInbound(probeUnitName, endpoint)
	e.Ingress = []extension.IngressSource{{System: "probe", Lands: []extension.RecordKind{extension.KindActivity}}}
	return e
}

// mountProbe mounts one probe unit and returns the mux plus the reported routes.
func mountProbe(t *testing.T, p *inboundProbe) (*http.ServeMux, []InboundRoute) {
	t.Helper()
	mux := http.NewServeMux()
	ws := ids.From[ids.WorkspaceKind](ids.MustParse("00000000-0000-0000-0000-0000000000a1"))
	routes := MountInboundEndpoints(mux, []extension.Extension{p.unit()},
		func(context.Context) (ids.WorkspaceID, error) { return ws, nil },
		extensionRuntimeBinding{}, quietLog())
	return mux, routes
}

// signedRequest is a well-formed request at the given time.
func signedRequest(path string, at time.Time, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set(extension.InboundHeaderTimestamp, strconv.FormatInt(at.Unix(), 10))
	r.Header.Set(extension.InboundHeaderNonce, "0f1e2d3c")
	r.Header.Set(extension.InboundHeaderSignature, "sha256=deadbeef")
	return r
}

func serve(mux *http.ServeMux, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// The registrations must be REPORTED, not merely mounted: extparity's documented
// residual is that a mux.Handle nobody returned is invisible to the sweep, and
// an anonymous edge is the last route that should be invisible to a census.
func TestMountInboundReportsWhatItRegistered(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	_, routes := mountProbe(t, p)
	if len(routes) != 1 {
		t.Fatalf("reported %d routes, want 1", len(routes))
	}
	if routes[0].Pattern != "/webhooks/ext/u/capture" {
		t.Fatalf("reported pattern %q", routes[0].Pattern)
	}
}

func TestMountInboundReportsNothingForAUnitWithNoEdge(t *testing.T) {
	mux := http.NewServeMux()
	routes := MountInboundEndpoints(mux, []extension.Extension{{Name: "u", Version: "0.1.0"}},
		func(context.Context) (ids.WorkspaceID, error) { return ids.WorkspaceID{}, nil },
		extensionRuntimeBinding{}, quietLog())
	if len(routes) != 0 {
		t.Fatalf("reported %d routes for a unit declaring none", len(routes))
	}
}

func TestInboundServesADeclaredEndpoint(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	w := serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), `{"a":1}`))
	if w.Code != http.StatusAccepted {
		t.Fatalf("answered %d, want 202", w.Code)
	}
	if p.calls != 1 {
		t.Fatalf("handler reached %d times, want 1", p.calls)
	}
	if string(p.saw.Body) != `{"a":1}` {
		t.Fatalf("handler saw body %q", p.saw.Body)
	}
	if p.saw.Nonce != "0f1e2d3c" || p.saw.Signature != "sha256=deadbeef" {
		t.Fatalf("handler saw nonce %q signature %q", p.saw.Nonce, p.saw.Signature)
	}
	if p.saw.Slug != "capture" {
		t.Fatalf("handler saw slug %q — it must be the DECLARED value, not the raw path segment", p.saw.Slug)
	}
}

// An undeclared slug is a 404 from the mux, never a refusal this code composed:
// answering from inside would turn "does this endpoint exist" into a question
// the handler answers, which is the enumeration the opaque 401 prevents.
func TestInboundUndeclaredPathsAre404(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	for _, path := range []string{
		"/webhooks/ext/u/other",
		"/webhooks/ext/other/capture",
		"/webhooks/ext/u",
		"/webhooks/ext/",
	} {
		if got := serve(mux, signedRequest(path, time.Now(), "{}")).Code; got != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", path, got)
		}
	}
	if p.calls != 0 {
		t.Fatalf("an undeclared path reached the unit %d times", p.calls)
	}
}

// The mux answers the cases above, so the handler's own resolution is never
// reached through it. It is exercised DIRECTLY here, because the handler is
// shared by every declared endpoint and its guard has to hold on its own terms —
// a check nothing can reach is a check nobody is keeping.
func TestInboundHandlerResolvesAgainstTheDeclarationItself(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux := http.NewServeMux()
	ws := ids.From[ids.WorkspaceKind](ids.MustParse("00000000-0000-0000-0000-0000000000a1"))
	MountInboundEndpoints(mux, []extension.Extension{p.unit()},
		func(context.Context) (ids.WorkspaceID, error) { return ws, nil },
		extensionRuntimeBinding{}, quietLog())

	// The same handler the mux holds, addressed without it.
	h, pattern := mux.Handler(signedRequest("/webhooks/ext/u/capture", time.Now(), "{}"))
	if pattern == "" {
		t.Fatal("the declared pattern did not route")
	}
	for _, path := range []string{
		"/webhooks/ext/u/undeclared",
		"/webhooks/ext/unknown/capture",
		"/webhooks/ext/u",
		"/not-the-prefix/u/capture",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest(path, time.Now(), "{}"))
		if w.Code != http.StatusNotFound {
			t.Errorf("the handler answered %d for %s, want 404 — it resolved a path nothing declared", w.Code, path)
		}
	}
	if p.calls != 0 {
		t.Fatalf("the unit ran for %d undeclared paths", p.calls)
	}
}

// The edge mounts one pattern per DECLARED endpoint, never a prefix. A prefix
// handler would answer for everything under /webhooks/ext/ — and, mounted one
// segment higher, for the provider receivers too — so this fixes the shape
// rather than only the current paths.
//
// Mount order matches routes.go: the provider receivers first, the extension
// edges after, which is the order in which a prefix registration would do damage.
func TestInboundMountsExactPatternsAndShadowsNothing(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux := http.NewServeMux()
	reached := ""
	for _, path := range []string{"/webhooks/gmail", "/webhooks/hubspot"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			reached = r.URL.Path
			w.WriteHeader(http.StatusOK)
		})
	}
	ws := ids.From[ids.WorkspaceKind](ids.MustParse("00000000-0000-0000-0000-0000000000a1"))
	MountInboundEndpoints(mux, []extension.Extension{p.unit()},
		func(context.Context) (ids.WorkspaceID, error) { return ws, nil },
		extensionRuntimeBinding{}, quietLog())

	for _, path := range []string{"/webhooks/gmail", "/webhooks/hubspot"} {
		reached = ""
		if got := serve(mux, httptest.NewRequest(http.MethodPost, path, nil)).Code; got != http.StatusOK {
			t.Errorf("%s answered %d — the extension mount shadowed it", path, got)
		}
		if reached != path {
			t.Errorf("%s reached %q instead", path, reached)
		}
	}

	// The half a prefix registration would break: a sibling path under
	// /webhooks/ that nothing declared must 404 from the mux, not reach the
	// extension handler and be judged there.
	// A single trailing segment IS a declared shape now (the per-member ref), so
	// the undeclared cases are a sibling path and a second trailing segment.
	for _, path := range []string{"/webhooks/unclaimed", "/webhooks/ext/u/capture/ref/extra"} {
		if got := serve(mux, signedRequest(path, time.Now(), "{}")).Code; got != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404 — the mount is answering for paths it never declared", path, got)
		}
	}
	if p.calls != 0 {
		t.Fatalf("the extension handler was reached %d times by paths it does not own", p.calls)
	}
}

func TestInboundRefusesNonPost(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	r := httptest.NewRequest(http.MethodGet, "/webhooks/ext/u/capture", nil)
	if got := serve(mux, r).Code; got != http.StatusMethodNotAllowed {
		t.Fatalf("GET answered %d, want 405", got)
	}
	if p.calls != 0 {
		t.Fatal("a GET reached the unit")
	}
}

// Every reason a request fails to authenticate answers ONE opaque 401 with an
// empty body — byte-identical, or the differences enumerate the installation.
func TestInboundRefusalsAreByteIdentical(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundUnauthenticated}
	mux, _ := mountProbe(t, p)
	now := time.Now()
	requests := map[string]*http.Request{
		"stale timestamp":  signedRequest("/webhooks/ext/u/capture", now.Add(-time.Hour), "{}"),
		"future timestamp": signedRequest("/webhooks/ext/u/capture", now.Add(time.Hour), "{}"),
		"the unit refused": signedRequest("/webhooks/ext/u/capture", now, "{}"),
	}
	absent := signedRequest("/webhooks/ext/u/capture", now, "{}")
	absent.Header.Del(extension.InboundHeaderTimestamp)
	requests["absent timestamp"] = absent
	noNonce := signedRequest("/webhooks/ext/u/capture", now, "{}")
	noNonce.Header.Del(extension.InboundHeaderNonce)
	requests["absent nonce"] = noNonce
	noSig := signedRequest("/webhooks/ext/u/capture", now, "{}")
	noSig.Header.Del(extension.InboundHeaderSignature)
	requests["absent signature"] = noSig

	for name, r := range requests {
		w := serve(mux, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s answered %d, want 401", name, w.Code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("%s answered with a %d-byte body — the refusal must reveal nothing", name, w.Body.Len())
		}
	}
}

func TestInboundOutcomesMapToStatus(t *testing.T) {
	tests := []struct {
		outcome extension.InboundOutcome
		want    int
	}{
		{extension.InboundAccepted, http.StatusAccepted},
		{extension.InboundUnauthenticated, http.StatusUnauthorized},
		{extension.InboundOverCapacity, http.StatusTooManyRequests},
		// Poison answers 2xx so a sender does not retry forever against a
		// payload we will never accept.
		{extension.InboundPoison, http.StatusAccepted},
		{extension.InboundTransient, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		p := &inboundProbe{outcome: tc.outcome, err: errors.New("probe")}
		mux, _ := mountProbe(t, p)
		if got := serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), "{}")).Code; got != tc.want {
			t.Errorf("outcome %d answered %d, want %d", tc.outcome, got, tc.want)
		}
	}
}

// An outcome outside the declared set is a bug in the unit, not a fact about
// the request, and must not be read as success.
func TestInboundUnrecognizedOutcomeIsAServerError(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundOutcome(99)}
	mux, _ := mountProbe(t, p)
	if got := serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), "{}")).Code; got != http.StatusInternalServerError {
		t.Fatalf("an unrecognized outcome answered %d, want 500", got)
	}
}

func TestInboundCapsTheBody(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	// The fixture declares 64 KiB.
	oversized := strings.Repeat("x", 64<<10+1)
	if got := serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), oversized)).Code; got != http.StatusRequestEntityTooLarge {
		t.Fatalf("an oversized body answered %d, want 413", got)
	}
	if p.calls != 0 {
		t.Fatal("an oversized body reached the unit")
	}
}

// A deployment fault must not be described to a stranger: not-bootstrapped and
// more-than-one-workspace are indistinguishable from outside.
func TestInboundTenantFailureIsOpaque(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux := http.NewServeMux()
	MountInboundEndpoints(mux, []extension.Extension{p.unit()},
		func(context.Context) (ids.WorkspaceID, error) {
			return ids.WorkspaceID{}, errors.New("identity: installation not bootstrapped")
		},
		extensionRuntimeBinding{}, quietLog())
	w := serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), "{}"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("answered %d, want an opaque 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "bootstrap") {
		t.Fatalf("the response named the deployment fault: %q", w.Body.String())
	}
	if p.calls != 0 {
		t.Fatal("the unit ran without a resolved workspace")
	}
}

// unattended is false on this edge, so Caller() is NOT the zero value — it
// derives from the principal. Pinned, because the design's own note is that the
// answer is not automatic.
func TestInboundCallerIsABareConnector(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), "{}"))
	if p.caller.Type != extension.CallerConnector {
		t.Fatalf("Caller().Type = %v, want a connector", p.caller.Type)
	}
	if p.caller.UserID != "" {
		t.Fatalf("Caller().UserID = %q, want empty — an anonymous edge acts on nobody's behalf", p.caller.UserID)
	}
}

// The edge must stay closed to capture. A signed POST is a stranger holding a
// secret, not a member exercising authority, and a record landed straight from
// one would be landed on nobody's behalf.
func TestInboundCannotIngest(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted, tryIngest: true}
	unit := p.unit()
	// Ingest resolves the caller's declared sources from the composed set, so
	// the unit has to BE composed or the call stops at the source check and the
	// test would pass on a refusal that says nothing about the edge's authority.
	previous := ComposedExtensions()
	setComposedExtensions([]extension.Extension{unit})
	t.Cleanup(func() { setComposedExtensions(previous) })

	mux, _ := mountProbe(t, p)
	serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), "{}"))
	if !errors.Is(p.ingestErr, extension.ErrAttendedIngest) {
		t.Fatalf("Ingest from an inbound Runtime answered %v, want ErrAttendedIngest", p.ingestErr)
	}
}

// Empty permissions are load-bearing: auth.Require has no connector branch, so a
// bare connector passes exactly what its permissions allow, which is nothing.
func TestABareConnectorIsDeniedByRequire(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   "connector:ext:u",
	})
	if err := auth.Require(ctx, "person", principal.ActionRead); err == nil {
		t.Fatal("a bare connector with no permissions passed auth.Require")
	}
}

// inboundEdgeDoors are the ways into the anonymous edge: the mount that builds
// the handler, the handler itself, and the runtime an anonymous invocation is
// given. A walk that started anywhere else would be describing a different
// surface.
var inboundEdgeDoors = []string{"MountInboundEndpoints", "inboundHandler.ServeHTTP", "inboundRuntimeFor"}

// auth.RequireHuman ADMITS connectors, so a gate spelled with it alone would
// pass an anonymous signed request as though a person had made it. This walks
// the compose package from the edge's doors through every same-package call it
// can reach and pins that none of them calls it.
//
// It reads the WHOLE package rather than the edge's own file, because the edge
// is already spread across more than one: a file-scoped grep goes on reporting
// PASS the moment a reachable helper moves next door, and a security census
// that can fail short has already failed. Its blind spot is named rather than
// implied — a call made through a func value, or into another package, is not
// followed, so a helper reached only that way is not covered here.
func TestTheInboundEdgeReachesNoRequireHumanGate(t *testing.T) {
	bodies := composeFuncDecls(t)
	for _, door := range inboundEdgeDoors {
		if bodies[door] == nil {
			t.Fatalf("the edge no longer declares %s — this walk would start nowhere, so re-point "+
				"the door here rather than leaving the gate reading an empty set", door)
		}
	}
	reached := map[string]bool{}
	for _, door := range inboundEdgeDoors {
		walkInboundCalls(t, door, bodies, reached)
	}
	// The floor is however many functions extinbound.go itself declares today,
	// read from that file rather than hard-coded: a hard-coded number goes
	// stale the moment the edge is split across files or grows a function, and
	// either direction of drift is invisible to a count nobody derives from the
	// file it is supposed to describe. Reaching fewer than that many in total
	// means the walk stopped resolving calls and is reporting agreement with a
	// corpus it failed to read.
	floor := funcDeclCount(t, "extinbound.go")
	if len(reached) < floor {
		t.Fatalf("the walk reached only %d functions from the edge's doors, which is fewer than the "+
			"%d extinbound.go itself declares — it is passing on a corpus it failed to read", len(reached), floor)
	}
}

// funcDeclCount counts the function declarations with a body in one file of
// the compose package, using the same parse the census above already performs
// over the whole directory.
func funcDeclCount(t *testing.T, name string) int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	count := 0
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			count++
		}
	}
	return count
}

// composeFuncDecls indexes every function the compose package declares, keyed
// the way a resolved call site names it: `Name`, or `Receiver.Name`.
func composeFuncDecls(t *testing.T) map[string]*ast.FuncDecl {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the compose package: %v", err)
	}
	fset := token.NewFileSet()
	bodies := map[string]*ast.FuncDecl{}
	read := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		read++
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			bodies[declKey(fn)] = fn
		}
	}
	if read == 0 {
		t.Fatal("read no compose source — the walk would report PASS having looked at nothing")
	}
	return bodies
}

// walkInboundCalls marks one function reached and follows every same-package
// call in its body, failing the moment one of them is auth.RequireHuman.
func walkInboundCalls(t *testing.T, key string, bodies map[string]*ast.FuncDecl, reached map[string]bool) {
	t.Helper()
	if reached[key] {
		return
	}
	reached[key] = true
	fn := bodies[key]
	if fn == nil {
		return
	}
	types := receiverTypes(fn)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		// Any MENTION, not only a call: handing RequireHuman on as a func value
		// puts it just as surely on this path, and a walk that watched only call
		// sites would report PASS on the one spelling written to evade it.
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "auth" && sel.Sel.Name == "RequireHuman" {
				t.Errorf("%s names auth.RequireHuman and is reachable from the anonymous inbound edge — "+
					"connectors pass that check, so it would admit a signed request as though a person had made it", key)
			}
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			walkInboundCalls(t, callee.Name, bodies, reached)
		case *ast.SelectorExpr:
			ident, ok := callee.X.(*ast.Ident)
			if !ok {
				return true
			}
			if owner := types[ident.Name]; owner != "" {
				walkInboundCalls(t, owner+"."+callee.Sel.Name, bodies, reached)
			}
		}
		return true
	})
}

// receiverTypes answers, for one function, which of its identifiers name a
// value of a type declared in this package — its receiver and its parameters.
// A method call on anything else is not followed, which is the blind spot the
// test's own comment names.
func receiverTypes(fn *ast.FuncDecl) map[string]string {
	types := map[string]string{}
	fields := []*ast.Field{}
	if fn.Recv != nil {
		fields = append(fields, fn.Recv.List...)
	}
	if fn.Type.Params != nil {
		fields = append(fields, fn.Type.Params.List...)
	}
	for _, field := range fields {
		name := bareTypeName(field.Type)
		if name == "" {
			continue
		}
		for _, ident := range field.Names {
			types[ident.Name] = name
		}
	}
	return types
}

// declKey names a declaration the way a resolved call site spells it.
func declKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return bareTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func bareTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return bareTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// A declared slug is the same for every caller, so a connector needing one URL
// per member gets a trailing ref it minted and resolves for itself.
func TestInboundCarriesTheTrailingRefToTheUnit(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	if got := serve(mux, signedRequest("/webhooks/ext/u/capture/mB1e-7Zq", time.Now(), "{}")).Code; got != http.StatusAccepted {
		t.Fatalf("a request with a ref answered %d", got)
	}
	if p.saw.Ref != "mB1e-7Zq" {
		t.Fatalf("the unit saw ref %q, want the trailing segment", p.saw.Ref)
	}
	if p.saw.Slug != "capture" {
		t.Fatalf("the unit saw slug %q — the ref must not displace the declared slug", p.saw.Slug)
	}
}

// The bare form still serves, and answers an empty ref rather than refusing: an
// endpoint that needs no per-member handle should not have to invent one.
func TestInboundWithoutARefAnswersAnEmptyOne(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)
	if got := serve(mux, signedRequest("/webhooks/ext/u/capture", time.Now(), "{}")).Code; got != http.StatusAccepted {
		t.Fatalf("the bare form answered %d", got)
	}
	if p.saw.Ref != "" {
		t.Fatalf("a request with no ref carried %q", p.saw.Ref)
	}
}

// A ref outside the published rule is a 404, never an empty one passed on: a
// unit handed "" for a value the caller did spell would resolve the wrong row,
// or none, and answer the same opaque 401 either way.
func TestInboundRefusesAnUngrammaticalRef(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux := http.NewServeMux()
	ws := ids.From[ids.WorkspaceKind](ids.MustParse("00000000-0000-0000-0000-0000000000a1"))
	MountInboundEndpoints(mux, []extension.Extension{p.unit()},
		func(context.Context) (ids.WorkspaceID, error) { return ws, nil },
		extensionRuntimeBinding{}, quietLog())
	h, _ := mux.Handler(signedRequest("/webhooks/ext/u/capture", time.Now(), "{}"))
	for _, ref := range []string{
		strings.Repeat("r", extension.MaxInboundRef+1),
		"has.dot",
		"has~tilde",
		"has:colon",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest("/webhooks/ext/u/capture/"+ref, time.Now(), "{}"))
		if w.Code != http.StatusNotFound {
			t.Errorf("ref %q answered %d, want 404", ref, w.Code)
		}
	}
	if p.calls != 0 {
		t.Fatalf("an ungrammatical ref reached the unit %d times", p.calls)
	}
}

// A ref is a handle, not a credential — it must never become a rate-limiter key,
// because ratelimit leaves an over-long key unmetered and ADMITTED, which would
// make a long ref a self-serve way off the meter.
func TestInboundMetersOnTheDeclaredEndpointNotTheRef(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)

	// EVERY REQUEST FROM ITS OWN ADDRESS, which is what makes this test about
	// the endpoint bucket at all. The fixture allows 60 per IP and 120 per
	// endpoint; driving one address past 60 refuses on the IP bucket and proves
	// nothing about the other, so the refusal below has to arrive with every
	// per-IP budget still untouched.
	//
	// A DISTINCT REF EACH TIME, because the ref is the caller-chosen segment: if
	// it reached the limiter key, each of these would open its own budget and
	// all 121 would be admitted.
	codes := map[int]int{}
	for i := range 121 {
		r := signedRequest("/webhooks/ext/u/capture/ref"+strconv.Itoa(i), time.Now(), "{}")
		r.RemoteAddr = "10.0." + strconv.Itoa(i/250) + "." + strconv.Itoa(i%250+1) + ":1234"
		codes[serve(mux, r).Code]++
	}
	if codes[http.StatusTooManyRequests] == 0 {
		t.Fatalf("121 requests, each from its own address and under its own ref, were all admitted (%v) — "+
			"the endpoint's 120/min budget was never spent, so the ref is buying a budget of its own", codes)
	}
	if codes[http.StatusAccepted] != 120 {
		t.Fatalf("the endpoint admitted %d of 121 (%v), want exactly its declared 120 — "+
			"a different number means the refusal came from some other bucket", codes[http.StatusAccepted], codes)
	}
}

// The nonce is caller-chosen and a unit is told to KEEP it, under an index
// that by construction never collapses two of them. An unbounded one is
// therefore an unbounded write, which is why the ref beside it is bounded too —
// and why this is refused at the core rather than left to each unit to remember.
func TestInboundRefusesAnUnboundedNonce(t *testing.T) {
	tests := []struct {
		name  string
		nonce string
		want  int
	}{
		{"at the bound", strings.Repeat("a", extension.MaxInboundNonce), http.StatusAccepted},
		{"one past the bound", strings.Repeat("a", extension.MaxInboundNonce+1), http.StatusUnauthorized},
		{"far past the bound", strings.Repeat("a", 64<<10), http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &inboundProbe{outcome: extension.InboundAccepted}
			mux, _ := mountProbe(t, p)
			r := signedRequest("/webhooks/ext/u/capture", time.Now(), "{}")
			r.Header.Set(extension.InboundHeaderNonce, tc.nonce)
			if got := serve(mux, r).Code; got != tc.want {
				t.Fatalf("a %d-character nonce answered %d, want %d", len(tc.nonce), got, tc.want)
			}
		})
	}
}

// An over-long nonce is refused with the SAME opaque answer as a wrong
// signature. Telling a caller its nonce was too long would be a second refusal
// reason distinguishable from the first, and the whole discipline of this edge
// is that there is exactly one.
func TestAnOverLongNonceIsRefusedLikeEverythingElse(t *testing.T) {
	p := &inboundProbe{outcome: extension.InboundAccepted}
	mux, _ := mountProbe(t, p)

	long := signedRequest("/webhooks/ext/u/capture", time.Now(), "{}")
	long.Header.Set(extension.InboundHeaderNonce, strings.Repeat("a", extension.MaxInboundNonce+1))
	over := serve(mux, long)

	absent := signedRequest("/webhooks/ext/u/capture", time.Now(), "{}")
	absent.Header.Del(extension.InboundHeaderNonce)
	none := serve(mux, absent)

	if over.Code != none.Code || over.Body.String() != none.Body.String() {
		t.Fatalf("an over-long nonce answered %d/%q and a missing one %d/%q — the two refusals are distinguishable",
			over.Code, over.Body.String(), none.Code, none.Body.String())
	}
}
