// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// A download's headers are spelled once, in platform/httperr's Download.
//
// StreamObject's docblock claimed to be "the one spelling of set
// Content-Type/-Disposition/-Length, copy" and it was not: two export handlers
// wrote their own, because StreamObject reads FROM a reader and they write
// their body themselves. The shared half was real and the difference was real,
// and the copies happened for the honest reason — the helper's shape excluded
// them. So the description of a download was split from the source of its
// bytes, and the header trio has one implementation both shapes call.
//
// The subject is Content-Disposition set on an http.ResponseWriter, because
// that header is what makes a response a download. Read off the syntax —
// `<expr>.Header().Set("Content-Disposition", …)` — and not off the text: this
// tree also writes that header name into MIME parts (an outbound Gmail
// message, a Telegram multipart upload), which is a different header on a
// different thing, and a gate that reported those would be waived into
// uselessness within a week.
//
// It reads the RESPONSE's header map only. `*http.Request` carries a `Header()`
// method too, writing the request's own headers — the opposite direction, and
// not a download — so the receiver is matched by name as well as the method.
// A writer under a name this tree does not use is outside the net, which is the
// cost of a syntactic census having no types to ask.
//
// WHAT IT CANNOT SEE. A download that carries no filename sets no
// Content-Disposition, so a handler serving inline bytes under Content-Type
// alone is outside this net (the organization logo is such a response, and it
// goes through Download anyway). Nor can it see a header set through a
// variable holding the name, or one written with Add rather than Set. It is a
// net under the shape the tree reaches for, not a proof.
//
// Extensions are their own Go modules and outside gatekit's tree, so they are
// not swept here. None carries the shape today.

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// downloadHeaderScope: the owner holds the spelling, and nothing outside it
// may write a download's disposition.
var downloadHeaderScope = gatekit.Scope{
	Roots:   []string{"internal/platform/httperr"},
	Subject: setsADownloadDisposition,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestADownloadsHeadersAreSpelledOnce(t *testing.T) {
	// EXACTLY one WRITE, not one file. A file is the wrong unit twice over:
	// two writes in one file are two spellings reported as one, and zero of
	// them reads the same as a clean tree.
	total := 0
	var where []string
	for _, f := range downloadHeaderScope.Files(t) {
		n := countDownloadDispositions(f.File)
		total += n
		where = append(where, fmt.Sprintf("%s (%d)", f.Path, n))
	}
	if total != 1 {
		sites := "(no file under internal/platform/httperr writes one)"
		if len(where) > 0 {
			sites = strings.Join(where, "\n\t")
		}
		t.Errorf("a download's disposition is written %d time(s):\n\t%s\n\n"+
			"One spelling of the header trio, so a handler cannot agree with the others by inspection and "+
			"then drift on the quoting, the disposition or the omitted Content-Length. Build an "+
			"httperr.Download and call WriteHeaders", total, sites)
	}
}

// setsADownloadDisposition reports whether a file writes Content-Disposition
// onto a response's header map.
func setsADownloadDisposition(_ string, file *ast.File) bool {
	return countDownloadDispositions(file) > 0
}

// countDownloadDispositions counts the writes, because the file is the wrong
// unit: two in one file are two spellings that can drift apart.
func countDownloadDispositions(file *ast.File) int {
	hoisted := headerMapsIn(file)
	total := 0
	ast.Inspect(file, func(n ast.Node) bool {
		if isResponseHeaderSet(n, "Content-Disposition", hoisted) {
			total++
		}
		return true
	})
	return total
}

// headerMapsIn names the locals holding a response's header map — `h :=
// w.Header()`. Without them the two-line spelling is a write this census
// cannot see, and hoisting the map is what an author does the moment a handler
// sets more than one header.
func headerMapsIn(file *ast.File) map[string]bool {
	named := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		name, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || !isHeaderCall(assign.Rhs[0]) {
			return true
		}
		named[name.Name] = true
		return true
	})
	return named
}

