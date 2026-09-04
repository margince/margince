// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The lanes cannot outlive the boot that started them, and the reason is the
// SHAPE rather than a line somebody remembered to write.
//
// It used to be the line. startEventLanes created the lane context and the wait
// group, returned them on a value, and run() wrote `defer lanes.join()` on the
// next line — correct, and one edit from silently wrong. Deleting that defer,
// or hoisting the call above the pool so LIFO reversed, left every test in this
// repository passing: the two `defer`s and their ORDER were the whole
// invariant, and nothing asserted either. margince/margince#454.
//
// Now run() creates the lifetime and defers its end BEFORE calling the function
// that starts anything on it, so a lane cannot exist ahead of its own shutdown.
// These hold the properties that makes true.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestALaneCannotStartBeforeItsShutdownIsRegistered is the structural half, and
// it is the one that would have caught the original defect.
//
// A behavioural test cannot: run() needs a database, a bus and a configured
// deployment before it reaches the lanes at all, so the orderings this is about
// are unreachable from a unit test and the integration lane starts the process
// rather than inspecting it. What CAN be checked is that run's body still puts
// the deferred join ahead of the call that starts lanes — which is the property,
// stated exactly.
func TestALaneCannotStartBeforeItsShutdownIsRegistered(t *testing.T) {
	t.Parallel()
	body := runBody(t, "main.go", "run")

	// The DEFER of whatever laneLifetime handed back, not a named function: the
	// end is a closure the helper returns, so the shape run() must keep is "a
	// deferred call registered before the start", and matching on a specific
	// callee would break the moment the helper's return is renamed while the
	// property still held.
	deferAt := indexOfStatement(body, func(s string) bool {
		return strings.HasPrefix(s, "defer endLanes(")
	})
	lifetimeAt := indexOfStatement(body, func(s string) bool {
		return strings.Contains(s, "laneLifetime(")
	})
	startAt := indexOfStatement(body, func(s string) bool {
		return strings.Contains(s, "startEventLanes(")
	})

	if deferAt < 0 || lifetimeAt < 0 {
		t.Fatal("run() no longer takes a lane lifetime and defers its end. That pair is what stops " +
			"goroutines reading the bus and the pool after run returns; without it a boot failure " +
			"leaves them running against a closed pool (#454)")
	}
	if startAt < 0 {
		t.Fatal("run() no longer calls startEventLanes, so this gate is about a shape that is gone")
	}
	if deferAt > startAt {
		t.Errorf("run() starts the lanes at statement %d and defers their shutdown at %d. The defer "+
			"must come FIRST: a lane that starts before its own shutdown is registered survives a "+
			"failure on any later line, which is exactly what #454 found nothing was holding.",
			startAt, deferAt)
	}
}

// TestTheLaneJoinIsNotAMethodOnTheValueItShutsDown pins why joinLanes is a free
// function.
//
// A method can only be deferred once its receiver exists — that is, after the
// call that starts the lanes — which is the ordering the defect lived in. Making
// it a function over the context and the group is what lets run() defer it
// first, so the shape above is possible at all.
func TestTheLaneJoinIsNotAMethodOnTheValueItShutsDown(t *testing.T) {
	t.Parallel()
	file := parseWorkerFile(t, "lanes.go")
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "joinLanes" {
			continue
		}
		if fn.Recv != nil {
			t.Error("joinLanes has a receiver. A method can only be deferred after the call that " +
				"produced it, which is the ordering #454 was about — run() must be able to defer " +
				"the shutdown before anything starts.")
		}
		return
	}
	t.Fatal("no joinLanes declaration in lanes.go")
}

// And it still ends the lanes and waits for them, bounded.
func TestJoinLanesStopsTheLanesAndWaits(t *testing.T) {
	t.Parallel()
	ctx, stop := context.WithCancel(context.Background())
	var background sync.WaitGroup
	observed := make(chan struct{})

	background.Add(1)
	go func() {
		defer background.Done()
		<-ctx.Done() // only the stop can release this
		close(observed)
	}()

	joinLanes(stop, &background, slog.New(slog.DiscardHandler))

	select {
	case <-observed:
	default:
		t.Fatal("joinLanes returned while a lane goroutine was still running — the pool and the bus " +
			"it reads are closed on the next deferred statement")
	}
}

// An overrunning lane is bounded rather than hanging the process, which on the
// boot-failure path is the difference between `exit 1` and a supervisor reading
// a dead worker as still starting.
func TestJoinLanesGivesUpOnALaneThatIgnoresCancellation(t *testing.T) {
	t.Parallel()
	_, stop := context.WithCancel(context.Background())
	var background sync.WaitGroup
	release := make(chan struct{})
	background.Add(1)
	go func() {
		defer background.Done()
		<-release // deliberately deaf to the cancel
	}()
	t.Cleanup(func() { close(release) })

	var log strings.Builder
	done := make(chan struct{})
	go func() {
		joinLanesWithin(stop, &background, slog.New(slog.NewTextHandler(&log, nil)), 10*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("joinLanesWithin never returned — a lane that ignores its context hangs the process")
	}
	if !strings.Contains(log.String(), "level=ERROR") {
		t.Errorf("the overrun was not reported: %s", log.String())
	}
}

// --- helpers -----------------------------------------------------------------

func parseWorkerFile(t *testing.T, name string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return file
}

// runBody renders each top-level statement of one function as source-ish text,
// in order, so a test can ask which comes first.
func runBody(t *testing.T, file, fn string) []string {
	t.Helper()
	parsed := parseWorkerFile(t, file)
	for _, decl := range parsed.Decls {
		decl, ok := decl.(*ast.FuncDecl)
		if !ok || decl.Name.Name != fn || decl.Body == nil {
			continue
		}
		out := make([]string, 0, len(decl.Body.List))
		for _, stmt := range decl.Body.List {
			out = append(out, renderStatement(parsed, stmt))
		}
		return out
	}
	t.Fatalf("no %s declaration in %s", fn, file)
	return nil
}

// renderStatement is enough of a rendering to match on: the callee names and the
// `defer` keyword, which is all these assertions ask about.
func renderStatement(file *ast.File, stmt ast.Stmt) string {
	var b strings.Builder
	if _, ok := stmt.(*ast.DeferStmt); ok {
		b.WriteString("defer ")
	}
	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			b.WriteString(fun.Name + "(")
		case *ast.SelectorExpr:
			b.WriteString(fun.Sel.Name + "(")
		}
		return true
	})
	_ = file
	return b.String()
}

func indexOfStatement(body []string, match func(string) bool) int {
	for i, s := range body {
		if match(s) {
			return i
		}
	}
	return -1
}
