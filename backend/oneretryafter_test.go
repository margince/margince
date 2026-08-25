// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// The Retry-After header is read in one place, shared/kernel/retryafter.
//
// It was read in six: four byte-identical copies under capture (Gmail, the
// Google connector base, Graph, the OAuth flow), the geocoder's own, and
// Telegram's. Only the geocoder's handled the whole header — RFC 9110 §10.2.3
// gives Retry-After two forms, delta-seconds and an HTTP-date, and the four
// capture copies parsed only the first.
//
// That difference is the reason this is worth a gate rather than a tidy-up. A
// provider answering with a date was read as having said nothing at all, so the
// connector fell back to its own backoff and came back when IT guessed rather
// than when the provider asked — and coming back early on a rate limit is how a
// throttle becomes a ban. Fixing it meant editing four files, which is exactly
// the arithmetic that leaves one of them unedited.
//
// WHAT THIS GATE CAN AND CANNOT SEE. The subject is a read of the header name:
// `Header.Get("Retry-After")`, in any casing, off the syntax rather than the
// text so the name inside a comment is not a finding. It cannot see a read
// through a variable holding the name, nor a provider-specific interval carried
// somewhere else entirely — Telegram's own `parameters.retry_after` is such a
// value, and it stays in Telegram because it IS Telegram's, with the header
// underneath it as the fallback. That envelope is not this subject.

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// retryAfterScope claims the header is read in kernel and nowhere else.
// Nothing is exempt: kernel sits below every tier that talks to a provider, so
// every caller can reach retryafter.Of.
var retryAfterScope = gatekit.Scope{
	Roots:   []string{"internal/shared/kernel/retryafter"},
	Subject: readsTheRetryAfterHeader,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestTheRetryAfterReadingIsSpelledOnce(t *testing.T) {
	inside := retryAfterScope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("the Retry-After header is read in %d files inside the package that owns it:\n\t%s\n\n"+
			"One reading, so a connector cannot be given the HTTP-date form while another is not. Call "+
			"retryafter.Of", len(inside), strings.Join(where, "\n\t"))
	}
}

// readsTheRetryAfterHeader reports whether a file reads the header off a
// header map — `<expr>.Get("Retry-After")`, the casing being the wire's own
// business rather than the caller's.
func readsTheRetryAfterHeader(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		get, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || get.Sel.Name != "Get" {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(literal.Value)
		if err == nil && strings.EqualFold(name, "Retry-After") {
			found = true
		}
		return !found
	})
	return found
}

// TestTheRetryAfterCensusStillSeesItsSubject is the vacuity check: a census
// that has stopped matching passes by finding nothing, which is the same word
// it prints over a clean tree.
func TestTheRetryAfterCensusStillSeesItsSubject(t *testing.T) {
	subjects := map[string]string{
		"a read off a response's headers": "" +
			"package p\nfunc f(resp *http.Response) string { return resp.Header.Get(\"Retry-After\") }",
		"the same name in the casing the wire uses": "" +
			"package p\nfunc f(h http.Header) string { return h.Get(\"retry-after\") }",
		"a read off something that is not called resp": "" +
			"package p\nfunc f(r *http.Response) string { return r.Header.Get(\"Retry-After\") }",
	}
	for name, body := range subjects {
		if !readsTheRetryAfterHeader("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"the header NAME in a comment, which is prose and not a read": "" +
			"package p\n// Retry-After is honoured by the registry.\nfunc f() {}",
		"a different header, which half the transports read": "" +
			"package p\nfunc f(resp *http.Response) string { return resp.Header.Get(\"Content-Type\") }",
		"a provider's own interval carried in its envelope": "" +
			"package p\nfunc f(env envelope) int { return env.Parameters.RetryAfter }",
		"a Get with the wrong arity, which is a different method": "" +
			"package p\nfunc f(m url.Values) string { return m.Get(\"Retry-After\", \"fallback\") }",
	}
	for name, body := range nearMisses {
		if readsTheRetryAfterHeader("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census claims %s is a second reading of the header", name)
		}
	}
}