// isHeaderCall matches `<w>.Header()` on a RESPONSE writer.
//
// The receiver has to be named, not merely have the method. `*http.Request`
// carries `Header()` too — reading and writing the REQUEST's headers, which is
// the opposite direction and not a download at all. A census that matched the
// method alone would report `r.Header().Set("Content-Disposition", …)` as a
// second spelling of the response trio, and a gate that fires on the wrong
// direction is one nobody reads twice.
//
// Named rather than type-checked: a census over syntax has no types, and every
// response writer in this tree is called `w` or `rw`. The cost is stated in the
// header — a writer under another name is outside the net.
func isHeaderCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	accessor, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || accessor.Sel.Name != "Header" {
		return false
	}
	receiver, ok := accessor.X.(*ast.Ident)
	return ok && responseWriterNames[receiver.Name]
}

// responseWriterNames are what a response writer is called in this tree.
var responseWriterNames = map[string]bool{"w": true, "rw": true, "writer": true}

// isResponseHeaderSet matches `<expr>.Header().Set("<name>", …)`.
//
// The receiver of Set must be a response's header map — reached inline as
// `w.Header()`, or through a local this file assigned that call to. That is
// what separates it from a MIME part's: `header.Set("Content-Disposition", …)`
// on a textproto.MIMEHeader is the same method name on a different thing, and
// both live in this tree. Tracking the local is what stops the two-line
// spelling — which is what an author writes the moment a handler sets more
// than one header — from being a write this census cannot see.
func isResponseHeaderSet(n ast.Node, name string, hoisted map[string]bool) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	set, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || set.Sel.Name != "Set" {
		return false
	}
	// Either the map is reached inline — `w.Header().Set(…)` — or through a
	// local this file assigned it to.
	if !isHeaderCall(set.X) {
		receiver, isIdent := set.X.(*ast.Ident)
		if !isIdent || !hoisted[receiver.Name] {
			return false
		}
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}
	unquoted, err := strconv.Unquote(literal.Value)
	return err == nil && strings.EqualFold(unquoted, name)
}

// TestTheDownloadHeaderCensusStillSeesItsSubject is the vacuity check: a
// census that has stopped matching passes by finding nothing, which is the same
// word it prints over a clean tree.
func TestTheDownloadHeaderCensusStillSeesItsSubject(t *testing.T) {
	subjects := map[string]string{
		"a disposition written straight onto the response": "" +
			"func f(w http.ResponseWriter) { w.Header().Set(\"Content-Disposition\", `attachment; filename=\"x\"`) }",
		"the same header named in the casing the wire uses": "" +
			"func f(w http.ResponseWriter) { w.Header().Set(\"content-disposition\", d) }",
		"a disposition set on something that is not called w": "" +
			"func f(rw http.ResponseWriter) { rw.Header().Set(\"Content-Disposition\", d) }",
	}
	subjects["the two-line spelling, through a local holding the header map"] = "" +
		"func f(w http.ResponseWriter) {\n\th := w.Header()\n\th.Set(\"Content-Disposition\", d)\n}"
	for name, body := range subjects {
		if !setsADownloadDisposition("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the census no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"the same header name on a MIME part, which is not a download": "" +
			"func f(header textproto.MIMEHeader) { header.Set(\"Content-Disposition\", `form-data; name=\"a\"`) }",
		"a header name passed to a helper rather than set on a response": "" +
			"func f(b *bytes.Buffer) { writeHeader(b, \"Content-Disposition\", d) }",
		"a different response header, which every JSON handler writes": "" +
			"func f(w http.ResponseWriter) { w.Header().Set(\"Content-Type\", \"application/json\") }",
		"a read of the request's own disposition": "" +
			"func f(r *http.Request) string { return r.Header.Get(\"Content-Disposition\") }",
		"a WRITE to the request's headers, which is the other direction": "" +
			"func f(r *http.Request) { r.Header().Set(\"Content-Disposition\", d) }",
		"the same through a local, which is how an outbound request is built": "" +
			"func f(r *http.Request) {\n\th := r.Header()\n\th.Set(\"Content-Disposition\", d)\n}",
		"a Set on a local that holds a MIME part, not a response's map": "" +
			"func f(part textproto.MIMEHeader) { part.Set(\"Content-Disposition\", d) }",
	}
	for name, body := range nearMisses {
		if setsADownloadDisposition("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the census counts %s as a second spelling of a download's headers; it will be waived "+
				"into uselessness if it fires on every appearance of the header name", name)
		}
	}
}
