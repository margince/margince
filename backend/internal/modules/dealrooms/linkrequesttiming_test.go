// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// That an anonymous link request costs the same wall-clock whichever address it
// names.
//
// The status, the body and the headers are uniform already; the clock was not.
// WriteHeader does not reach the network — net/http buffers a response until
// the handler returns — so a 202 written before the seat lookup, the credential
// reissue and a synchronous SMTP send was still DELIVERED after all three. A
// caller times a known address against an unknown one and learns which
// addresses this installation has seats for.
//
// Two things have to hold and each is checked its own way: the answer is
// actually pushed (behaviour, below), and it is pushed BEFORE the work
// (structure, over the source — the same instrument public_boundary_test.go
// uses on this file, because the store call underneath needs a database and the
// ordering is a property of the handler rather than of any answer it gives).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTheAnswerIsPushedRatherThanBuffered(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	answerAndFlush(rec, httptest.NewRequest(http.MethodPost, "/v1/public/rooms/link-request", nil),
		http.StatusAccepted)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}
	if !rec.Flushed {
		t.Error("the answer was written and not flushed: net/http holds it until the handler " +
			"returns, so the caller waits out the work and times a known address against an " +
			"unknown one")
	}
}

// A wrapped writer still flushes. Every handler in this tree sees one — access
// logging and correlation both wrap — and `w.(http.Flusher)` answers false for
// a wrapper that does not forward the method, which is the shape where the leak
// stands with the code that closes it in place.
func TestAWrappedWriterStillFlushes(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	answerAndFlush(onlyAResponseWriter{rec},
		httptest.NewRequest(http.MethodPost, "/v1/public/rooms/link-request", nil),
		http.StatusAccepted)

	if !rec.Flushed {
		t.Error("a wrapper that does not itself implement Flusher swallowed the flush — " +
			"the answer is buffered again and the timing says which addresses are known")
	}
}

// onlyAResponseWriter forwards the interface and nothing else, the way a
// middleware wrapper written without Flush does.
type onlyAResponseWriter struct{ inner http.ResponseWriter }

func (o onlyAResponseWriter) Header() http.Header         { return o.inner.Header() }
func (o onlyAResponseWriter) Write(b []byte) (int, error) { return o.inner.Write(b) }
func (o onlyAResponseWriter) WriteHeader(status int)      { o.inner.WriteHeader(status) }

// Unwrap is what ResponseController follows. A wrapper without it cannot be
// unwrapped by anything, which is a fact about the wrapper rather than about
// this edge.
func (o onlyAResponseWriter) Unwrap() http.ResponseWriter { return o.inner }

func TestTheLinkRequestAnswersBeforeItLooksAnythingUp(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), publicHandlersFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", publicHandlersFile, err)
	}
	body := functionNamed(t, file, "RequestDealRoomLink")

	answeredAt, workAt := -1, -1
	deferred := false
	ast.Inspect(body, func(n ast.Node) bool {
		// A DEFERRED answer reads as first and runs as last, which is the one
		// rewrite that would keep the position check below green while putting
		// the whole leak back.
		if d, isDefer := n.(*ast.DeferStmt); isDefer {
			if fn, isIdent := d.Call.Fun.(*ast.Ident); isIdent && fn.Name == "answerAndFlush" {
				deferred = true
			}
			return true
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name == "answerAndFlush" && answeredAt < 0 {
				answeredAt = int(call.Pos())
			}
		case *ast.SelectorExpr:
			// The first call into the store is the first thing whose duration
			// depends on whether the address is known.
			if sel, isSel := fn.X.(*ast.SelectorExpr); isSel && sel.Sel.Name == "store" && workAt < 0 {
				workAt = int(call.Pos())
			}
		}
		return true
	})

	if answeredAt < 0 {
		t.Fatal("RequestDealRoomLink no longer answers through answerAndFlush: a bare WriteHeader " +
			"is buffered until the handler returns, which is the leak this holds")
	}
	if workAt < 0 {
		t.Fatal("RequestDealRoomLink reaches the store nowhere — the ordering this checks is gone, " +
			"so the check has stopped meaning anything")
	}
	if deferred {
		t.Error("the answer is deferred: it reads as the first thing the handler does and runs as " +
			"the last, so the caller waits out the work exactly as before")
	}
	if answeredAt > workAt {
		t.Error("the handler reaches the store before it answers: the caller then waits for a " +
			"lookup whose cost depends on whether the address is known, which is the timing " +
			"channel the uniform status and body were chosen to close")
	}
}

// functionNamed answers one top-level function's body.
func functionNamed(t *testing.T, file *ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name && fn.Body != nil {
			return fn.Body
		}
	}
	t.Fatalf("%s declares no %s", publicHandlersFile, name)
	return nil
}
