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
	inside := downloadHeaderScope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("a download's disposition is written in %d files inside the package that owns it:\n\t%s\n\n"+
			"One spelling of the header trio, so a handler cannot agree with the others by inspection and "+
			"then drift on the quoting, the disposition or the omitted Content-Length. Build an "+
			"httperr.Download and call WriteHeaders", len(inside), strings.Join(where, "\n\t"))
	}
}

// setsADownloadDisposition reports whether a file writes Content-Disposition
// onto a response's header map.
func setsADownloadDisposition(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		if isResponseHeaderSet(n, "Content-Disposition") {
			found = true
		}
		return !found
	})
	return found
}

// isResponseHeaderSet matches `<expr>.Header().Set("<name>", …)`.
//
// The receiver of Set must itself be a call to Header, which is what separates
// a response's header map from a MIME part's: `header.Set("Content-Disposition",
// …)` on a textproto.MIMEHeader is the same method name on a different thing,
// and both live in this tree.
func isResponseHeaderSet(n ast.Node, name string) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	set, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || set.Sel.Name != "Set" {
		return false
	}
	header, ok := set.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	accessor, ok := header.Fun.(*ast.SelectorExpr)
	if !ok || accessor.Sel.Name != "Header" || len(header.Args) != 0 {
		return false
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
	}
	for name, body := range nearMisses {
		if setsADownloadDisposition("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the census counts %s as a second spelling of a download's headers; it will be waived "+
				"into uselessness if it fires on every appearance of the header name", name)
		}
	}
}
